# CLI reference

Seven commands under `orq stack`. Console output below is real, captured from the dry
smoke run against the [workspace simulator](quickstart.md#risk-free-the-simulator)
(paths shortened; against a live workspace only the server ids differ).

```console
$ orq stack --help
Available Commands:
  apply       Reconcile the workspace to match the manifests
  destroy     Delete every resource owned by the stack (reverse dependency order)
  init        Scaffold orq.yaml and an example manifest
  plan        Show the changes apply would make (exit 2 when changes are pending)
  pull        Serialize live workspace resources into manifest files
  state       Inspect stack state
  validate    Validate manifests offline (schema, refs, vars) — no credentials needed
```

Global flags apply everywhere: `--server` (API base URL), `--profile` (credentials
profile), `-o/--output-format json|yaml|toon` (machine-readable subcommands). Note
`-o` belongs to the root command — that is why `pull` spells its output directory
`--out`.

## Exit codes

| Code | Meaning |
|---|---|
| `0` | success — for `plan`: no changes, workspace converged |
| `1` | error — validation failure, API error, partial apply failure, cancelled prompt |
| `2` | **changes pending** (`plan` only) — the CI gate |

## orq stack init

Scaffold a stack directory: `orq.yaml`, an example agent, a vars file. Refuses to
overwrite an existing `orq.yaml`.

| Flag | Default | |
|---|---|---|
| `[stack]` (positional) | — | stack name; scaffolds `./<stack>/` unless `-f` is given |
| `-f, --file` | `./<stack>` or `.` | directory to scaffold |
| `--stack` | directory name | stack name (lowercase kebab: `^[a-z][a-z0-9-]*$`) |
| `--project` | stack name | existing orq project resources live in (prompted interactively when omitted) |

```console
$ orq stack init -f ./stack --stack orq-dsl-smoke --project orq-dsl-smoke
created  agents/example-agent.yaml
created  orq.yaml
created  vars/example.yaml

next:
  orq stack validate -f ./stack
  orq stack plan -f ./stack
```

## orq stack validate

Offline pipeline: load → interpolate → schema/identity/ref/duplicate checks. No
network, no credentials — safe in any PR pipeline.

| Flag | Default | |
|---|---|---|
| `-f, --file` | `.` | stack directory (containing `orq.yaml`) |
| `--var-file` | — | YAML file with variable values |
| `--var` | — | `name=value` override (repeatable) |

```console
$ orq stack validate -f ./stack
✓ 11 manifests · 9 kinds · schema ok · refs ok · vars ok
```

Failures print one line per problem, anchored to `file:line`, and exit `1`:

```console
$ orq stack validate -f ./broken
✗ memory-stores/user-context.yaml:4  memory store key "user-context": letters/digits/dots/underscores only — dashes are not allowed
✗ evaluators/judge.yaml:6  llm_eval with mode single requires spec.model
Error: 2 validation error(s)
```

## orq stack plan

Everything validate does, plus: fetch live state, diff, order into waves. Read-only.
Exits `2` when changes are pending.

Flags: same as `validate` (`-f`, `--var-file`, `--var`).

```console
$ orq stack plan -f ./stack
stack: orq-dsl-smoke · 0 live · state rev 0

wave 1  + Project/orq-dsl-smoke
wave 2  + Dataset/orq-dsl-smoke|smoke-golden
        + Evaluator/smoke-judge
        + Evaluator/smoke-python-guard
        + KnowledgeBase/smoke-kb
        + MemoryStore/smoke_memory
        + Prompt/orq-dsl-smoke|smoke-classify
        + Skill/orq-dsl-smoke|smoke_playbook
        + Tool/smoke-http
        + Tool/smoke-linear
wave 3  + Agent/smoke-companion
          └─ needs KnowledgeBase/smoke-kb · MemoryStore/smoke_memory · Evaluator/smoke-python-guard · Evaluator/smoke-judge · Tool/smoke-http · Tool/smoke-linear

Plan: 11 to create, 0 to update, 0 to delete, 0 to replace.

Run `orq stack apply -f ./stack` to execute.
```

The wave gutter is the dependency graph, topologically sorted: a wave only runs
after everything above it, and `apply` executes the same waves. `└─ needs` lists a
resource's `ref:` targets. Line glyphs: `+` create · `~` update (changed field
paths indented below) · `−` delete (in state, absent from files) · `±` replace
(immutable field changed; reason shown). Drift renders as updates with the
changed paths:

```console
$ orq stack plan -f ./stack        # after editing two manifests
stack: orq-dsl-smoke · 11 live · state rev 11

wave 1  ~ Evaluator/smoke-judge
          prompt
wave 2  ~ Agent/smoke-companion
          instructions
          └─ needs KnowledgeBase/smoke-kb · MemoryStore/smoke_memory · Evaluator/smoke-python-guard · Evaluator/smoke-judge · Tool/smoke-http · Tool/smoke-linear

Plan: 0 to create, 2 to update, 0 to delete, 0 to replace.
```

A converged stack exits `0`:

```console
$ orq stack plan -f ./stack
stack: orq-dsl-smoke · 11 live · state rev 11

No changes. Workspace matches the manifests.
```

## orq stack apply

Render the plan, confirm, execute in dependency waves (4 concurrent ops per wave),
saving stack state after every successful operation.

| Flag | Default | |
|---|---|---|
| `-f, --file` | `.` | stack directory |
| `--var-file`, `--var` | — | as in validate |
| `--auto-approve` | `false` | skip the confirmation prompt (CI) |

```console
$ orq stack apply -f ./stack --auto-approve
[plan output as above]

wave 1  + Project/orq-dsl-smoke  created project_0001 (0s)
wave 2  + Dataset/orq-dsl-smoke|smoke-golden  created dataset_0007 (0s)
wave 2  + Evaluator/smoke-judge  created evaluator_0006 (0s)
wave 2  + KnowledgeBase/smoke-kb  created knowledgebase_0010 (0s)
wave 2  + MemoryStore/smoke_memory  created memorystore_0008 (0s)
wave 2  + Tool/smoke-linear  created tool_0003 (0s)
   ⋮
wave 3  + Agent/smoke-companion  created agent_0014 (0s)

Apply complete: 11 created, 0 updated, 0 deleted, 0 replaced.
```

Without `--auto-approve`, apply asks `Apply these N changes?` and cancels on anything
but yes. On failure, the failed resource's dependents print
`↷ <identity>  skipped (dependency failed: …)`, independent branches continue, and the
run exits `1` with `apply finished with failures — re-run to converge`. Re-running is
always safe — the engine is idempotent.

Removing a manifest plans and applies a delete, scoped to stack ownership:

```console
$ rm stack/dataset.yaml && orq stack apply -f ./stack --auto-approve
stack: orq-dsl-smoke · 10 live · state rev 13

  − Dataset/orq-dsl-smoke|smoke-golden  removed from files · owned by stack

Plan: 0 to create, 0 to update, 1 to delete, 0 to replace.

wave 1  − Dataset/orq-dsl-smoke|smoke-golden  deleted (0s)

Apply complete: 0 created, 0 updated, 1 deleted, 0 replaced.
```

## orq stack pull

Serialize live workspace resources into manifest files:
`<kind-plural>/<identity>.yaml`, normalized so that **pull then plan = no changes**.

| Flag | Default | |
|---|---|---|
| `--project` | from `orq.yaml` | project name to scope the pull |
| `--all` | `false` | pull the entire workspace (no project scope) |
| `--out` | `.` | output directory (no `-o` shorthand — that's the global output-format flag) |
| `--stack` | — | existing stack whose state should inform paths/identities |

Pull is project-scoped by design. Scope resolves as: `--project` wins; otherwise the
enclosing stack's project (first segment of `defaults.path` in `./orq.yaml`); otherwise
the command errors — pass `--all` explicitly to serialize the whole workspace.

```console
$ orq stack pull --project orq-dsl-smoke --stack orq-dsl-smoke --out ./pulled
written  agents/smoke-companion.yaml
written  datasets/smoke-golden.yaml
written  evaluators/smoke-judge.yaml
written  evaluators/smoke-python-guard.yaml
written  knowledge-bases/smoke-kb.yaml
written  memory-stores/smoke_memory.yaml
written  prompts/smoke-classify.yaml
written  skills/smoke_playbook.yaml
written  tools/smoke-http.yaml
written  tools/smoke-linear.yaml
⚠ Tool/smoke-linear: mcp.headers.Authorization.value redacted → ${env.SMOKE_LINEAR_AUTHORIZATION} — set it before apply

pulled 10 resources → ./pulled
```

Secret-bearing fields come out as `${env.*}` placeholders with a warning each — set
the variables before applying. Reserved state skills are never pulled. See the
[migration guide](guide/migrate-pull.md) for adopting the result into a stack.

## orq stack destroy

Delete everything in the stack's inventory, reverse dependency order (agents first,
project last), then remove the state skill itself.

| Flag | Default | |
|---|---|---|
| `-f, --file` | `.` | stack directory (reads `orq.yaml` for the stack name) |
| `--auto-approve` | `false` | skip the typed confirmation |

```console
$ orq stack destroy -f ./stack --auto-approve
Stack orq-dsl-smoke owns 10 resources:
  − Project/orq-dsl-smoke
  − Evaluator/smoke-judge
   ⋮
  − Agent/smoke-companion

wave 1  − Agent/smoke-companion  deleted (0s)
wave 2  − Evaluator/smoke-judge  deleted (0s)
wave 2  − KnowledgeBase/smoke-kb  deleted (0s)
   ⋮
wave 3  − Project/orq-dsl-smoke  deleted (0s)

Destroyed 10 resources. Stack state removed.
```

!!! warning "Typed confirmation"
    Interactively, destroy demands you **type the exact stack name** — a yes/no prompt
    is too cheap for "delete everything". `--auto-approve` skips this, so guard the
    flag in automation. Only stack-owned resources are touched, ever.

## orq stack state list

Print the stack inventory (the [state document](state-internals.md)) through the CLI's
standard formatter — `-o json|yaml|toon`, `--json`, `-q` JMESPath all work.

| Flag | Default | |
|---|---|---|
| `-f, --file` | `.` | stack directory (reads `orq.yaml` for the stack name) |

```console
$ orq stack state list -f ./stack
resources[10]{applied_at,identity,kind,path,server_id,spec_hash}:
  "2026-07-09T06:54:38Z",Project/orq-dsl-smoke,Project,"",project_0001,"sha256:ff8978f78cabd641"
  "2026-07-09T06:54:38Z",Evaluator/smoke-judge,Evaluator,orq-dsl-smoke,evaluator_0006,"sha256:ffe35a13ce4c604b"
  "2026-07-09T06:54:38Z",Agent/smoke-companion,Agent,orq-dsl-smoke,agent_0014,"sha256:8090ec89b464f136"
   ⋮
revision: 14
stack: orq-dsl-smoke
version: 1
```

A never-applied stack prints `Stack <name> has never been applied (no state).`
