---
title: Testing
---
<!-- generated-by: gsd-doc-writer -->

# Testing

The Workspace Stacks engine (`cli/custom/dsl/`) is tested with Go's standard
`testing` package only — no third-party test framework, assertion library, or
mocking library. Tests run against either fully in-process fixtures or an
in-memory HTTP simulator; none of the suite talks to the real orq.ai API.

## Test framework and setup

- **Framework**: Go standard library `testing` (`go test`), Go 1.25 or newer
  (matches the `go 1.25.0` directive in `go.mod`).
- **Setup**: none beyond a normal Go toolchain — no `.env`, database, or
  external service is required. `go mod download` (or a plain `go build`)
  resolves dependencies on first run.
- **Location**: every test file lives beside the code it tests in
  `cli/custom/dsl/*_test.go`, all in package `dsl`. There are no test files
  outside that package (`cli/generated/` is bartolo-generated and untested by
  hand; `cli/custom/commands/`, `cli/custom/auth/`, and `cmd/orq/` currently
  have no `_test.go` files).

## Test suite layout

| File | Covers |
|---|---|
| `types_test.go` | `Manifest.Identity()` / `IdentityValue()` derivation, `ValidationError` string formatting |
| `loader_test.go` | `LoadStack` (`orq.yaml` parsing), `LoadManifests` (single/multi-document YAML, envelope errors, skipping the `vars/` directory) |
| `interp_test.go` | `ResolveVars` precedence (`orq.yaml` < `--var-file` < `--var` < `ORQ_VAR_*`), `Interpolate` for `${var.*}` / `${env.*}` / `{$file: ...}`, missing-var and missing-env failures |
| `validate_test.go` | `Registry()` integrity (every non-gated `KindInfo` has `Plural`/`BasePath`/`IdentityMode` and either `IDField` or `GetByIdentity`), offline schema/ref/identity validation, duplicate-identity detection, Tool payload shape, reserved state-skill name rejection |
| `refs_test.go` | `extractRefs` (ref sites: `knowledge_bases`, `memory_stores`, `team_of_agents`, guardrails, evaluators, tools), `BuildWaves` ordering and cycle detection, `ResolveRefs` and its error paths |
| `diff_test.go` | `DiffSpec` (managed-fields comparison, nested maps/slices, missing keys), `Classify` (create/update/delete/replace/noop), envelope drift, spec-hash stability, `symbolizeLive` for Agent refs |
| `plan_test.go` | `BuildPlan` — all-creates, update/drift/delete mix, unresolved-ref failure. Also defines the shared `planStack` fixture and `simClient` helper used across the suite |
| `apply_test.go` | `Execute` — create ordering by wave, partial-failure handling (dependents skipped), `DestroyPlan` reverse-tier ordering, adoption of unchanged live resources into state (`plan.Adoptions`) |
| `state_test.go` | `StateDoc` lifecycle (save/load), `ErrStateConflict` compare-and-swap, state-skill naming (`orq_dsl_state_<stack>`), corrupt-state handling |
| `client_test.go` | `Client.Do` auth header injection and decoding, missing-API-key error, `APIError` envelope parsing, 429 retry/backoff, no-retry on 400, `ListAll` cursor pagination (including the Project-specific cursor shape) |
| `pull_test.go` | `Pull` round-trip (live → manifest → re-plan with no drift), `orq stack init` scaffold generation and its refusal to overwrite an existing `orq.yaml` |

## Running tests

```console
$ go test ./...
```

This is the only test command in the project — there is no `make test` target
(`make help` lists `build`, `install-local`, `tidy`, `doctor`, `completions`,
`sync` only). Useful variations:

```console
# verbose output, one line per test
$ go test -v ./cli/custom/dsl/...

# a single test by name (regex match)
$ go test ./cli/custom/dsl/... -run TestApplyCreatesEverythingInOrder

# a single file's tests, e.g. everything ref/wave related
$ go test ./cli/custom/dsl/... -run TestExtractRefs -v

# race detector (the engine runs plan/apply with bounded concurrency)
$ go test -race ./cli/custom/dsl/...
```

Since every `_test.go` file in the repo lives under `cli/custom/dsl/`,
`go test ./...` and `go test ./cli/custom/dsl/...` currently exercise the same
set of tests.

## The workspace simulator

Two layers of testing use the same fake backend, `Simulator`
(`cli/custom/dsl/sim.go`): an in-memory stand-in for the public `/v2/*` API
that is faithful to the identity semantics the engine relies on (key-addressed
Agents and MemoryStores, name-or-id Skills, wrapped GET-one responses,
cursor-paginated lists). It derives its supported kinds and identity rules
directly from `Registry()`, so a new kind added to the registry is served by
the simulator automatically — no simulator changes are needed unless the kind
needs bespoke behavior (for example, `Simulator.create` synthesizes discovered
sub-tools for `Tool` manifests with `type: mcp`, mirroring the platform's MCP
sync).

**Unit tests** point a real `Client` at an `httptest.Server` wrapping
`sim.Handler()` — see the `simClient(t, sim)` helper in `plan_test.go`, used
throughout `apply_test.go`, `plan_test.go`, and others. `Simulator.Seed`,
`.Objects`, and `.DumpIdentities` give tests direct read/write access to the
in-memory store for setup and assertions.

**Dry smoke** (`scripts/dsl-smoke.sh`) exercises the real, compiled `orq`
binary end-to-end against the same simulator, run standalone instead of
wrapped in `httptest`:

```console
$ go run ./cli/custom/dsl/simserver -port 7899 &
$ export ORQ_SERVER=http://127.0.0.1:7899
$ export ORQ_API_KEY=dry-run
$ ./scripts/dsl-smoke.sh
```

`scripts/dsl-smoke.sh` builds `bin/orq`, starts `simserver` on a background
port, then drives a full lifecycle with a 9-kind stack (Project, Prompt,
KnowledgeBase, Dataset, MemoryStore, Skill, two Evaluators, two Tools, one
Agent referencing all of them): `init` → `validate` (both the `stack` and
legacy `dsl` alias) → `plan` (expects exit 2, all creates) → `apply` → `plan`
(expects exit 0, idempotent) → mutate two files → `plan`/`apply` (drift) →
`pull` into a fresh directory → re-`plan` the pulled output (expects no
changes) → delete a manifest → `plan`/`apply` (delete) → `state list` →
`destroy`. Each step asserts the CLI's exit code with the script's
`expect_exit` helper and aborts immediately on a mismatch.

A captured run is checked in at
[`docs/transcripts/dry-smoke-2026-07-09.txt`](transcripts/dry-smoke-2026-07-09.txt)
for reference — it is excluded from the built mkdocs site
(`exclude_docs: transcripts/` in `mkdocs.yml`) but readable straight from the
repo. Regenerate it by redirecting the script's output:
`./scripts/dsl-smoke.sh > docs/transcripts/dry-smoke-<date>.txt 2>&1`.

## Writing a new test for a resource kind

1. **Register the kind** — add a `KindInfo` entry to the `registry` map in
   `cli/custom/dsl/registry.go` (base path, identity mode, apply-wave tier,
   required/immutable/strip/secret fields). See
   [Architecture → Extension points](../ARCHITECTURE.md#extension-points) for
   the full field-by-field rationale.
2. **Let the integrity check catch omissions early** —
   `TestRegistryIntegrity` in `validate_test.go` iterates every non-gated
   `Registry()` entry and fails if `Plural`, `BasePath`, or `IdentityMode` is
   empty, or if neither `IDField` nor `GetByIdentity` is set. Run it first:
   `go test ./cli/custom/dsl/... -run TestRegistryIntegrity -v`.
3. **Add validation coverage** — extend a fixture in `validate_test.go` (see
   `validStack`, used by `TestValidateOK`) with a manifest for the new kind,
   and add at least one negative case (missing required field, bad identity
   charset, duplicate identity) following the pattern in
   `TestValidateBadStack`.
4. **Add plan/apply coverage** — extend the shared `planStack` fixture in
   `plan_test.go` (or write a new one) with a manifest for the new kind, using
   the `writeFile(t, dir, name, yaml)` and `stackDir(t)` helpers from
   `loader_test.go`. Build a plan against a fresh `Simulator` with
   `simClient(t, sim)`, then assert on `PlanResult.Waves` /
   `sim.DumpIdentities()` the way `TestApplyCreatesEverythingInOrder` does. If
   the kind can appear in a `ref:` site, add coverage to `refs_test.go`
   (`extractRefs`, `ResolveRefs`) following the existing Agent-reference
   cases.
5. **Iterate against just the new tests**:
   ```console
   $ go test ./cli/custom/dsl/... -run TestNewKind -v
   ```
6. **Run the full suite before opening a PR**:
   ```console
   $ go test ./...
   ```
7. **Optional — extend the dry smoke script**: add the new kind's manifest to
   `scripts/dsl-smoke.sh`'s smoke stack and re-run it against the simulator to
   confirm the compiled binary's `init`/`validate`/`plan`/`apply`/`pull`/
   `destroy` lifecycle handles it end-to-end, then refresh the checked-in
   transcript under `docs/transcripts/`.

## Coverage requirements

No coverage threshold is configured — there is no `.nycrc`, `c8` config, or
`coverageThreshold` equivalent for Go, and no `-cover` flag is wired into the
`Makefile` or any CI step. To check coverage manually:

```console
$ go test ./cli/custom/dsl/... -cover
$ go test ./cli/custom/dsl/... -coverprofile=coverage.out && go tool cover -html=coverage.out
```

## CI integration

The only workflow in `.github/workflows/` is `release.yml`, triggered on
`release: published`. It builds and publishes platform binaries and npm
packages — it does **not** run `go test`. There is currently no CI job that
runs the test suite automatically on push or pull request; `go test ./...`
(and, for end-to-end confidence, `./scripts/dsl-smoke.sh` against the
simulator) are run locally before submitting a change.

## Where next

- [Architecture](../ARCHITECTURE.md) — package layout and the abstractions
  these tests exercise (`Manifest`, `KindInfo`/`Registry()`, `Client`,
  `StateDoc`, `Change`/`PlanResult`, `refResolver`, `Simulator`)
- [Configuration](CONFIGURATION.md) — environment variables, including
  `ORQ_SERVER`/`ORQ_API_KEY`, used to point the CLI at the simulator during
  dry smoke
- [CLI reference](cli.md) — every `orq stack` command exercised by
  `scripts/dsl-smoke.sh`
