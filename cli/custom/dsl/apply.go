package dsl

import (
	"fmt"
	"io"
	"net/url"
	"strings"
	"sync"
	"time"
)

// ErrPartialFailure: some resources failed; the run should exit non-zero but
// everything independent was still attempted (re-run converges).
var ErrPartialFailure = fmt.Errorf("apply finished with failures — re-run to converge")

// Execute applies a plan wave by wave. State saves after every successful
// operation, so an interrupted apply resumes cleanly.
func Execute(c *Client, p *PlanResult, out io.Writer, now func() time.Time) error {
	pal := colors(true)
	st := p.State
	stateID := p.StateID
	failed := map[string]bool{} // identity → failed (dependents skip)
	anyFailed := false

	saveState := func() {
		id, err := SaveState(c, st, stateID, p.Config.Defaults.Path)
		if err != nil {
			fmt.Fprintf(out, "  %s! state save failed: %v%s\n", pal.del, err, pal.reset)
			anyFailed = true
			return
		}
		stateID = id
		p.StateID = id
	}

	// Adopt unchanged live resources into state first (no API writes).
	if len(p.Adoptions) > 0 {
		for _, ch := range p.Adoptions {
			st.Upsert(newStateResource(ch, ch.LiveID, now))
			fmt.Fprintf(out, "%sadopted%s  %s  %sunchanged, now stack-owned%s\n", pal.dim, pal.reset, ch.Identity, pal.dim, pal.reset)
		}
		saveState()
	}

	for wi, wave := range p.Waves {
		type result struct {
			ch     Change
			err    error
			msg    string
			upsert *StateResource // state mutation, applied sequentially below
			remove bool
		}
		results := make([]result, len(wave))
		var wg sync.WaitGroup
		sem := make(chan struct{}, 4)
		for i, ch := range wave {
			wg.Add(1)
			go func(i int, ch Change) {
				defer wg.Done()
				sem <- struct{}{}
				defer func() { <-sem }()
				if dep := failedDependency(ch, failed); dep != "" {
					results[i] = result{ch: ch, err: errSkipped, msg: "dependency failed: " + dep}
					return
				}
				msg, upsert, remove, err := executeChange(c, p, ch, now)
				results[i] = result{ch: ch, err: err, msg: msg, upsert: upsert, remove: remove}
			}(i, ch)
		}
		wg.Wait()

		// Sequential post-processing: state mutations and saves stay ordered
		// and race-free (workers never touch StateDoc).
		for _, r := range results {
			glyph, color := opGlyph(r.ch.Op, pal)
			switch {
			case r.err == errSkipped:
				fmt.Fprintf(out, "%swave %d%s  ↷ %s  %sskipped (%s)%s\n", pal.dim, wi+1, pal.reset, r.ch.Identity, pal.dim, r.msg, pal.reset)
				failed[r.ch.Identity] = true
				anyFailed = true
			case r.err != nil:
				fmt.Fprintf(out, "%swave %d%s  %s✗ %s%s  %v\n", pal.dim, wi+1, pal.reset, pal.del, r.ch.Identity, pal.reset, r.err)
				failed[r.ch.Identity] = true
				anyFailed = true
				if r.remove { // replace: old deleted, new failed
					st.Remove(r.ch.Kind, r.ch.Identity)
					saveState()
				}
			default:
				fmt.Fprintf(out, "%swave %d%s  %s%s %s%s  %s%s%s\n", pal.dim, wi+1, pal.reset, color, glyph, r.ch.Identity, pal.reset, pal.dim, r.msg, pal.reset)
				if r.upsert != nil {
					st.Upsert(*r.upsert)
				} else if r.ch.Op == OpDelete {
					st.Remove(r.ch.Kind, r.ch.Identity)
				}
				saveState()
			}
		}
	}
	if anyFailed {
		return ErrPartialFailure
	}
	return nil
}

var errSkipped = fmt.Errorf("skipped")

// failedDependency returns the identity of a failed dependency, if any.
func failedDependency(ch Change, failed map[string]bool) string {
	if len(failed) == 0 || ch.Manifest == nil {
		return ""
	}
	for _, ref := range extractRefs(ch.Manifest) {
		for id := range failed {
			if strings.HasPrefix(id, ref.Kind+"/") && strings.HasSuffix(id, "/"+ref.Key) || id == ref.Kind+"/"+ref.Key {
				return id
			}
		}
	}
	if ch.Kind != "Project" {
		proj, _, _ := strings.Cut(ch.Manifest.Metadata.Path, "/")
		if failed["Project/"+proj] {
			return "Project/" + proj
		}
	}
	return ""
}

func opGlyph(op Op, pal palette) (string, string) {
	switch op {
	case OpCreate:
		return "+", pal.add
	case OpUpdate:
		return "~", pal.mod
	case OpDelete:
		return "−", pal.del
	case OpReplace:
		return "±", pal.rep
	}
	return "·", pal.dim
}

// executeChange performs one operation. It returns the state mutation for
// the caller to apply sequentially: an upsert record, or remove=true when the
// (kind, identity) entry must be dropped even on failure (replace half-done).
// It must NOT touch p.State itself — it runs concurrently within a wave.
func executeChange(c *Client, p *PlanResult, ch Change, now func() time.Time) (string, *StateResource, bool, error) {
	info, err := Lookup(ch.Kind)
	if err != nil {
		return "", nil, false, err
	}
	start := now()

	switch ch.Op {
	case OpCreate:
		id, err := createResource(c, p, ch, info)
		if err != nil {
			return "", nil, false, err
		}
		r := newStateResource(ch, id, now)
		return fmt.Sprintf("created %s (%s)", id, since(start, now)), &r, false, nil

	case OpUpdate:
		if err := updateResource(c, p, ch, info); err != nil {
			return "", nil, false, err
		}
		r := newStateResource(ch, ch.LiveID, now)
		return fmt.Sprintf("updated (%s)", since(start, now)), &r, false, nil

	case OpReplace:
		if err := deleteResource(c, info, ch.LiveID, ch); err != nil && !IsNotFound(err) {
			return "", nil, false, fmt.Errorf("replace/delete: %w", err)
		}
		id, err := createResource(c, p, ch, info)
		if err != nil {
			return "", nil, true, fmt.Errorf("replace/create: %w", err)
		}
		r := newStateResource(ch, id, now)
		return fmt.Sprintf("replaced → %s (%s)", id, since(start, now)), &r, false, nil

	case OpDelete:
		if err := deleteResource(c, info, ch.LiveID, ch); err != nil && !IsNotFound(err) {
			return "", nil, false, err
		}
		return fmt.Sprintf("deleted (%s)", since(start, now)), nil, false, nil
	}
	return "", nil, false, nil
}

func newStateResource(ch Change, id string, now func() time.Time) StateResource {
	path := ""
	hash := ""
	if ch.Manifest != nil {
		path = ch.Manifest.Metadata.Path
		hash = SpecHash(ch.Manifest)
	}
	return StateResource{
		Kind: ch.Kind, Identity: ch.Identity, ServerID: id,
		Path: path, SpecHash: hash, AppliedAt: now().UTC().Format(time.RFC3339),
	}
}

// createResource builds the create body: resolved spec + identity + path.
func createResource(c *Client, p *PlanResult, ch Change, info KindInfo) (string, error) {
	m := ch.Manifest
	spec, err := ResolveRefs(m, p.Resolver)
	if err != nil {
		return "", err
	}
	body := spec
	switch info.IdentityMode {
	case "name":
		body["name"] = m.Metadata.Name
	case "key":
		body["key"] = m.Metadata.Key
	case "display_name":
		body["display_name"] = m.Metadata.DisplayName
	}
	if m.Metadata.DisplayName != "" && info.IdentityMode == "key" {
		body["display_name"] = m.Metadata.DisplayName
	}
	if ch.Kind != "Project" {
		body["path"] = m.Metadata.Path
	}

	var raw map[string]any
	if err := c.Do("POST", info.BasePath, body, &raw); err != nil {
		return "", err
	}
	obj := unwrap(raw, info)
	id := extractID(obj, info)
	if id == "" && info.GetByIdentity {
		id = m.IdentityValue()
	}
	if id == "" {
		// Fall back to rediscovery so state never stores an empty id.
		live, foundID, ferr := FetchLive(c, m, info, p.State)
		if ferr == nil && live != nil {
			id = foundID
		}
	}
	if id == "" {
		return "", fmt.Errorf("create succeeded but no id in response")
	}
	p.Resolver.put(ch.Kind, m.IdentityValue(), id)
	// Fresh MCP tools expose discovered sub-tools only after create.
	if ch.Kind == "Tool" {
		registerMCPDiscovery(c, p, m, info, id)
	}
	return id, nil
}

func registerMCPDiscovery(c *Client, p *PlanResult, m *Manifest, info KindInfo, id string) {
	if t, _ := m.Spec["type"].(string); t != "mcp" {
		return
	}
	obj, err := getOne(c, info, id)
	if err != nil || obj == nil {
		return
	}
	discovered := map[string]string{}
	if mcp, ok := obj["mcp"].(map[string]any); ok {
		if tools, ok := mcp["tools"].([]any); ok {
			for _, t := range tools {
				if tm, ok := t.(map[string]any); ok {
					name, _ := tm["name"].(string)
					tid, _ := tm["id"].(string)
					if name != "" && tid != "" {
						discovered[name] = tid
					}
				}
			}
		}
	}
	p.Resolver.putMCP(m.IdentityValue(), discovered)
}

// updateResource PATCHes only the changed top-level fields (managed fields),
// resolving refs first. Envelope-path changes send `path`; display_name
// drift sends display_name.
func updateResource(c *Client, p *PlanResult, ch Change, info KindInfo) error {
	m := ch.Manifest
	spec, err := ResolveRefs(m, p.Resolver)
	if err != nil {
		return err
	}
	body := map[string]any{}
	topLevel := map[string]bool{}
	for _, path := range ch.Paths {
		switch path {
		case "metadata.path":
			body["path"] = m.Metadata.Path
		case "metadata.display_name":
			body["display_name"] = m.Metadata.DisplayName
		default:
			top, _, _ := strings.Cut(path, ".")
			topLevel[top] = true
		}
	}
	for top := range topLevel {
		if v, ok := spec[top]; ok {
			body[top] = v
		}
	}
	// Evaluator update quirk: the flat PATCH schema wants the discriminator.
	if ch.Kind == "Evaluator" {
		if t, ok := m.Spec["type"]; ok {
			body["type"] = t
		}
	}
	return c.Do("PATCH", info.BasePath+"/"+url.PathEscape(updateTarget(ch, info)), body, nil)
}

// updateTarget: key-addressed kinds PATCH by identity value, others by id.
func updateTarget(ch Change, info KindInfo) string {
	if info.GetByIdentity && ch.Manifest != nil {
		return ch.Manifest.IdentityValue()
	}
	return ch.LiveID
}

func deleteResource(c *Client, info KindInfo, liveID string, ch Change) error {
	target := liveID
	if info.GetByIdentity && target == "" && ch.Manifest != nil {
		target = ch.Manifest.IdentityValue()
	}
	if target == "" {
		return nil // nothing to delete
	}
	return c.Do("DELETE", info.BasePath+"/"+url.PathEscape(target), nil, nil)
}

func since(start time.Time, now func() time.Time) string {
	return now().Sub(start).Round(time.Millisecond).String()
}

// DestroyPlan builds a pure-delete plan from state (reverse tier order).
func DestroyPlan(cfg StackConfig, st *StateDoc, stateID string) (*PlanResult, error) {
	p := &PlanResult{Stack: cfg.Stack, Config: cfg, State: st, StateID: stateID, Resolver: newRefResolver()}
	if st == nil || len(st.Resources) == 0 {
		return p, nil
	}
	var changes []Change
	for _, r := range st.Resources {
		changes = append(changes, Change{Op: OpDelete, Kind: r.Kind, Identity: r.Identity, LiveID: r.ServerID})
	}
	waves, err := BuildWaves(changes)
	if err != nil {
		return nil, err
	}
	p.Waves = waves
	p.Deletes = len(changes)
	return p, nil
}
