package dsl

import (
	"encoding/json"
	"fmt"
	"strings"
)

// Stack state lives server-side in a reserved Skill entity. Why a Skill: the
// public API has no writable key/value store (RemoteConfigs are read-only),
// but Skills have a workspace-unique, DB-enforced display_name, are directly
// addressable by that name (GET /v2/skills/{name}), and their `instructions`
// string PATCHes in place — everything a small JSON state document needs.
// Platform ask #2 (native /v2/stacks) replaces this.

// StateResource is one stack-owned resource: the identity→server-id mapping
// plus the declared path (write-only on several kinds) and the spec hash of
// the last applied manifest.
type StateResource struct {
	Kind      string `json:"kind"`
	Identity  string `json:"identity"` // Manifest.Identity()
	ServerID  string `json:"server_id"`
	Path      string `json:"path"`
	SpecHash  string `json:"spec_hash"`
	AppliedAt string `json:"applied_at"` // RFC3339; informational
}

// StateDoc is the JSON document stored in the state skill's instructions.
type StateDoc struct {
	Version   int             `json:"version"`
	Stack     string          `json:"stack"`
	Revision  int             `json:"revision"`
	Resources []StateResource `json:"resources"`
}

// ErrStateConflict: someone else applied between our read and write.
type ErrStateConflict struct{ Live, Parent int }

func (e *ErrStateConflict) Error() string {
	return fmt.Sprintf("state conflict: another apply moved the stack to revision %d (this run started from %d) — re-run to reconcile", e.Live, e.Parent)
}

func stateSkillName(stack string) string {
	return stateSkillPrefix + strings.ReplaceAll(stack, "-", "_")
}

type skillWrap struct {
	Skill map[string]any `json:"skill"`
}

// LoadState fetches the stack's state skill. A missing skill is not an error:
// it means the stack has never been applied (nil doc, empty id).
func LoadState(c *Client, stack string) (*StateDoc, string, error) {
	var wrap skillWrap
	err := c.Do("GET", "/v2/skills/"+stateSkillName(stack), nil, &wrap)
	if err != nil {
		if IsNotFound(err) {
			return nil, "", nil
		}
		return nil, "", fmt.Errorf("load state: %w", err)
	}
	sk := wrap.Skill
	if sk == nil {
		return nil, "", fmt.Errorf("load state: unexpected response shape")
	}
	id, _ := sk["skill_id"].(string)
	instructions, _ := sk["instructions"].(string)
	var doc StateDoc
	if err := json.Unmarshal([]byte(instructions), &doc); err != nil {
		return nil, id, fmt.Errorf("load state: skill %s holds invalid JSON (%v) — repair or delete it", stateSkillName(stack), err)
	}
	return &doc, id, nil
}

// SaveState persists doc, bumping its revision. skillID=="" creates the
// skill (first apply); otherwise an advisory guard re-reads the live revision
// and refuses to clobber a concurrent writer, then PATCHes in place.
// Returns the skill id (fresh on create).
func SaveState(c *Client, doc *StateDoc, skillID string, defaultPath string) (string, error) {
	parent := doc.Revision
	doc.Revision++
	body, err := json.MarshalIndent(doc, "", " ")
	if err != nil {
		return skillID, err
	}

	if skillID == "" {
		var wrap skillWrap
		payload := map[string]any{
			"display_name": stateSkillName(doc.Stack),
			"path":         defaultPath,
			"description":  "orq stack state — managed by `orq stack`, do not edit",
			"instructions": string(body),
		}
		if err := c.Do("POST", "/v2/skills", payload, &wrap); err != nil {
			doc.Revision = parent
			return "", fmt.Errorf("create state skill: %w", err)
		}
		id, _ := wrap.Skill["skill_id"].(string)
		if id == "" {
			// Some create responses return the object unwrapped; retry via GET.
			liveDoc, liveID, err := LoadState(c, doc.Stack)
			if err != nil || liveDoc == nil {
				return "", fmt.Errorf("create state skill: response missing skill_id")
			}
			id = liveID
		}
		return id, nil
	}

	live, _, err := LoadState(c, doc.Stack)
	if err != nil {
		doc.Revision = parent
		return skillID, err
	}
	if live != nil && live.Revision != parent {
		doc.Revision = parent
		return skillID, &ErrStateConflict{Live: live.Revision, Parent: parent}
	}
	if err := c.Do("PATCH", "/v2/skills/"+skillID, map[string]any{"instructions": string(body)}, nil); err != nil {
		doc.Revision = parent
		return skillID, fmt.Errorf("save state: %w", err)
	}
	return skillID, nil
}

// DeleteState removes the state skill (destroy's last step).
func DeleteState(c *Client, skillID string) error {
	if skillID == "" {
		return nil
	}
	err := c.Do("DELETE", "/v2/skills/"+skillID, nil, nil)
	if err != nil && !IsNotFound(err) {
		return err
	}
	return nil
}

// Find returns the state entry for (kind, identity), or nil.
func (s *StateDoc) Find(kind, identity string) *StateResource {
	if s == nil {
		return nil
	}
	for i := range s.Resources {
		if s.Resources[i].Kind == kind && s.Resources[i].Identity == identity {
			return &s.Resources[i]
		}
	}
	return nil
}

// Upsert inserts or replaces the entry keyed by (kind, identity).
func (s *StateDoc) Upsert(r StateResource) {
	for i := range s.Resources {
		if s.Resources[i].Kind == r.Kind && s.Resources[i].Identity == r.Identity {
			s.Resources[i] = r
			return
		}
	}
	s.Resources = append(s.Resources, r)
}

// Remove deletes the entry keyed by (kind, identity).
func (s *StateDoc) Remove(kind, identity string) {
	for i := range s.Resources {
		if s.Resources[i].Kind == kind && s.Resources[i].Identity == identity {
			s.Resources = append(s.Resources[:i], s.Resources[i+1:]...)
			return
		}
	}
}

// IDToIdentity builds the reverse map server_id → identity, used to
// symbolize live refs back into `ref:` form before diffing.
func (s *StateDoc) IDToIdentity() map[string]string {
	out := map[string]string{}
	if s == nil {
		return out
	}
	for _, r := range s.Resources {
		if r.ServerID != "" {
			out[r.ServerID] = r.Identity
		}
	}
	return out
}
