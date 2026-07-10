package dsl

import (
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
)

// PlanResult is a computed reconciliation plan.
type PlanResult struct {
	Stack    string
	Config   StackConfig
	Waves    [][]Change
	Live     int // manifests with a live counterpart
	State    *StateDoc
	StateID  string
	Resolver *refResolver
	// Adoptions: unchanged live resources not yet in state; apply records
	// them (state-only writes, no API mutation).
	Adoptions []Change

	Creates, Updates, Deletes, Replaces int
}

// HasChanges reports whether apply would do anything.
func (p *PlanResult) HasChanges() bool {
	return p.Creates+p.Updates+p.Deletes+p.Replaces > 0
}

// ErrChangesPending distinguishes "plan found changes" (exit 2) from errors.
var ErrChangesPending = fmt.Errorf("changes pending")

// BuildPlan assembles the full plan: fetch live state for every manifest,
// classify, compute deletes from state, and topo-order the result.
func BuildPlan(ms []Manifest, cfg StackConfig, c *Client, st *StateDoc, stateID string) (*PlanResult, error) {
	res := &PlanResult{Stack: cfg.Stack, State: st, StateID: stateID, Resolver: newRefResolver()}
	if st == nil {
		res.State = &StateDoc{Version: 1, Stack: cfg.Stack}
	}

	type fetched struct {
		idx  int
		live map[string]any
		id   string
		err  error
	}
	results := make([]fetched, len(ms))
	sem := make(chan struct{}, 4)
	var wg sync.WaitGroup
	for i := range ms {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			info, err := Lookup(ms[i].Kind)
			if err != nil {
				results[i] = fetched{idx: i, err: err}
				return
			}
			live, id, err := FetchLive(c, &ms[i], info, res.State)
			results[i] = fetched{idx: i, live: live, id: id, err: err}
		}(i)
	}
	wg.Wait()

	idMap := res.State.IDToIdentity()
	mcpIdx := map[string]mcpDiscovered{}
	// Register live ids with the resolver + collect discovered MCP tools.
	for i, f := range results {
		if f.err != nil {
			return nil, fmt.Errorf("%s: %w", ms[i].Identity(), f.err)
		}
		if f.id != "" {
			res.Resolver.put(ms[i].Kind, ms[i].IdentityValue(), f.id)
			idMap[f.id] = ms[i].Identity()
		}
		if ms[i].Kind == "Tool" && f.live != nil {
			if mcp, ok := f.live["mcp"].(map[string]any); ok {
				discovered := map[string]string{}
				if tools, ok := mcp["tools"].([]any); ok {
					for _, t := range tools {
						if tm, ok := t.(map[string]any); ok {
							name, _ := tm["name"].(string)
							id, _ := tm["id"].(string)
							if name != "" && id != "" {
								discovered[name] = id
								mcpIdx[id] = mcpDiscovered{ParentIdentity: ms[i].Identity(), Name: name}
							}
						}
					}
				}
				res.Resolver.putMCP(ms[i].IdentityValue(), discovered)
			}
		}
	}

	// Classify every manifest.
	var changes []Change
	inManifests := map[string]bool{}
	for i := range ms {
		m := &ms[i]
		inManifests[m.Kind+"/"+m.Identity()] = true
		info, _ := Lookup(m.Kind)
		f := results[i]
		statePath := ""
		if r := res.State.Find(m.Kind, m.Identity()); r != nil {
			statePath = r.Path
		}
		live := f.live
		if live != nil {
			res.Live++
			live = symbolizeLive(live, m.Kind, idMap, mcpIdx)
		}
		changes = append(changes, Classify(m, live, f.id, info, statePath))
	}

	// Deletes: state resources whose identity vanished from the manifests.
	for _, r := range res.State.Resources {
		if !inManifests[r.Kind+"/"+r.Identity] {
			changes = append(changes, Change{Op: OpDelete, Kind: r.Kind, Identity: r.Identity, LiveID: r.ServerID})
		}
	}

	// Same-stack refs must resolve: either to a manifest in the stack or to a
	// live workspace resource already registered with the resolver.
	for i := range ms {
		for _, ref := range extractRefs(&ms[i]) {
			if res.Resolver.has(ref.Kind, ref.Key) {
				continue
			}
			found := false
			for j := range ms {
				if ms[j].Kind == ref.Kind && ms[j].IdentityValue() == ref.Key {
					found = true
					break
				}
			}
			if !found {
				// Last chance: live workspace lookup by identity.
				info, _ := Lookup(ref.Kind)
				probe := Manifest{Kind: ref.Kind}
				switch info.IdentityMode {
				case "key":
					probe.Metadata.Key = ref.Key
				case "display_name":
					probe.Metadata.DisplayName = ref.Key
				default:
					probe.Metadata.Name = ref.Key
				}
				live, id, err := FetchLive(c, &probe, info, res.State)
				if err != nil {
					return nil, err
				}
				if live == nil {
					return nil, fmt.Errorf("%s: %s: unresolved ref — no %s %q in this stack or the live workspace",
						ms[i].Identity(), ref.Site, ref.Kind, ref.Key)
				}
				res.Resolver.put(ref.Kind, ref.Key, id)
			}
		}
	}

	// Adoption: live resources that match their manifest (noop) but are
	// missing from state get recorded at apply time — this is how a pulled
	// workspace becomes stack-owned without any API write.
	for _, ch := range changes {
		if ch.Op == OpNoop && ch.LiveID != "" && res.State.Find(ch.Kind, ch.Identity) == nil {
			res.Adoptions = append(res.Adoptions, ch)
		}
	}

	waves, err := BuildWaves(changes)
	if err != nil {
		return nil, err
	}
	res.Waves = waves
	for _, wave := range waves {
		for _, ch := range wave {
			switch ch.Op {
			case OpCreate:
				res.Creates++
			case OpUpdate:
				res.Updates++
			case OpDelete:
				res.Deletes++
			case OpReplace:
				res.Replaces++
			}
		}
	}
	return res, nil
}

// --- rendering ---

type palette struct{ add, mod, del, rep, dim, head, reset string }

func colors(enabled bool) palette {
	if !enabled || os.Getenv("NO_COLOR") != "" {
		return palette{}
	}
	return palette{
		add: "\033[32m", mod: "\033[33m", del: "\033[31m", rep: "\033[35m",
		dim: "\033[2m", head: "\033[1m", reset: "\033[0m",
	}
}

// RenderPlan prints the human plan, grouped into the same execution waves
// apply will run — the wave gutter IS the dependency graph, topologically
// sorted. Each resource that references others gets a dim "needs" line.
func RenderPlan(w io.Writer, p *PlanResult, color bool) {
	pal := colors(color)
	rev := 0
	if p.State != nil {
		rev = p.State.Revision
	}
	fmt.Fprintf(w, "%sstack: %s · %d live · state rev %d%s\n\n", pal.dim, p.Stack, p.Live, rev, pal.reset)

	if !p.HasChanges() {
		fmt.Fprintf(w, "%sNo changes. Workspace matches the manifests.%s\n", pal.head, pal.reset)
		return
	}
	for wi, wave := range p.Waves {
		label := fmt.Sprintf("wave %d", wi+1)
		gutter := label
		indent := strings.Repeat(" ", len(label)+4)
		for _, ch := range wave {
			if ch.Op == OpNoop {
				continue
			}
			glyph, c := opGlyph(ch.Op, pal)
			suffix := ""
			switch ch.Op {
			case OpDelete:
				suffix = fmt.Sprintf("  %sremoved from files · owned by stack%s", pal.dim, pal.reset)
			case OpReplace:
				suffix = fmt.Sprintf("  %s%s → delete + create%s", pal.dim, ch.Reason, pal.reset)
			}
			fmt.Fprintf(w, "%s%s%s  %s%s %s%s%s\n", pal.dim, gutter, pal.reset, c, glyph, ch.Identity, pal.reset, suffix)
			gutter = strings.Repeat(" ", len(label))
			for _, path := range ch.Paths {
				fmt.Fprintf(w, "%s%s%s%s\n", indent, pal.dim, path, pal.reset)
			}
			if deps := changeDeps(ch); len(deps) > 0 {
				fmt.Fprintf(w, "%s%s└─ needs %s%s\n", indent, pal.dim, strings.Join(deps, " · "), pal.reset)
			}
		}
	}
	fmt.Fprintf(w, "\n%sPlan: %d to create, %d to update, %d to delete, %d to replace.%s\n",
		pal.head, p.Creates, p.Updates, p.Deletes, p.Replaces, pal.reset)
}

// changeDeps lists the distinct in-manifest references of a change, in
// Kind/key form, for the plan's "needs" annotation.
func changeDeps(ch Change) []string {
	if ch.Manifest == nil {
		return nil
	}
	seen := map[string]bool{}
	var out []string
	for _, r := range extractRefs(ch.Manifest) {
		id := r.Kind + "/" + r.Key
		if !seen[id] {
			seen[id] = true
			out = append(out, id)
		}
	}
	return out
}

