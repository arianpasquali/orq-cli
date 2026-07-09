package dsl

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"reflect"
	"sort"
	"strings"
)

// Op classifies a planned change.
type Op string

const (
	OpCreate  Op = "create"
	OpUpdate  Op = "update"
	OpDelete  Op = "delete"
	OpReplace Op = "replace"
	OpNoop    Op = "noop"
)

// Change is one planned mutation.
type Change struct {
	Op       Op
	Kind     string
	Identity string
	Paths    []string  // changed spec paths (update/replace)
	Manifest *Manifest // nil for pure deletes
	LiveID   string    // server id when known
	Reason   string    // human note (e.g. which immutable field forced replace)
}

// DiffSpec compares desired against live with managed-fields semantics: only
// fields present in desired are considered; live-only fields are ignored.
// Returned paths are dotted (arrays compared atomically at their path).
func DiffSpec(desired, live map[string]any, prefix string) []string {
	var changed []string
	for k, dv := range desired {
		path := k
		if prefix != "" {
			path = prefix + "." + k
		}
		lv, exists := live[k]
		if !exists {
			changed = append(changed, path)
			continue
		}
		dm, dIsMap := dv.(map[string]any)
		lm, lIsMap := lv.(map[string]any)
		if dIsMap && lIsMap {
			changed = append(changed, DiffSpec(dm, lm, path)...)
			continue
		}
		if !scalarEqual(dv, lv) {
			changed = append(changed, path)
		}
	}
	sort.Strings(changed)
	return changed
}

// scalarEqual compares leaves after normalizing numbers (YAML ints vs JSON
// float64) via a canonical JSON round-trip.
func scalarEqual(a, b any) bool {
	if reflect.DeepEqual(a, b) {
		return true
	}
	ja, errA := json.Marshal(normNumbers(a))
	jb, errB := json.Marshal(normNumbers(b))
	return errA == nil && errB == nil && string(ja) == string(jb)
}

func normNumbers(v any) any {
	switch t := v.(type) {
	case int:
		return float64(t)
	case int32:
		return float64(t)
	case int64:
		return float64(t)
	case float32:
		return float64(t)
	case json.Number:
		f, err := t.Float64()
		if err != nil {
			return t.String()
		}
		return f
	case map[string]any:
		out := make(map[string]any, len(t))
		for k, e := range t {
			out[k] = normNumbers(e)
		}
		return out
	case []any:
		out := make([]any, len(t))
		for i, e := range t {
			out[i] = normNumbers(e)
		}
		return out
	default:
		return v
	}
}

// Classify turns (manifest, raw live object) into a Change. statePath is the
// declared path recorded at last apply — the comparison target for kinds
// whose reads never return path.
func Classify(m *Manifest, live map[string]any, liveID string, info KindInfo, statePath string) Change {
	ch := Change{Kind: m.Kind, Identity: m.Identity(), Manifest: m, LiveID: liveID}
	if live == nil {
		ch.Op = OpCreate
		return ch
	}
	paths := DiffSpec(m.Spec, NormalizeLive(live, info), "")

	// Envelope-level drift, compared outside spec.
	if m.Kind != "Project" {
		// Optional display_name on key-identified kinds (Agent, Tool, ...).
		if info.IdentityMode == "key" && m.Metadata.DisplayName != "" {
			if lv, _ := live["display_name"].(string); lv != m.Metadata.DisplayName {
				paths = append(paths, "metadata.display_name")
			}
		}
		// Path: live value when reads carry it, else the state record.
		target := ""
		if info.ReadHasPath {
			target, _ = live["path"].(string)
		} else {
			target = statePath
		}
		if target != "" && m.Metadata.Path != "" && target != m.Metadata.Path {
			paths = append(paths, "metadata.path")
		}
	}

	sort.Strings(paths)
	ch.Paths = paths
	if len(paths) == 0 {
		ch.Op = OpNoop
		return ch
	}
	for _, p := range paths {
		for _, imm := range info.Immutable {
			if p == imm || strings.HasPrefix(p, imm+".") {
				ch.Op = OpReplace
				ch.Reason = "immutable: " + imm
				return ch
			}
		}
	}
	ch.Op = OpUpdate
	return ch
}

// SpecHash fingerprints a resolved manifest (identity + spec + path), used to
// short-circuit no-op saves and detect out-of-band edits cheaply.
func SpecHash(m *Manifest) string {
	payload := map[string]any{
		"kind": m.Kind, "identity": m.Identity(), "path": m.Metadata.Path,
		"spec": normNumbers(m.Spec),
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return ""
	}
	sum := sha256.Sum256(data)
	return "sha256:" + hex.EncodeToString(sum[:8])
}

// symbolizeLive rewrites known server ids inside a live object back into the
// DSL's symbolic ref shapes, so desired (refs) and live (ids) diff cleanly.
// idMap: server_id → identity ("Kind/key"); mcp: discovered-tool id → parent
// Tool + tool name; refs are compared by bare key.
func symbolizeLive(live map[string]any, kind string, idMap map[string]string, mcp map[string]mcpDiscovered) map[string]any {
	if live == nil || len(idMap) == 0 {
		return live
	}
	bareKey := func(identity string) string {
		_, rest, _ := strings.Cut(identity, "/")
		if i := strings.LastIndex(rest, "|"); i >= 0 {
			return rest[i+1:]
		}
		return rest
	}
	out := deepCopyMap(live)
	if kind != "Agent" {
		return out
	}
	// knowledge_bases: [{knowledge_id: id}] → [{ref: key}]
	if kbs, ok := out["knowledge_bases"].([]any); ok {
		for i, e := range kbs {
			if em, ok := e.(map[string]any); ok {
				if id, _ := em["knowledge_id"].(string); id != "" {
					if ident, known := idMap[id]; known {
						kbs[i] = map[string]any{"ref": bareKey(ident)}
					}
				}
			}
		}
	}
	// settings.{guardrails,evaluators}: [{id: ..., execute_on, sample_rate}] → [{ref: key, ...}]
	if settings, ok := out["settings"].(map[string]any); ok {
		for _, field := range []string{"guardrails", "evaluators"} {
			items, ok := settings[field].([]any)
			if !ok {
				continue
			}
			for i, e := range items {
				em, ok := e.(map[string]any)
				if !ok {
					continue
				}
				id, _ := em["id"].(string)
				if ident, known := idMap[id]; known && id != "" {
					ne := deepCopyMap(em)
					delete(ne, "id")
					ne["ref"] = bareKey(ident)
					items[i] = ne
				}
			}
		}
		// settings.tools: mcp entries {type: mcp, tool_id} group back into
		// {type: mcp, ref, tools: [names]} when ids belong to one stack Tool.
		if tools, ok := settings["tools"].([]any); ok {
			settings["tools"] = symbolizeMCPTools(tools, mcp, bareKey)
		}
	}
	return out
}

// mcpDiscovered maps a discovered tool id to its parent Tool + tool name; it
// is populated at plan time from the live Tool objects the stack references.
type mcpDiscovered struct {
	ParentIdentity string
	Name           string
}

func symbolizeMCPTools(tools []any, mcpIndex map[string]mcpDiscovered, bareKey func(string) string) []any {
	type group struct {
		ref   string
		names []any
		extra map[string]any
	}
	var out []any
	groups := map[string]*group{}
	order := []string{}
	for _, e := range tools {
		em, ok := e.(map[string]any)
		if !ok || em["type"] != "mcp" {
			// http/code/function tools referenced by key: symbolize `key` → ref
			if ok {
				if k, _ := em["key"].(string); k != "" {
					ne := deepCopyMap(em)
					delete(ne, "key")
					delete(ne, "id")
					ne["ref"] = k
					out = append(out, ne)
					continue
				}
			}
			out = append(out, e)
			continue
		}
		id, _ := em["tool_id"].(string)
		disc, known := mcpIndex[id]
		if !known {
			out = append(out, e)
			continue
		}
		ref := bareKey(disc.ParentIdentity)
		g, exists := groups[ref]
		if !exists {
			g = &group{ref: ref, extra: map[string]any{}}
			groups[ref] = g
			order = append(order, ref)
			out = append(out, nil) // placeholder, filled below
		}
		g.names = append(g.names, disc.Name)
	}
	gi := 0
	for i, e := range out {
		if e != nil {
			continue
		}
		g := groups[order[gi]]
		gi++
		entry := map[string]any{"type": "mcp", "ref": g.ref, "tools": g.names}
		out[i] = entry
	}
	return out
}

