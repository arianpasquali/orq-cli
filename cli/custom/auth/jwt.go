package auth

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

func decodeBase64Segment(seg string) ([]byte, error) {
	if data, err := base64.RawURLEncoding.DecodeString(seg); err == nil {
		return data, nil
	}
	if data, err := base64.URLEncoding.DecodeString(seg); err == nil {
		return data, nil
	}
	if data, err := base64.RawStdEncoding.DecodeString(seg); err == nil {
		return data, nil
	}
	return base64.StdEncoding.DecodeString(seg)
}

func decodeJWTExpiry(token string) (time.Time, error) {
	parts := strings.Split(token, ".")
	if len(parts) < 2 {
		return time.Time{}, errors.New("invalid JWT format")
	}
	payload, err := decodeBase64Segment(parts[1])
	if err != nil {
		return time.Time{}, err
	}
	var claims struct {
		Exp int64 `json:"exp"`
	}
	if err := json.Unmarshal(payload, &claims); err != nil {
		return time.Time{}, err
	}
	if claims.Exp == 0 {
		return time.Time{}, errors.New("token missing exp claim")
	}
	return time.Unix(claims.Exp, 0).UTC(), nil
}

// formatISO formats a time as an ISO-8601 string with millisecond precision
// (e.g. 2026-04-13T12:34:56.000Z), matching JavaScript's Date.toISOString().
func formatISO(t time.Time) string {
	return t.UTC().Format("2006-01-02T15:04:05.000Z")
}

// parseISO parses an ISO-8601 timestamp accepting both fractional-second and
// no-fractional-second forms.
func parseISO(s string) (time.Time, error) {
	if s == "" {
		return time.Time{}, errors.New("empty timestamp")
	}
	layouts := []string{
		"2006-01-02T15:04:05.000Z07:00",
		time.RFC3339Nano,
		time.RFC3339,
	}
	for _, layout := range layouts {
		if t, err := time.Parse(layout, s); err == nil {
			return t, nil
		}
	}
	return time.Time{}, fmt.Errorf("unparseable timestamp: %s", s)
}

func isExpired(expiresAt string, skewSeconds int) bool {
	t, err := parseISO(expiresAt)
	if err != nil {
		return true
	}
	return time.Now().Add(time.Duration(skewSeconds) * time.Second).After(t)
}
