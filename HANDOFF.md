# Handoff: orq Workspace Stacks (`orq stack`)

Last updated: 2026-07-10. Branch `feat/dsl`, rebased on main 4.12.0-rc.13, pushed to
`github.com/arianpasquali/orq-cli`. Tracking ticket: RES-1095 (Linear, Research team).

## What this is

Declarative provisioning for orq.ai workspaces: Kubernetes-style YAML manifests,
Terraform-style lifecycle. `orq stack validate | plan | apply | pull | destroy |
state list | init` (`orq dsl` is a permanent alias). Nine kinds, `ref:` references,
`${var.*}` / `${env.*}` / `$file` interpolation, server-side state with CAS,
dependency waves, drift detection, pull round-trip.

Start with `docs/quickstart.md`. Full design: `docs/architecture.md` and the
manifest reference under `docs/manifests/`.

## Repo geography (important, slightly unusual)

| Path | Role |
|---|---|
| `orq-cli-dsl/` (this dir) | Working git worktree on `feat/dsl`. Work here. |
| `orq-cli-bkp/` | The old clone that physically stores this worktree's `.git`. Do NOT delete it. |
| `orq-cli/` | Fresh clone of the fork, `main` at rc.13. Wired here as remote `fresh`. |

Remotes in this worktree: `origin` = github.com/arianpasquali/orq-cli (branch pushed,
tracking), `fresh` = the local clone. To rebase on newer main:
`git fetch fresh main && git rebase fresh/main`.

## Build, test, run

```console
$ go build -o bin/orq ./cmd/orq          # or: make install-local (→ ~/.local/bin)
$ go test ./cli/custom/dsl/              # unit + golden tests
$ sh scripts/dsl-smoke.sh                # full lifecycle against the simulator
$ uvx --with mkdocs-material mkdocs serve   # docs site (build --strict must pass)
```

The simulator (`go run ./cli/custom/dsl/simserver -port 7899`) speaks enough v2 API
for the entire lifecycle; point the CLI at it with `ORQ_SERVER=http://127.0.0.1:7899`
and any `ORQ_API_KEY`. Zero production traffic. It is deliberately strict about
key-only addressing for Agents/MemoryStores (that strictness caught a real bug).

Code layout: engine in `cli/custom/dsl/` (validate, plan, diff, refs, apply, pull,
state, sim), command wiring in `cli/custom/commands/dsl.go`, kind contracts in
`cli/custom/dsl/registry.go`. Adding a kind = registry entry + validation +
(sometimes) ref sites + sim support + docs page.

## Platform realities learned the hard way

1. **API keys are project-scoped** (UI mints no workspace keys anymore). Reads 404
   outside the key's project, but some identities (agent keys) are workspace-unique,
   so creates can 409 against resources the key cannot see. Mitigations shipped:
   init prefixes scaffold keys with the stack name, `${var.stack}` / `${var.unique}`
   builtins exist, and 409s explain themselves. Workspace-scoped keys can be minted
   via a Management Key (`POST /v2/api-keys` with `project_scope: {all: {}}`).
2. **`kind: Project` requires a workspace-scoped key** (`/v2/projects` 403s
   otherwise). Default flow: no Project manifest; init asks for the pre-existing
   project (the one the key was created in).
3. **Key-addressed kinds (Agent, MemoryStore) must be deleted/updated by key**, not
   server id; the platform 404s id routes and the idempotency swallow hides it.
   Fixed; the simulator enforces it now.
4. Datasets/Prompts/Skills have no user-settable key: identity is path+display_name,
   rename = replace. Platform ask #1.

## State of play

Done and green: rename to `orq stack`, wave-grouped plan output with `needs`
annotations, aligned validation errors (no cobra usage dump, exit 1 CLI-wide),
project-aware init (asks project, scaffolds `./<stack>/`) and pull (`--project` >
`orq.yaml` > `--all`), builtin vars, thales demo stack (`demos/thales-workspace`,
17 manifests, validates clean), dry smoke 14/14.

## Open threads, in rough priority order

1. **Wave 1 kinds: RoutingRule, GuardrailRule, Policy.** Full public CRUD exists and
   is now in `openapi.yaml` (rc.13). Same list+match engine path as Dataset; evaluator
   `ref:` translation already exists. Design notes and manifest sketches:
   `specs/platform-api-coverage.md` plus the architecture doc's wave-1 preview.
   Constraint: workspace-global only in v1 (`project_id` is an id; resolving names
   needs a projects list that project-scoped keys cannot call).
2. **Wave 2/3 kinds**: AutoRouter (clean key identity), WorkspaceModel (presence
   kind, no GET: read via `enabled` flag on `GET /v2/models`), Identity, Notifier,
   Budget. All public. Same spec file.
3. **Platform asks** (prioritized in `specs/platform-api-coverage.md`): public BYOK
   endpoints, `GET /v2/workspace-models`, user-settable keys, name-based rule
   scoping, `/v2/environments`.
4. **Real-workspace cleanup**: two stray example agents in the orq-research
   workspace from the delete-bug episode ("Example Agent" in project
   orq-stack-thales-onboarding, plus an older `example-agent`). Delete in UI.
5. **Docs sweep for per-stack keys**: recommend direnv (`.envrc`, gitignored) for
   per-stack `ORQ_API_KEY`; deliberately no `.env` support in the CLI.
6. **PR to upstream** when ready: branch is rebased and pushed to the fork.

## Reference documents

- `specs/dsl-to-stack.md`: the rename decision and vocabulary.
- `specs/platform-api-coverage.md`: the API audit driving the kind roadmap.
- `docs/transcripts/dry-smoke-2026-07-09.txt`: real CLI output, refreshed by smoke.
- RES-1095 carries team-readable summaries of both design artifacts in its comments.
- Private artifacts (rendered versions): architecture
  `claude.ai/code/artifact/8e608063-…`, coverage audit `…/87326294-…`.
