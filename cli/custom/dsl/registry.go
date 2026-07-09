package dsl

import (
	"fmt"
	"regexp"
	"strings"
)

// KindInfo is the per-kind API contract, extracted from the platform OpenAPI
// specs (orq-cli openapi.json + orquesta-web .openapi/v2/public) and verified
// against orquesta-web handler sources. See the design spec's kind table.
type KindInfo struct {
	Kind     string
	Plural   string // manifest directory name used by pull
	BasePath string // /v2/...
	Tier     int    // apply wave tier: 0 Project, 1 leaves, 2 referencing kinds

	// IdentityMode: "name" (Project), "key", or "display_name".
	IdentityMode string
	// GetByIdentity: GET/DELETE address by the identity value itself
	// (Agent key, MemoryStore key, Skill display_name). Others use server _id.
	GetByIdentity bool
	// ReadHasPath: GET/LIST responses include `path`. When false the declared
	// path lives only in stack state and is never diffed against live.
	ReadHasPath bool
	// IDField in read responses ("_id" default).
	IDField string
	// Wrapper key for GET-one responses ({"project": {...}}, {"skill": {...}}).
	Wrap string

	// Required spec fields on create (dotted paths, presence-checked offline).
	Required []string
	// Immutable spec paths — a change forces replace (delete + create).
	Immutable []string
	// Strip: server-computed fields dropped when normalizing live objects.
	Strip []string
	// Secret: dotted spec paths redacted to ${env.*} placeholders on pull.
	// A trailing ".*" segment means "every child key's `value` field".
	Secret []string
	// Gated: declared in the DSL but not writable via the public API.
	Gated string // non-empty = human explanation
}

var commonStrip = []string{
	"_id", "id", "created", "updated", "created_at", "updated_at",
	"created_by_id", "updated_by_id", "owner", "domain_id", "workspace_id",
	"project_id", "version", "status", "metrics", "type", "object",
}

// strip returns commonStrip minus keep, plus extra.
func strip(keep []string, extra ...string) []string {
	keepSet := map[string]bool{}
	for _, k := range keep {
		keepSet[k] = true
	}
	var out []string
	for _, s := range commonStrip {
		if !keepSet[s] {
			out = append(out, s)
		}
	}
	return append(out, extra...)
}

var registry = map[string]KindInfo{
	"Project": {
		Kind: "Project", Plural: "projects", BasePath: "/v2/projects", Tier: 0,
		IdentityMode: "name", IDField: "project_id", Wrap: "project",
		Strip: strip(nil, "is_archived", "is_default", "teams", "key"),
	},
	"Prompt": {
		Kind: "Prompt", Plural: "prompts", BasePath: "/v2/prompts", Tier: 1,
		IdentityMode: "display_name", ReadHasPath: false, IDField: "_id",
		Required: []string{"prompt.messages"},
		Strip:    strip(nil, "prompt_config", "display_name", "description", "metadata"),
	},
	"Agent": {
		Kind: "Agent", Plural: "agents", BasePath: "/v2/agents", Tier: 2,
		IdentityMode: "key", GetByIdentity: true, ReadHasPath: true, IDField: "_id",
		Required: []string{"role", "description", "instructions", "model", "settings"},
		Strip:    strip(nil, "source", "key", "display_name", "path"),
	},
	"Evaluator": {
		Kind: "Evaluator", Plural: "evaluators", BasePath: "/v2/evaluators", Tier: 1,
		IdentityMode: "key", ReadHasPath: false, IDField: "_id",
		Required: []string{"type"},
		Strip:    strip([]string{"type"}, "key", "description"),
	},
	"KnowledgeBase": {
		Kind: "KnowledgeBase", Plural: "knowledge-bases", BasePath: "/v2/knowledge", Tier: 1,
		IdentityMode: "key", ReadHasPath: true, IDField: "_id",
		Immutable: []string{"embedding_model"},
		Secret:    []string{"external_config.api_key"},
		Strip:     strip([]string{"type"}, "key", "description", "path", "model"),
	},
	"Dataset": {
		Kind: "Dataset", Plural: "datasets", BasePath: "/v2/datasets", Tier: 1,
		IdentityMode: "display_name", ReadHasPath: false, IDField: "_id",
		Strip: strip(nil, "display_name", "metadata"),
	},
	"Tool": {
		Kind: "Tool", Plural: "tools", BasePath: "/v2/tools", Tier: 1,
		IdentityMode: "key", ReadHasPath: true, IDField: "_id",
		Required: []string{"type", "description"},
		Secret:   []string{"mcp.headers.*"},
		Strip:    strip([]string{"type", "status"}, "key", "display_name", "path", "description"),
	},
	"MemoryStore": {
		Kind: "MemoryStore", Plural: "memory-stores", BasePath: "/v2/memory-stores", Tier: 1,
		IdentityMode: "key", GetByIdentity: true, ReadHasPath: false, IDField: "_id",
		Required:  []string{"description", "embedding_config"},
		Immutable: []string{"embedding_config"},
		Strip:     strip(nil, "key", "description"),
	},
	"Skill": {
		Kind: "Skill", Plural: "skills", BasePath: "/v2/skills", Tier: 1,
		IdentityMode: "display_name", GetByIdentity: true, ReadHasPath: true,
		IDField: "skill_id", Wrap: "skill",
		Strip:   strip(nil, "display_name", "path", "tags", "description", "skill_id"),
	},
	"Deployment": {
		Kind: "Deployment", Plural: "deployments", BasePath: "/v2/deployments", Tier: 2,
		IdentityMode: "key",
		Gated:        "Deployments have no public create/update API (platform ask #4); manage them in the UI for now",
	},
}

// Registry returns the kind contract table.
func Registry() map[string]KindInfo { return registry }

// Lookup resolves a manifest kind, with a helpful error for typos.
func Lookup(kind string) (KindInfo, error) {
	if info, ok := registry[kind]; ok {
		return info, nil
	}
	kinds := make([]string, 0, len(registry))
	for k := range registry {
		kinds = append(kinds, k)
	}
	return KindInfo{}, fmt.Errorf("unknown kind %q (kinds: %s)", kind, strings.Join(sorted(kinds), ", "))
}

// Identity field regexes per platform validation rules.
var (
	agentKeyRe       = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9]*([._-][A-Za-z0-9]+)*$`)
	memoryStoreKeyRe = regexp.MustCompile(`^[A-Za-z]([A-Za-z0-9]*([._][A-Za-z0-9]+)*)?$`) // no dashes
	skillNameRe      = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_]*$`)                      // letters/digits/underscores
)

// stateSkillPrefix marks the reserved Skill used as the stack state store.
const stateSkillPrefix = "orq_dsl_state_"

func sorted(s []string) []string {
	out := append([]string(nil), s...)
	for i := 1; i < len(out); i++ {
		for j := i; j > 0 && out[j] < out[j-1]; j-- {
			out[j], out[j-1] = out[j-1], out[j]
		}
	}
	return out
}
