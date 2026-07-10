# Architecture

How a directory of YAML becomes a converged workspace. The engine is hand-written Go
under `cli/custom/dsl/` in the orq CLI, calling the public v2 REST API — the same
endpoints the SDKs use. Nothing here requires platform changes.

## The pipeline

Every command is a prefix of the same pipeline:

```
files ──► load ──► interpolate ──► validate ──► fetch live ──► symbolize ──► diff ──► waves ──► execute
          orq.yaml  ${var} ${env}   schema       state id →     live ids →    managed   topo     POST/PATCH/
          *.yaml    $file           refs, vars   GET by key →   ref: keys     fields    tiers    DELETE /v2/*
          multi-doc                 identity     list+match
                                    charsets

validate = load → interpolate → validate            (offline, no credentials)
plan     = … → fetch → symbolize → diff → waves      (read-only)
apply    = the whole thing                           (writes, saves state per op)
```

- **Load** — walk the stack directory for `*.yaml`/`*.yml` (multi-doc files allowed;
  `orq.yaml` and `vars/` excluded), parse the envelope, apply `defaults.path` from
  `orq.yaml` to any manifest that omits `metadata.path`.
- **Interpolate** — resolve `${var.*}` (precedence: `orq.yaml` variables < `--var-file`
  < `--var k=v` < `ORQ_VAR_<name>` env), `${env.*}` (process env, errors when unset),
  and `{$file: rel}` includes (file content becomes the string, path relative to the
  manifest). No recursion into substituted values, no loops, no conditionals.
- **Validate** — kind registry checks (unknown/gated kinds, required spec fields,
  identity charsets), duplicate identities, ref syntax, reserved names. Errors cite
  `file:line`.
- **Fetch live** — per manifest, in order: state entry → `GET` by stored server id
  (404 falls through: deleted out-of-band); identity-addressable kinds (Agent,
  MemoryStore, Skill) → `GET` by identity value; otherwise list + client-side match on
  the identity field. Multiple matches error out and ask for state repair.
- **Symbolize** — live objects hold server ids where manifests hold `ref:` keys.
  Before diffing, known ids are translated *back* into ref shapes (see below), so
  desired and live compare in the same vocabulary.
- **Diff** — managed-fields comparison producing `+ ~ − ±` changes.
- **Waves** — topological ordering into apply tiers.
- **Execute** — POST/PATCH/DELETE against `/v2/*`, four concurrent operations per
  wave, state saved after every success.

## Managed-fields semantics

The differ compares **only the fields present in the manifest** — the `kubectl apply`
model:

- Live-only fields are ignored. Server-computed noise (ids, timestamps, versions) never
  causes diffs, and fields you don't declare are left alone.
- **Removing a field from a manifest does not reset it server-side.** To change a
  value, declare the new value. This is documented behavior, not a gap.
- Arrays compare atomically at their path (element order matters).
- Numbers are normalized first (YAML `10` == JSON `10.0`), so int/float representation
  never diffs.

Drift detection falls out for free: a UI edit to a declared field appears as `~` on the
next plan, with the files winning on apply.

Envelope fields diff specially: `metadata.display_name` (optional label on key-identified
kinds) compares against live; `metadata.path` compares against live when the kind's
reads return `path`, otherwise against the path recorded in stack state
(see [Limitations](limitations.md) for the affected kinds).

## References: key → id translation

Manifests reference other resources by **key** (`{ref: engineering-kb}`); several API
fields want server-generated **ids**. The engine translates at apply time; ids never
appear in files. All ref sites live on Agent in v1:

| Ref site | Target kind | Sent to the API as |
|---|---|---|
| `knowledge_bases[].ref` | KnowledgeBase | `{knowledge_id: <id>}` |
| `settings.guardrails[].ref` | Evaluator | `{id: <id>, execute_on, …}` |
| `settings.evaluators[].ref` | Evaluator | `{id: <id>, …}` |
| `settings.tools[].ref` (http/code/function/json_schema) | Tool | `{key: <key>, …}` (API accepts keys) |
| `settings.tools[].ref` (mcp) | Tool | expanded — see below |
| `memory_stores[]` (plain strings) | MemoryStore | keys, verbatim (API takes keys) |
| `team_of_agents[].key` | Agent | keys, verbatim (same-kind dependency edge) |

Refs resolve against the stack's own manifests first (adding a dependency edge for
ordering), then against the live workspace (pre-existing, unmanaged resources), else
plan fails with the offending site.

### MCP discovered-tool expansion

The one place the DSL adds sugar beyond the API. An MCP Tool entity discovers its
server's tools after creation; agents attach them by `tool_id`. In the manifest you
write names:

```yaml
settings:
  tools:
    - type: mcp
      ref: linear-readonly
      tools: [list_issues, get_issue]   # discovered-tool names
```

At apply time the engine reads the referenced Tool's discovered `mcp.tools[]` and
expands the entry into one `{type: mcp, tool_id: …}` per name. An empty/omitted `tools:`
list attaches **all** discovered tools. A name that doesn't exist fails with the list of
available names. Freshly created MCP tools are re-fetched right after create so
same-apply agents can resolve them. On `pull`, the reverse grouping restores the
`{type: mcp, ref, tools: [...]}` shape.

## State: a JSON document in a reserved Skill

Full reconcile needs to know what the stack *owns* — so removed manifests delete their
resources and colleagues' hand-made resources are never touched. That inventory lives
**server-side**, as JSON in the `instructions` of a reserved Skill named
`orq_dsl_state_<stack>` (dashes → underscores).

Why a Skill: the public API has no writable key/value store (RemoteConfigs are
read-only), but Skills have a workspace-unique, DB-enforced `display_name`, are
directly addressable by that name (`GET /v2/skills/{name}`), and their `instructions`
string PATCHes in place — everything a small state document needs. A native
`/v2/stacks` API (platform ask #2) replaces this mechanism without a shape change.

Server-side state means: no state file in git, no merge conflicts, works identically
from laptops and CI runners, one shared point per stack.

**Advisory revision guard** — the platform offers no compare-and-swap, so every save
re-reads the live revision first and refuses to clobber a concurrent writer:

```
state conflict: another apply moved the stack to revision 7 (this run started
from 5) — re-run to reconcile
```

Advisory, not atomic: a true race in the read-check-write window can still interleave,
which is why re-running always converges. Details in [State internals](state-internals.md).

## Apply ordering, concurrency, partial failure

`BuildWaves` orders non-noop changes with Kahn's algorithm over two edge sources:
explicit ref edges, plus an implicit edge from every resource to the Project manifest
whose name is its path's first segment. Tiers break remaining ties:

```
tier 0   Project
tier 1   KnowledgeBase · MemoryStore · Tool · Evaluator · Prompt · Dataset · Skill
tier 2   Agent (references tier-1 resources)

deletes  trailing waves, reverse tier order (Agents removed before what they reference)
```

Within a wave, up to 4 operations run concurrently, with exponential backoff on
429/502/503 (1s → 2s → 4s → 8s, max 5 attempts, `Retry-After` honored).

**Partial failure stance** — Terraform's, exactly:

- A failed resource marks its dependents `↷ skipped (dependency failed)`.
- Independent branches continue.
- State is saved after **every** successful operation, not at the end — an interrupted
  apply loses nothing.
- The run exits non-zero with per-resource errors; **re-running converges** (the whole
  engine is idempotent). No rollback.

## Pull: the round-trip invariant

`orq stack pull` serializes live resources to `<kind-plural>/<identity>.yaml`, normalizing
the same way plan does: server-computed fields stripped, ids symbolized back to `ref:`
keys, MCP tool ids grouped back into `{type: mcp, ref, tools: [...]}`, secret fields
redacted to `${env.*}` placeholders (with a warning naming each).

The contract, enforced by tests:

> **`pull` then `plan` against the same workspace = no changes.**

That makes pull the migration path for existing workspaces and the "iterated in the UI,
now commit it" workflow — what comes out of pull is exactly what apply would maintain.
Pulled files are literal: variables cannot be reverse-inferred, so parameterize by hand
afterwards ([guide](guide/migrate-pull.md)).

## What the engine never does

- Invent spec fields — `spec` is the v2 create/update body, verbatim.
- Touch resources outside its state inventory (deletes are scoped to stack ownership).
- Orchestrate version promotion (Draft→Live) — updates get whatever version behavior
  the public update endpoint has.
- Move data: KB documents and dataset rows are data plane, out of scope.
