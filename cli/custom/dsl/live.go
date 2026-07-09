package dsl

import (
	"fmt"
	"net/url"
	"strings"
)

// FetchLive resolves the live counterpart of a manifest. Resolution order:
//  1. state entry → GET by stored server id (404 = deleted out-of-band → fall through)
//  2. identity-addressable kinds → GET by identity value
//  3. list + client-side match on the identity field
//
// Returns (nil, "", nil) when the resource does not exist.
func FetchLive(c *Client, m *Manifest, info KindInfo, st *StateDoc) (map[string]any, string, error) {
	if r := st.Find(m.Kind, m.Identity()); r != nil && r.ServerID != "" {
		obj, err := getOne(c, info, r.ServerID)
		if err == nil && obj != nil {
			return obj, r.ServerID, nil
		}
		if err != nil && !IsNotFound(err) {
			return nil, "", err
		}
		// state points at a deleted resource: fall through to rediscovery
	}

	if info.GetByIdentity {
		obj, err := getOne(c, info, m.IdentityValue())
		if err != nil && !IsNotFound(err) {
			return nil, "", err
		}
		if obj != nil {
			return obj, extractID(obj, info), nil
		}
		return nil, "", nil
	}

	items, err := c.ListAll(info.BasePath, "")
	if err != nil {
		return nil, "", err
	}
	var matches []map[string]any
	for _, item := range items {
		if matchesIdentity(item, m, info) {
			matches = append(matches, item)
		}
	}
	switch len(matches) {
	case 0:
		return nil, "", nil
	case 1:
		return matches[0], extractID(matches[0], info), nil
	default:
		return nil, "", fmt.Errorf("%s: %d live resources match identity %q — the workspace has duplicates; delete the extras or import the right one into state",
			m.Identity(), len(matches), m.IdentityValue())
	}
}

func getOne(c *Client, info KindInfo, idOrIdentity string) (map[string]any, error) {
	var raw map[string]any
	err := c.Do("GET", info.BasePath+"/"+url.PathEscape(idOrIdentity), nil, &raw)
	if err != nil {
		return nil, err
	}
	return unwrap(raw, info), nil
}

// unwrap peels {"project": {...}} / {"skill": {...}} single-key wrappers.
func unwrap(obj map[string]any, info KindInfo) map[string]any {
	if info.Wrap == "" {
		return obj
	}
	if inner, ok := obj[info.Wrap].(map[string]any); ok {
		return inner
	}
	return obj
}

func extractID(obj map[string]any, info KindInfo) string {
	for _, f := range []string{info.IDField, "_id", "id"} {
		if f == "" {
			continue
		}
		if v, ok := obj[f].(string); ok && v != "" {
			return v
		}
	}
	return ""
}

// matchesIdentity compares a list item against the manifest identity.
func matchesIdentity(item map[string]any, m *Manifest, info KindInfo) bool {
	get := func(f string) string {
		v, _ := item[f].(string)
		return v
	}
	switch info.IdentityMode {
	case "name":
		return get("name") == m.Metadata.Name
	case "key":
		// Evaluator reads expose key (server maps display_name back to key).
		return get("key") == m.Metadata.Key
	default: // display_name
		if !strings.EqualFold(get("display_name"), m.Metadata.DisplayName) {
			return false
		}
		// When reads carry a path, use it to disambiguate folders.
		if info.ReadHasPath {
			if p := get("path"); p != "" && m.Metadata.Path != "" && p != m.Metadata.Path {
				return false
			}
		}
		return true
	}
}

// NormalizeLive deep-copies a live object and drops server-computed fields so
// only spec-comparable content remains.
func NormalizeLive(live map[string]any, info KindInfo) map[string]any {
	if live == nil {
		return nil
	}
	out := deepCopyMap(live)
	for _, f := range info.Strip {
		delete(out, f)
	}
	return out
}

func deepCopyMap(in map[string]any) map[string]any {
	out := make(map[string]any, len(in))
	for k, v := range in {
		out[k] = deepCopyVal(v)
	}
	return out
}

func deepCopyVal(v any) any {
	switch t := v.(type) {
	case map[string]any:
		return deepCopyMap(t)
	case []any:
		out := make([]any, len(t))
		for i, e := range t {
			out[i] = deepCopyVal(e)
		}
		return out
	default:
		return v
	}
}
