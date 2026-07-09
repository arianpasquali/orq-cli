---
title: Development
---
<!-- generated-by: gsd-doc-writer -->

# Development

This guide covers the local dev workflow for `orq` itself: building the binary, running the
in-memory Workspace Stacks simulator, project layout, how to add a new resource kind to the
DSL registry, and code-style conventions. For end-user CLI documentation, see the
[MkDocs site](index.md); for the code-level architecture of the DSL engine, see
[`ARCHITECTURE.md`](../ARCHITECTURE.md) at the repository root.

## Prerequisites

- Go **1.25** or newer (`go.mod` declares `go 1.25.0`).
- Optional, for serving the docs site locally: [`uv`](https://docs.astral.sh/uv/) (provides
  `uvx`) or any way to run `mkdocs` with the `mkdocs-material` theme.
- Optional, for regenerating `cli/generated/` from the OpenAPI spec: the
  [`bartolo`](https://github.com/orq-ai/bartolo) code generator (invoked via `make sync`).

## Local setup

```sh
git clone https://github.com/orq-ai/orq-cli.git
cd orq-cli
git checkout feat/dsl
cp .env.example .env    # then edit .env and set ORQ_API_KEY
make build               # -> ./bin/orq
```

`.env` and `.env.local` in the working directory are loaded automatically by the binary — no
`export` required (see [CONFIGURATION.md](CONFIGURATION.md#env-files)). `.env.example` only
declares `ORQ_API_KEY`; the rest of the CLI's environment variables are documented there.

## Build commands

All of the following are `make` targets defined in the root [`Makefile`](../Makefile). Each
corresponds to a small wrapper script in [`scripts/`](../scripts):

| Command | Description |
|---|---|
| `make build` | Builds `./bin/orq` from `./cmd/orq` (`scripts/build.sh`). |
| `make install-local` | Builds and installs `orq` to `~/.local/bin` (override with `INSTALL_DIR`) (`scripts/install-local.sh`). |
| `make tidy` | Runs `go mod tidy`. |
| `make doctor` | Runs `go run ./cmd/orq --json doctor` — diagnoses auth, config, and endpoint reachability. |
| `make completions` | Generates bash/zsh/fish/powershell completions into `./completions/`. |
| `make sync` | Runs `bartolo sync`, which wipes and rebuilds `cli/generated/` from `openapi.json`. `cli/custom/` is never touched. |
| `make help` | Prints the target list (also the default target). |

There is no `make test` or `make lint` target; run `go test` and formatting tools directly
(see [Running tests](#running-tests) and [Code style](#code-style) below).

`scripts/release-build.sh <semver>` is a separate, CI-oriented script that cross-compiles all
five shipped platform binaries into `npm/cli-<os>-<arch>/bin/`, ad-hoc signs the macOS
binaries, and stamps the version into every `npm/*/package.json`. It runs inside
[`.github/workflows/release.yml`](../.github/workflows/release.yml) on `macos-latest` when a
GitHub Release is published; it is safe to run locally but not part of the everyday dev loop.

## Running tests

The DSL engine's tests live alongside the source they cover in
[`cli/custom/dsl/`](../cli/custom/dsl) (`*_test.go` per file — e.g. `apply.go` /
`apply_test.go`), all in `package dsl` (white-box tests, no separate `_test` package).

```sh
go test ./...              # every package in the module
go test ./cli/custom/dsl/... -v   # just the DSL engine, verbose
go test ./cli/custom/dsl/... -run TestRegistryIntegrity   # a single test
```

Shared test fixtures live in `loader_test.go`: `stackDir(t)` creates a temp directory with a
minimal `orq.yaml` and returns it already loaded as a `StackConfig`, and `writeFile(t, dir,
name, content)` writes a manifest file into it. Most `*_test.go` files build on these two
helpers rather than hand-rolling temp-directory setup.

Network-touching code (`plan`, `apply`, `pull`, `live`) is tested against the in-memory
`Simulator` (`cli/custom/dsl/sim.go`) via `httptest.Server` — tests construct a `*Client`
pointed at `httptest.NewServer(sim.Handler())` instead of mocking individual HTTP calls (see
`Simulator` in [`ARCHITECTURE.md`](../ARCHITECTURE.md#key-abstractions)).

There is no CI workflow that runs `go test` today — [`.github/workflows/release.yml`](../.github/workflows/release.yml)
only builds and publishes on a published GitHub Release. Run tests locally before pushing.

### Full-binary smoke test

For an end-to-end check that exercises the real compiled binary (not just Go unit tests)
against a standalone copy of the simulator:

```sh
make build
go run ./cli/custom/dsl/simserver -port 7899 &
export ORQ_SERVER=http://127.0.0.1:7899
export ORQ_API_KEY=dry-run
./scripts/dsl-smoke.sh
```

`scripts/dsl-smoke.sh` scaffolds a throwaway stack covering all nine manifest kinds, then
drives it through `init → validate → plan → apply → plan (idempotence) → mutate → plan (drift)
→ apply → pull → plan (round-trip) → delete → plan → apply → state list → destroy`, asserting
the exit code at every step (`0` = no pending changes, `2` = changes pending). It builds
`./bin/orq` itself, so a prior `make build` is not required. Pass a working directory as `$1`
to keep the generated stack around for inspection (default: a fresh `mktemp -d`); override the
simulator port with `PORT=<n>`.

`cli/custom/dsl/simserver/main.go` is a thin `main` package that wraps `dsl.NewSimulator()` in
a standalone `net/http` server (`go run ./cli/custom/dsl/simserver -port <n>`) — the same
`Simulator` type the unit tests use in-process via `httptest.Server`.

## Code style

There is no ESLint/Prettier/Biome-equivalent linter or lint config (no `.golangci.yml`)
committed to this repository. Formatting is standard Go tooling plus [`.editorconfig`](../.editorconfig):

- `gofmt -l .` — should report no files; run `gofmt -w .` to fix.
- `go vet ./...` — static analysis, no separate config.
- `.editorconfig` enforces tabs / LF / trimmed trailing whitespace for `*.go`, `*.md`, `*.yml`,
  `*.yaml`, `*.sh` (Markdown uses 2-space indent instead of tabs).

Commit messages in this repository follow a Conventional-Commits-style `type(scope): summary`
format, e.g. `feat(dsl): apply, destroy and state list commands` or `test(dsl): dry smoke —
real binary against workspace simulator, full lifecycle` (see `git log` for more examples).
There is no `CONTRIBUTING.md` in this repository documenting a stricter convention.

## Branch conventions

No branch-naming convention is documented in the repository. The DSL work in this guide lives
on the `feat/dsl` feature branch off `main`.

## PR process

No `.github/PULL_REQUEST_TEMPLATE.md` or `CONTRIBUTING.md` is present in this repository, so no
formal PR checklist is enforced. In the absence of documented requirements:

- Run `go test ./...` and `gofmt -l .` before opening a PR.
- Keep commit messages in the `type(scope): summary` style used in `git log`.
- For changes to `cli/custom/dsl/`, prefer exercising `./scripts/dsl-smoke.sh` in addition to
  unit tests when the change touches the plan/apply/pull pipeline end-to-end.

## Project layout

```
cmd/orq/main.go               entrypoint — wires generated + custom command trees onto
                               bartolo's root command and executes
cli/generated/                 bartolo-generated OpenAPI commands (DO NOT edit; rebuilt by
                                `make sync`)
cli/custom/
├── register.go                custom entrypoint: session-aware auth middleware + command wiring
├── auth/                      OAuth device-login client, session store, URL resolution
├── commands/                  cobra commands: auth, workspace, doctor, identity, stack (dsl.go)
└── dsl/                       Workspace Stacks engine: load, interpolate, validate, diff,
                                apply, pull, state, simulator (see ../ARCHITECTURE.md)
    ├── types.go                Manifest, Metadata, StackConfig, Identity()
    ├── loader.go                orq.yaml + manifest file loading
    ├── interp.go                ${var.*} / ${env.*} / {$file:} interpolation
    ├── registry.go              per-kind API contract table — see "Adding a new resource kind" below
    ├── validate.go              offline schema/ref/identity validation
    ├── live.go                  live resource resolution + normalization
    ├── refs.go                  ref extraction, dependency waves, ref-to-id resolution
    ├── diff.go                  managed-fields diff, classification, id<->ref symbolization
    ├── plan.go                  plan assembly + human-readable rendering
    ├── apply.go                 wave execution, state persistence, partial-failure handling
    ├── pull.go                  live-to-manifest serialization, secret redaction, `init`
    ├── state.go                 server-side state document (stored in a reserved Skill)
    ├── client.go                authenticated v2 REST client with retry/backoff
    ├── sim.go                   in-memory API simulator, backs unit tests via httptest.Server
    ├── simserver/main.go        standalone simulator binary for full-process smoke tests
    └── *_test.go                one test file per source file, package-internal (white-box)
docs/                          MkDocs Material site (Workspace Stacks user documentation)
npm/
├── cli/                       @orq-ai/cli wrapper (JS shim + optionalDependencies)
└── cli-<os>-<arch>/            per-platform binary containers, populated by release-build.sh
scripts/
├── build.sh                   local dev build (`make build`)
├── install-local.sh            install to ~/.local/bin (`make install-local`)
├── release-build.sh            cross-compile all platforms + stamp version (CI release only)
└── dsl-smoke.sh                 scripted Workspace Stacks lifecycle against the simulator
examples/                       example stack manifests
specs/                          design/decision specs (e.g. the `dsl` → `stack` rename rationale)
install.sh                      curl | sh installer
.github/workflows/release.yml   CI release workflow (build + npm publish on GitHub Release)
```

## Adding a new resource kind to the DSL registry

The DSL engine is kind-generic: adding support for a new orq.ai resource kind is primarily a
matter of adding one entry to the `registry` map in
[`cli/custom/dsl/registry.go`](../cli/custom/dsl/registry.go). `validate.go`, `live.go`,
`diff.go`, `plan.go`, `pull.go`, and `sim.go` all read this table instead of hard-coding
per-kind behavior.

1. **Add a `KindInfo` entry** to the `registry` map, keyed by the manifest `kind` string:

   ```go
   "MyKind": {
       Kind: "MyKind", Plural: "my-kinds", BasePath: "/v2/my-kinds", Tier: 1,
       IdentityMode: "key", // or "display_name" / "name"
       Required:  []string{"some.required.field"},
       Immutable: []string{"field.that.forces.replace"},
       Strip:     strip("server_only_field"),
   },
   ```

   Key fields to set correctly (see the `KindInfo` doc comments in `registry.go` for the full
   list): `Tier` (0 = Project, 1 = leaf kinds, 2 = kinds that reference other kinds — controls
   apply-wave ordering), `IdentityMode` (`"name"`, `"key"`, or `"display_name"` — how the
   manifest's stable identity is derived), `GetByIdentity` (set `true` if the platform's GET/DELETE
   route addresses the resource by its identity value rather than a server `_id`),
   `ReadHasPath` (set `true` if GET/LIST responses include a `path` field), `IDField` (defaults
   to `_id`), `Wrap` (set if GET-one responses nest the object under a key, e.g.
   `{"project": {...}}`), `Required` (dotted spec paths checked offline at `validate` time),
   `Immutable` (dotted spec paths that force delete+create instead of update), `Strip` (server-computed
   fields dropped when normalizing live objects — start from `strip(...)`, which prepends
   `commonStrip`), and `Secret` (dotted spec paths redacted to `${env.*}` placeholders by `pull`).
   If the platform has no public write API for the kind yet, set `Gated` to a human-readable
   explanation instead of the rest of the fields (see the `Deployment` entry).

2. **Add kind-specific spec validation only if needed** — most kinds need nothing beyond the
   `Required`/`Immutable` declarations, which `validate.go` checks generically. If the kind has
   a discriminated spec shape (e.g. `Evaluator`'s `type: llm_eval` vs `type: python_eval`, or
   `Tool`'s `type: http` vs `type: mcp`), add a case to `validateKindSpecific` in
   [`validate.go`](../cli/custom/dsl/validate.go).

3. **Add ref-site handling only if the kind holds `ref:` fields** — if manifests of the new kind
   can reference other resources (like `Agent.settings.tools[].ref`), extend `extractRefs` (dependency
   graph, consumed by `BuildWaves`) and `ResolveRefs` (ref → server-id substitution at apply time)
   together in [`refs.go`](../cli/custom/dsl/refs.go).

4. **Extend the simulator** in [`sim.go`](../cli/custom/dsl/sim.go) so the new kind's `BasePath`
   is routed to an in-memory CRUD handler — this is what both unit tests and
   `scripts/dsl-smoke.sh` exercise against.

5. **Add a test** — add `TestRegistryIntegrity`-style coverage is already generic (it iterates
   every registry entry), but add kind-specific test cases to `validate_test.go`, `diff_test.go`,
   and `plan_test.go` covering the new kind's create/update/delete/replace behavior, following
   the `stackDir(t)` / `writeFile(t, ...)` pattern from `loader_test.go`.

6. **Document the manifest shape** — the MkDocs site has one page per kind under
   [`docs/manifests/`](manifests/index.md); add `docs/manifests/<kind>.md` and link it from
   `mkdocs.yml`'s `nav:` and `docs/manifests/index.md`.

Run `go test ./cli/custom/dsl/...` and `./scripts/dsl-smoke.sh` after adding a kind to confirm
the full validate → plan → apply → pull → destroy lifecycle works end-to-end.

## Regenerating from OpenAPI

```sh
make sync    # runs `bartolo sync`
```

This wipes `cli/generated/` and rebuilds it from `openapi.json`. `cli/custom/` (including the
DSL engine) is never touched — `bartolo` detects the existing directory and skips the stub.

## Serving the docs site locally

The [`docs/`](.) directory is a MkDocs Material site (`mkdocs.yml` at the repository root,
`docs_dir: docs`, `site_dir: site`):

```sh
uvx --with mkdocs-material mkdocs serve      # live reload at http://127.0.0.1:8000
uvx --with mkdocs-material mkdocs build --strict   # production build, fails on warnings
```

## Next steps

- [`ARCHITECTURE.md`](../ARCHITECTURE.md) — code-level package layout, call chain, and key
  abstractions of the DSL engine.
- [CONFIGURATION.md](CONFIGURATION.md) — every environment variable, `orq.yaml`, and variable
  resolution order.
- [Quickstart](quickstart.md) and [CLI reference](cli.md) — using the built binary end-to-end.
