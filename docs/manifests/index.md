# Manifest reference

One resource per YAML document. Multi-document files are allowed; the convention is one
directory per kind, one file per resource (`agents/eng-companion.yaml`). Any layout
under the stack directory works — the loader walks recursively (skipping `orq.yaml`,
`vars/`, and dot-directories).

## The envelope

```yaml
apiVersion: orq.ai/v1        # exactly this, always
kind: Agent                  # one of the nine kinds below
metadata:
  key: eng-companion         # identity field — which one depends on the kind
  path: acme/agents          # project[/folder/...]; folders auto-created
  display_name: Companion    # optional label on key-identified kinds
spec:
  ...                        # v2 API create/update body, verbatim
```

- `apiVersion` must be `orq.ai/v1`.
- `metadata.path` defaults to `defaults.path` from `orq.yaml` when omitted (required
  one way or the other for every kind except Project). Its first segment names the
  project.
- Server ids (`_id`, `id`) never appear in files — they are computed state.

## The one rule: spec is the v2 API body, verbatim

`spec` mirrors the public v2 API create/update body — snake_case, no field renaming, no
camelCase translation, nothing invented. Consequences you can rely on:

- The [API reference](https://docs.orq.ai/reference) documents your manifest fields 1:1.
- `pull` is symmetric serialization, not a translation layer.
- Fields the DSL doesn't know about pass straight through to the API.

The DSL adds exactly three constructs on top: the envelope above, `ref:` references,
and `${var}` / `${env}` / `$file` interpolation.

## Identity per kind

The identity is the resource's declarative address within the stack. It comes from the
kind contract table in the engine's registry:

| Kind | Identity | Addressed on the API by | Reads include `path` | Create requires (spec) | Immutable (spec) → `±` replace |
|---|---|---|---|---|---|
| [Project](project.md) | `metadata.name` | list + match | n/a (no path) | — | — |
| [Prompt](prompt.md) | `path` + `display_name` | list + match | no (state holds path) | `prompt.messages` | — |
| [Agent](agent.md) | `metadata.key` | key directly | yes | `role`, `description`, `instructions`, `model`, `settings` | — |
| [Evaluator](evaluator.md) | `metadata.key` | list + match | no (state holds path) | `type` (+ per-type fields) | — |
| [KnowledgeBase](knowledge-base.md) | `metadata.key` | list + match | yes | `embedding_model` (internal) / `external_config` (external) | `embedding_model` |
| [Dataset](dataset.md) | `path` + `display_name` | list + match | no (state holds path) | — | — |
| [Tool](tool.md) | `metadata.key` | list + match | yes | `type`, `description` (+ per-type payload) | — |
| [MemoryStore](memory-store.md) | `metadata.key` (**no dashes**) | key directly | no (state holds path) | `description`, `embedding_config` | `embedding_config` |
| [Skill](skill.md) | `path` + `display_name` (letters/digits/underscores) | name directly | yes | — | — |

Deployment is designed but gated — the public API has no deployment write endpoints
(platform ask #4). Declaring `kind: Deployment` is a validate-time error.

!!! note "Identity is the address"
    Changing the identity field (`key`, `name`, or `display_name`) does not rename the
    resource — it declares a *different* one. The old identity disappears from the
    files, so plan shows a `−` delete for it (stack-owned only) plus a `+` create for
    the new one. Changing an **immutable spec field** on the *same* identity shows as
    `±` replace (delete + create in one step).

!!! warning "Prompt, Dataset, Skill: path + display_name fallback"
    These three kinds have no user-settable `key` on the platform (ask #1), so their
    identity is `path + display_name`. Renaming either means replace. Prompt and
    Dataset reads don't return `path` at all — the declared path lives in stack state.

## References

`{ref: <key>}` wherever the API expects the id or key of another workspace entity —
all ref sites live on Agent in v1 (full table in
[Architecture](../architecture.md#references-key-id-translation)):

```yaml
knowledge_bases:
  - ref: engineering-kb            # KnowledgeBase key → {knowledge_id: ...}
settings:
  guardrails:
    - ref: prompt-injection        # Evaluator key → {id: ...}
      execute_on: input
  tools:
    - type: http
      ref: jira-lookup             # Tool key
    - type: mcp
      ref: linear-readonly         # MCP Tool key + discovered-tool names
      tools: [list_issues, get_issue]
memory_stores: [user_context]      # MemoryStore keys, plain strings
```

Resolution order: this stack's manifests first (creating an ordering edge), then the
live workspace (pre-existing resources), else plan-time error. Model ids stay plain
strings — models are a catalog, not stack resources.

## Variables, secrets, includes

```yaml
model: ${var.default_model}                     # from orq.yaml / --var-file / --var / ORQ_VAR_*
headers:
  Authorization:
    value: "Bearer ${env.LINEAR_API_KEY}"       # process env, resolved at plan/apply time
    encrypted: true
instructions: { $file: ./instructions.md }      # file content becomes the string
```

- `${var.name}` — scalar values only, never keys/kinds/field names. Precedence, later
  wins: `orq.yaml` `variables:` → `--var-file f.yaml` → `--var name=value` →
  `ORQ_VAR_name` env var.
- `${env.NAME}` — read from the process environment; unset is a validation error.
  Never written to state, plan output, or pulled files.
- `{$file: ./relative/path}` — must be the only key of its object; replaces the node
  with the file's content as a string. Paths are relative to the manifest file. Use it
  for agent instructions, prompt messages, judge prompts, Python evaluator code.

There is deliberately nothing else: no loops, no conditionals, no functions.

## Stack config: `orq.yaml`

One per stack directory root; `orq dsl init` scaffolds it:

```yaml
stack: acme-platform          # ownership scope + state name; lowercase kebab
defaults:
  path: acme                  # metadata.path fallback
variables:
  default_model: mistral/mistral-large-latest
```

Workspace credentials never live in files — auth comes from the CLI environment
(`ORQ_API_KEY`, profiles, `--server`).
