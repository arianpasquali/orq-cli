package dsl

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	bartolocli "github.com/orq-ai/bartolo/cli"
)

// Client is a thin authenticated JSON client for the public v2 API. It uses
// plain net/http (not the gentleman middleware chain) so that a missing
// API key surfaces as our own clear error and httptest servers stay trivial;
// auth semantics mirror bartolo's apikey middleware (same env vars).
type Client struct {
	base  string
	http  *http.Client
	sleep func(time.Duration) // injectable for tests
}

// NewClient resolves the base URL the same way every other orq command does
// (--server flag / ORQ_SERVER env / session default via bartolo).
func NewClient() *Client {
	return newClientWithBase(bartolocli.ResolveServer())
}

func newClientWithBase(base string) *Client {
	return &Client{
		base:  strings.TrimRight(base, "/"),
		http:  &http.Client{Timeout: 60 * time.Second},
		sleep: time.Sleep,
	}
}

func apiKey() string {
	for _, env := range []string{"ORQ_API_KEY", "ORQ_TOKEN", "ORQ_AUTHORIZATION"} {
		if v := os.Getenv(env); v != "" {
			return v
		}
	}
	return ""
}

// retryable HTTP statuses and cap.
const maxAttempts = 5

// Do performs one JSON request. body (when non-nil) is marshaled; out (when
// non-nil) receives the decoded 2xx response body. 429/502/503 retry with
// exponential backoff. Other non-2xx statuses return a parsed API error.
func (c *Client) Do(method, path string, body any, out any) error {
	key := apiKey()
	if key == "" {
		return fmt.Errorf("missing API key: set ORQ_API_KEY (or ORQ_TOKEN / ORQ_AUTHORIZATION)")
	}

	var payload []byte
	if body != nil {
		var err error
		if payload, err = json.Marshal(body); err != nil {
			return fmt.Errorf("%s %s: marshal body: %w", method, path, err)
		}
	}

	backoff := time.Second
	for attempt := 1; ; attempt++ {
		var reader io.Reader
		if payload != nil {
			reader = bytes.NewReader(payload)
		}
		req, err := http.NewRequest(method, c.base+path, reader)
		if err != nil {
			return err
		}
		req.Header.Set("Authorization", "Bearer "+key)
		if payload != nil {
			req.Header.Set("Content-Type", "application/json")
		}

		resp, err := c.http.Do(req)
		if err != nil {
			return fmt.Errorf("%s %s: %w", method, path, err)
		}
		data, _ := io.ReadAll(resp.Body)
		resp.Body.Close()

		switch {
		case resp.StatusCode >= 200 && resp.StatusCode < 300:
			if out != nil && len(bytes.TrimSpace(data)) > 0 {
				if err := json.Unmarshal(data, out); err != nil {
					return fmt.Errorf("%s %s: decode response: %w", method, path, err)
				}
			}
			return nil
		case (resp.StatusCode == 429 || resp.StatusCode == 502 || resp.StatusCode == 503) && attempt < maxAttempts:
			wait := backoff
			if ra := resp.Header.Get("Retry-After"); ra != "" {
				if d, err := time.ParseDuration(ra + "s"); err == nil {
					wait = d
				}
			}
			c.sleep(wait)
			backoff *= 2
		default:
			return &APIError{Method: method, Path: path, Status: resp.StatusCode, Message: parseAPIError(data)}
		}
	}
}

// APIError is a non-2xx response with the platform's message extracted.
type APIError struct {
	Method, Path string
	Status       int
	Message      string
}

func (e *APIError) Error() string {
	return fmt.Sprintf("%s %s: HTTP %d: %s", e.Method, e.Path, e.Status, e.Message)
}

// IsNotFound reports whether err is an API 404.
func IsNotFound(err error) bool {
	api, ok := err.(*APIError)
	return ok && api.Status == 404
}

// parseAPIError handles the platform's five-ish error body shapes:
// {error:{message}}, {message}, {error:"..."}, zod {success:false,error:{message}}, raw.
func parseAPIError(data []byte) string {
	var probe struct {
		Message string          `json:"message"`
		Error   json.RawMessage `json:"error"`
	}
	if err := json.Unmarshal(data, &probe); err == nil {
		if len(probe.Error) > 0 {
			var nested struct {
				Message string `json:"message"`
			}
			if json.Unmarshal(probe.Error, &nested) == nil && nested.Message != "" {
				return nested.Message
			}
			var s string
			if json.Unmarshal(probe.Error, &s) == nil && s != "" {
				return s
			}
		}
		if probe.Message != "" {
			return probe.Message
		}
	}
	msg := strings.TrimSpace(string(data))
	if len(msg) > 300 {
		msg = msg[:300] + "…"
	}
	if msg == "" {
		msg = "(no response body)"
	}
	return msg
}

// ListAll follows the cursor pagination of every v2 list endpoint
// ({object:"list", data:[], has_more}) and returns all items. extraQuery is
// appended verbatim ("&type=internal").
func (c *Client) ListAll(path string, extraQuery string) ([]map[string]any, error) {
	var (
		items []map[string]any
		after string
	)
	for {
		q := path + "?limit=100"
		if after != "" {
			q += "&starting_after=" + after
		}
		if extraQuery != "" {
			q += "&" + strings.TrimPrefix(extraQuery, "&")
		}
		var page struct {
			Data    []map[string]any `json:"data"`
			HasMore bool             `json:"has_more"`
		}
		if err := c.Do("GET", q, nil, &page); err != nil {
			return nil, err
		}
		items = append(items, page.Data...)
		if !page.HasMore || len(page.Data) == 0 {
			return items, nil
		}
		last := page.Data[len(page.Data)-1]
		after = ""
		for _, f := range []string{"_id", "id", "project_id", "skill_id"} {
			if v, ok := last[f].(string); ok && v != "" {
				after = v
				break
			}
		}
		if after == "" {
			return items, nil // cannot paginate further; avoid infinite loop
		}
	}
}
