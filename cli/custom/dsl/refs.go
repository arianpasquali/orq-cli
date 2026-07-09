package dsl

import (
	"fmt"
	"sort"
	"strings"
)

// refTargets extracts the same-stack dependencies of a manifest as bare keys
// grouped by the kind they must resolve against.
type refTarget struct {
	Kind string // target kind
	Key  string // bare identity value (key / display_name)
	Site string // dotted location, for error messages
}

// extractRefs walks a manifest and returns its dependency edges. The mapping
// of ref sites to target kinds is part of the kind contract:
//
//	Agent.knowledge_bases[].ref        → KnowledgeBase
//	Agent.settings.guardrails[].ref    → Evaluator
//	Agent.settings.evaluators[].ref    → Evaluator
//	Agent.settings.tools[].ref         → Tool (http/code/function/json_schema/mcp)
//	Agent.memory_stores[] (strings)    → MemoryStore
//	Agent.team_of_agents[].key         → Agent (same-kind edge)
//
// Other kinds hold no refs in v1.
func extractRefs(m *Manifest) []refTarget {
	if m.Kind != "Agent" {
		return nil
	}
	var refs []refTarget
	add := func(kind, key, site string) {
		if key != "" {
			refs = append(refs, refTarget{Kind: kind, Key: key, Site: site})
		}
	}

	if kbs, ok := m.Spec["knowledge_bases"].([]any); ok {
		for i, e := range kbs {
			if em, ok := e.(map[string]any); ok {
				if ref, _ := em["ref"].(string); ref != "" {
					add("KnowledgeBase", ref, fmt.Sprintf("knowledge_bases[%d]", i))
				}
			}
		}
	}
	if stores, ok := m.Spec["memory_stores"].([]any); ok {
		for i, e := range stores {
			if s, ok := e.(string); ok {
				add("MemoryStore", s, fmt.Sprintf("memory_stores[%d]", i))
			}
		}
	}
	if team, ok := m.Spec["team_of_agents"].([]any); ok {
		for i, e := range team {
			if em, ok := e.(map[string]any); ok {
				if k, _ := em["key"].(string); k != "" {
					add("Agent", k, fmt.Sprintf("team_of_agents[%d]", i))
				}
			}
		}
	}
	if settings, ok := m.Spec["settings"].(map[string]any); ok {
		for _, field := range []string{"guardrails", "evaluators"} {
			if items, ok := settings[field].([]any); ok {
				for i, e := range items {
					if em, ok := e.(map[string]any); ok {
						if ref, _ := em["ref"].(string); ref != "" {
							add("Evaluator", ref, fmt.Sprintf("settings.%s[%d]", field, i))
						}
					}
				}
			}
		}
		if tools, ok := settings["tools"].([]any); ok {
			for i, e := range tools {
				if em, ok := e.(map[string]any); ok {
					if ref, _ := em["ref"].(string); ref != "" {
						add("Tool", ref, fmt.Sprintf("settings.tools[%d]", i))
					}
				}
			}
		}
	}
	return refs
}

// BuildWaves orders non-noop changes into dependency tiers:
// creates/updates/replaces flow tier 0 → 2 with explicit ref edges inside a
// tier; deletes run afterwards in reverse tier order. Same-stack refs to
// manifests that are not in the plan (noop) are satisfied by definition.
func BuildWaves(changes []Change) ([][]Change, error) {
	var work, deletes []Change
	for _, ch := range changes {
		switch ch.Op {
		case OpNoop:
			continue
		case OpDelete:
			deletes = append(deletes, ch)
		default:
			work = append(work, ch)
		}
	}

	// Node lookup by "Kind/bareKey" for edge building.
	index := map[string]int{}
	for i, ch := range work {
		index[ch.Kind+"/"+bareIdentityValue(ch)] = i
	}

	adj := make(map[int][]int, len(work))  // node → dependents
	indeg := make([]int, len(work))
	tier := func(ch Change) int {
		info, _ := Lookup(ch.Kind)
		return info.Tier
	}
	for i, ch := range work {
		if ch.Manifest == nil {
			continue
		}
		for _, ref := range extractRefs(ch.Manifest) {
			if j, ok := index[ref.Kind+"/"+ref.Key]; ok && j != i {
				adj[j] = append(adj[j], i)
				indeg[i]++
			}
		}
		// Implicit edge: every non-Project depends on a Project node whose
		// name is the first path segment.
		if ch.Kind != "Project" {
			proj, _, _ := strings.Cut(ch.Manifest.Metadata.Path, "/")
			if j, ok := index["Project/"+proj]; ok && j != i {
				adj[j] = append(adj[j], i)
				indeg[i]++
			}
		}
	}

	// Kahn with (tier, identity) priority for deterministic waves.
	remaining := len(work)
	done := make([]bool, len(work))
	var waves [][]Change
	for remaining > 0 {
		var ready []int
		for i := range work {
			if !done[i] && indeg[i] == 0 {
				ready = append(ready, i)
			}
		}
		if len(ready) == 0 {
			var stuck []string
			for i := range work {
				if !done[i] {
					stuck = append(stuck, work[i].Identity)
				}
			}
			return nil, fmt.Errorf("dependency cycle among: %s", strings.Join(stuck, ", "))
		}
		sort.Slice(ready, func(a, b int) bool {
			ta, tb := tier(work[ready[a]]), tier(work[ready[b]])
			if ta != tb {
				return ta < tb
			}
			return work[ready[a]].Identity < work[ready[b]].Identity
		})
		// A wave = all ready nodes of the minimum tier present.
		minTier := tier(work[ready[0]])
		var wave []Change
		for _, i := range ready {
			if tier(work[i]) != minTier {
				break
			}
			wave = append(wave, work[i])
			done[i] = true
			remaining--
			for _, dep := range adj[i] {
				indeg[dep]--
			}
		}
		waves = append(waves, wave)
	}

	if len(deletes) > 0 {
		sort.Slice(deletes, func(a, b int) bool {
			ta, tb := tier(deletes[a]), tier(deletes[b])
			if ta != tb {
				return ta > tb // reverse: referencing kinds first
			}
			return deletes[a].Identity < deletes[b].Identity
		})
		// Group consecutive same-tier deletes into waves.
		var wave []Change
		last := -1
		for _, d := range deletes {
			if last != -1 && tier(d) != last {
				waves = append(waves, wave)
				wave = nil
			}
			last = tier(d)
			wave = append(wave, d)
		}
		waves = append(waves, wave)
	}
	return waves, nil
}

// bareIdentityValue: the ref-addressable value of a change's identity.
func bareIdentityValue(ch Change) string {
	if ch.Manifest != nil {
		return ch.Manifest.IdentityValue()
	}
	_, rest, _ := strings.Cut(ch.Identity, "/")
	if i := strings.LastIndex(rest, "|"); i >= 0 {
		return rest[i+1:]
	}
	return rest
}

// refResolver resolves "Kind/key" → server id at apply time.
type refResolver struct {
	ids map[string]string // "Kind/bareKey" → server id
	mcp map[string]map[string]string // Tool bareKey → discovered tool name → id
}

func newRefResolver() *refResolver {
	return &refResolver{ids: map[string]string{}, mcp: map[string]map[string]string{}}
}

func (r *refResolver) put(kind, bareKey, id string) { r.ids[kind+"/"+bareKey] = id }

func (r *refResolver) get(kind, bareKey string) (string, error) {
	if id, ok := r.ids[kind+"/"+bareKey]; ok && id != "" {
		return id, nil
	}
	return "", fmt.Errorf("unresolved ref: no %s with key %q in stack or workspace", kind, bareKey)
}

// putMCP records the discovered tools of an MCP Tool entity.
func (r *refResolver) putMCP(bareKey string, discovered map[string]string) { r.mcp[bareKey] = discovered }

// ResolveRefs rewrites a manifest spec's symbolic refs into the API's id
// shapes. Returns a deep copy; the manifest itself keeps its refs.
func ResolveRefs(m *Manifest, r *refResolver) (map[string]any, error) {
	spec := deepCopyMap(m.Spec)
	if m.Kind != "Agent" {
		return spec, nil
	}

	if kbs, ok := spec["knowledge_bases"].([]any); ok {
		for i, e := range kbs {
			em, ok := e.(map[string]any)
			if !ok {
				continue
			}
			ref, _ := em["ref"].(string)
			if ref == "" {
				continue
			}
			id, err := r.get("KnowledgeBase", ref)
			if err != nil {
				return nil, fmt.Errorf("%s: knowledge_bases[%d]: %w", m.Identity(), i, err)
			}
			kbs[i] = map[string]any{"knowledge_id": id}
		}
	}

	if settings, ok := spec["settings"].(map[string]any); ok {
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
				ref, _ := em["ref"].(string)
				if ref == "" {
					continue
				}
				id, err := r.get("Evaluator", ref)
				if err != nil {
					return nil, fmt.Errorf("%s: settings.%s[%d]: %w", m.Identity(), field, i, err)
				}
				ne := deepCopyMap(em)
				delete(ne, "ref")
				ne["id"] = id
				items[i] = ne
			}
		}

		if tools, ok := settings["tools"].([]any); ok {
			var out []any
			for i, e := range tools {
				em, ok := e.(map[string]any)
				if !ok {
					out = append(out, e)
					continue
				}
				ref, _ := em["ref"].(string)
				typ, _ := em["type"].(string)
				if ref == "" {
					out = append(out, e)
					continue
				}
				if typ == "mcp" {
					names, _ := em["tools"].([]any)
					discovered, ok := r.mcp[ref]
					if !ok {
						return nil, fmt.Errorf("%s: settings.tools[%d]: mcp tool %q is not applied yet or has no discovered tools", m.Identity(), i, ref)
					}
					if len(names) == 0 { // no allowlist → attach all discovered
						for name := range discovered {
							names = append(names, name)
						}
						sort.Slice(names, func(a, b int) bool { return names[a].(string) < names[b].(string) })
					}
					for _, n := range names {
						name, _ := n.(string)
						id, ok := discovered[name]
						if !ok {
							return nil, fmt.Errorf("%s: settings.tools[%d]: mcp tool %q has no discovered tool named %q (available: %s)",
								m.Identity(), i, ref, name, strings.Join(sortedKeys(discovered), ", "))
						}
						out = append(out, map[string]any{"type": "mcp", "tool_id": id})
					}
					continue
				}
				// http/code/function/json_schema reference workspace tools by key.
				ne := deepCopyMap(em)
				delete(ne, "ref")
				ne["key"] = ref
				out = append(out, ne)
			}
			settings["tools"] = out
		}
	}
	return spec, nil
}

func sortedKeys(m map[string]string) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
