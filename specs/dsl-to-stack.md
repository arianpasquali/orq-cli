# Spec: rename `orq dsl` → `orq stack`

Status: approved (Arian, 2026-07-09) · Effort: ~1h · Risk: low

## Decision

The command group becomes `orq stack`. `dsl` stays as a permanent, silent alias.

Rationale (research 2026-07-09): every major declarative tool converged on
"stack" for this exact unit — files + state + lifecycle managed as one:
CloudFormation stacks, `pulumi stack`, `cdk deploy <stack>`, `az stack`
(Bicep deployment stacks), `terraform stacks`, `docker stack deploy`,
Spacelift/env0. A DevOps engineer reads `orq stack plan` with zero
explanation. "dsl" names the mechanism, not the job.

Rejected alternates: `compose` (implies *running* services, not *owning*
resources; docker owns the mental slot), `deploy`/`deployments` (collides
with the orq Deployment resource kind), `workspace` (taken by CLI context
switching + means the org account), top-level verbs (root namespace belongs
to bartolo's generated API nouns), `iac`/`manifest` (same how-not-what flaw
as dsl).

One name only. No dual-branding stack/compose — a second name is a tax on
every doc sentence and every support answer.

## Vocabulary

| Concept | Term |
|---|---|
| Command group | `orq stack` |
| The unit (files + orq.yaml + state) | a **stack** (already the term: `orq.yaml` field `stack:`, per-stack state, "stack-owned") |
| Feature/brand in docs | **Workspace Stacks** |
| The YAML format | manifests (prose may still say "a Kubernetes-style DSL" when describing the language itself — accurate, just not the command name) |

## Changes

### 1. CLI (`cli/custom/commands/dsl.go`)

- `Use: "dsl"` → `Use: "stack"`, add `Aliases: []string{"dsl"}`.
- `Short`: "Declarative workspace provisioning — validate, plan, apply, pull, destroy".
- Run-hint strings: "Run `orq stack apply -f …`", init next-steps
  ("orq stack validate -f …", "orq stack plan -f …").
- No deprecation warning on the alias — it costs nothing to keep forever.

### 2. State description string (`cli/custom/dsl/state.go:92`)

"orq stack state — managed by `orq stack`, do not edit". Cosmetic; written on
next apply.

### 3. Smoke (`scripts/dsl-smoke.sh`)

All `"$BIN" dsl …` invocations → `"$BIN" stack …`; add ONE `"$BIN" dsl
validate` step to lock the alias. Regenerate `docs/transcripts/`.

### 4. Docs (`mkdocs.yml`, `docs/**`, `README.md`, `demos/*/README.md`)

- `site_name: orq Workspace DSL` → `orq Workspace Stacks`.
- Every `orq dsl <cmd>` → `orq stack <cmd>` (~50 occurrences; mechanical).
- Prose "the DSL" → "Stacks" / "the stack engine" where it means the product;
  keep "DSL" only in the architecture page's language-design discussion.
- Quickstart: one line noting `orq dsl` remains as an alias.

## Explicit non-changes

- **State skill prefix `orq_dsl_state_`** (`cli/custom/dsl/registry.go:148`)
  stays. It is persisted in live workspaces; renaming orphans every existing
  stack's state or forces a read-old/write-new migration for zero user-visible
  gain. Internal, invisible.
- **Go package** `cli/custom/dsl` stays. Internal identifier; renaming churns
  ~25 files and every import for no user impact. Optional follow-up, separate
  PR if ever.
- Repo name / branch `feat/dsl`, docs URL slugs, architecture artifact URL —
  out of scope.

## Acceptance

- `orq stack validate|plan|apply|pull|destroy|state|init` all work.
- `orq dsl validate` still works (alias).
- `scripts/dsl-smoke.sh` passes end-to-end on `stack` verbs.
- `uvx --with mkdocs-material mkdocs build --strict` green.
- Gate: `rg -n 'orq dsl' docs/ README.md demos/` returns only the alias note.
