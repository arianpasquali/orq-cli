package dsl

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

func testClient(t *testing.T, h http.Handler) *Client {
	t.Helper()
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	t.Setenv("ORQ_API_KEY", "test-key")
	c := newClientWithBase(srv.URL)
	c.sleep = func(time.Duration) {}
	return c
}

func TestDoDecodesAndAuths(t *testing.T) {
	c := testClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer test-key" {
			t.Errorf("auth header = %q", r.Header.Get("Authorization"))
		}
		json.NewEncoder(w).Encode(map[string]any{"key": "abc"})
	}))
	var out map[string]any
	if err := c.Do("GET", "/v2/agents/abc", nil, &out); err != nil {
		t.Fatal(err)
	}
	if out["key"] != "abc" {
		t.Errorf("out = %v", out)
	}
}

func TestDoMissingKey(t *testing.T) {
	t.Setenv("ORQ_API_KEY", "")
	t.Setenv("ORQ_TOKEN", "")
	t.Setenv("ORQ_AUTHORIZATION", "")
	c := newClientWithBase("http://unused")
	if err := c.Do("GET", "/x", nil, nil); err == nil || !contains(err.Error(), "missing API key") {
		t.Fatalf("err = %v", err)
	}
}

func TestErrorEnvelopes(t *testing.T) {
	bodies := map[string]string{
		`{"message":"plain not found"}`:                          "plain not found",
		`{"error":{"code":"404","message":"hono style"}}`:        "hono style",
		`{"error":"bare string"}`:                                "bare string",
		`{"success":false,"error":{"name":"ZodError","message":"zod says no"}}`: "zod says no",
		`totally not json`: "totally not json",
	}
	for body, want := range bodies {
		c := testClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(400)
			fmt.Fprint(w, body)
		}))
		err := c.Do("POST", "/v2/x", map[string]any{}, nil)
		if err == nil || !contains(err.Error(), want) {
			t.Errorf("body %s: err = %v, want contains %q", body, err, want)
		}
	}
}

func TestRetryOn429(t *testing.T) {
	var calls int32
	c := testClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if atomic.AddInt32(&calls, 1) <= 2 {
			w.WriteHeader(429)
			return
		}
		fmt.Fprint(w, `{"ok":true}`)
	}))
	var out map[string]any
	if err := c.Do("GET", "/v2/agents", nil, &out); err != nil {
		t.Fatal(err)
	}
	if calls != 3 {
		t.Errorf("calls = %d, want 3", calls)
	}
}

func TestNoRetryOn400(t *testing.T) {
	var calls int32
	c := testClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		w.WriteHeader(400)
		fmt.Fprint(w, `{"message":"bad"}`)
	}))
	err := c.Do("GET", "/v2/agents", nil, nil)
	if err == nil || calls != 1 {
		t.Fatalf("err=%v calls=%d", err, calls)
	}
	if !IsNotFound(&APIError{Status: 404}) || IsNotFound(err) {
		t.Error("IsNotFound misbehaves")
	}
}

func TestListAllPaginates(t *testing.T) {
	pages := []string{
		`{"object":"list","data":[{"_id":"a"},{"_id":"b"}],"has_more":true}`,
		`{"object":"list","data":[{"_id":"c"}],"has_more":true}`,
		`{"object":"list","data":[{"_id":"d"}],"has_more":false}`,
	}
	var cursors []string
	c := testClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cursors = append(cursors, r.URL.Query().Get("starting_after"))
		fmt.Fprint(w, pages[len(cursors)-1])
	}))
	items, err := c.ListAll("/v2/prompts", "")
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 4 {
		t.Fatalf("items = %d", len(items))
	}
	wantCursors := []string{"", "b", "c"}
	for i, w := range wantCursors {
		if cursors[i] != w {
			t.Errorf("cursor[%d] = %q, want %q", i, cursors[i], w)
		}
	}
}

func TestListAllProjectCursor(t *testing.T) {
	var second string
	c := testClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("starting_after") == "" {
			fmt.Fprint(w, `{"object":"list","data":[{"project_id":"p1","name":"x"}],"has_more":true}`)
			return
		}
		second = r.URL.Query().Get("starting_after")
		fmt.Fprint(w, `{"object":"list","data":[],"has_more":false}`)
	}))
	if _, err := c.ListAll("/v2/projects", ""); err != nil {
		t.Fatal(err)
	}
	if second != "p1" {
		t.Errorf("project cursor = %q", second)
	}
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || len(sub) == 0 || indexOf(s, sub) >= 0)
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
