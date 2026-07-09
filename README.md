<!-- generated-by: gsd-doc-writer -->
# orq CLI

Official command-line interface for the [orq.ai](https://orq.ai) API — manage prompts,
agents, evaluators, knowledge bases, deployments, and the rest of the orq.ai platform
from your terminal, CI, or scripts.

This branch (`feat/dsl`) adds **Workspace Stacks**: a declarative, Terraform-style layer
that turns a directory of YAML manifests into a reconciled orq.ai workspace via
`orq stack validate|plan|apply|pull|destroy`.

```console
$ orq stack plan -f ./workspace
stack: acme-platform · 12 live · state rev 14

  + Evaluator/citation-presence
  ~ Agent/eng-companion
      instructions
  − Tool/old-linear-mcp  removed from files · owned by stack

Plan: 1 to create, 1 to update, 1 to delete, 0 to replace.
```

---

## Installation

### npm (recommended)

```sh
npm install -g @orq-ai/cli
```

Requires Node.js 14 or newer. The matching native binary is downloaded automatically for
your platform; no postinstall scripts or network downloads at runtime.

### curl | sh

```sh
curl -fsSL https://raw.githubusercontent.com/orq-ai/orq-cli/main/install.sh | sh
```

Installs a raw binary to `~/.orq/bin/orq`. Pin a specific version or pick a different
install directory:

```sh
# pin a version
curl -fsSL https://raw.githubusercontent.com/orq-ai/orq-cli/main/install.sh | ORQ_CLI_VERSION=v0.1.0 sh

# custom install dir (must be writable by the current user)
curl -fsSL https://raw.githubusercontent.com/orq-ai/orq-cli/main/install.sh | ORQ_CLI_INSTALL_DIR=/usr/local/bin sh
```

### Pre-built release binaries

Grab a binary for your platform from the [Releases page](https://github.com/orq-ai/orq-cli/releases).
Assets are named `orq-<os>-<arch>[.exe]`.

> The `npm`, `curl`, and release-binary installs above track `main` and do not yet
> include `orq stack` — Workspace Stacks are still on this feature branch. To use
> `orq stack` today, build from source.

### Build from source (required for `orq stack`)

Requires Go 1.25 or newer:

```sh
git clone https://github.com/orq-ai/orq-cli.git
cd orq-cli
git checkout feat/dsl
make build
./bin/orq stack --help
```

`orq dsl` is a permanent alias for `orq stack` — either spelling works.

---

## Quick start

### The full CLI

```sh
orq auth login           # OAuth device login; picks an active workspace
orq whoami                # verify identity
orq workspace list        # see available workspaces
orq prompts list          # run any generated resource command
orq doctor                 # diagnose auth, config, and endpoint reachability
```

### Workspace Stacks

```sh
export ORQ_API_KEY=<your orq.ai API key>

mkdir quickstart && cd quickstart
orq stack init --stack quickstart      # scaffold orq.yaml + an example manifest
orq stack validate -f .                # offline: schema, refs, vars — no credentials needed
orq stack plan -f .                    # show creates/updates/deletes (exit 2 if pending)
orq stack apply -f .                   # reconcile the live workspace to match the files
```

Full walkthrough, including a zero-risk local simulator with no production traffic:
see [docs/quickstart.md](docs/quickstart.md).

---

## Documentation

The [`docs/`](docs) directory is a [MkDocs Material](https://squidfunk.github.io/mkdocs-material/)
site dedicated to Workspace Stacks:

- [Home](docs/index.md) — what Workspace Stacks are and why
- [Quickstart](docs/quickstart.md) — zero to a reconciled workspace in five minutes
- [Manifest reference](docs/manifests/index.md) — one page per kind (Project, Prompt,
  Agent, Evaluator, KnowledgeBase, Dataset, Tool, MemoryStore, Skill)
- [CLI reference](docs/cli.md) — all seven `orq stack` commands, flags, exit codes
- [Architecture](docs/architecture.md) — how the reconciliation engine works
- [Guides](docs/guide) — new stack from scratch, migrating an existing workspace, CI/GitOps,
  multi-environment
- [Use cases](docs/use-cases.md), [State internals](docs/state-internals.md),
  [Limitations & platform asks](docs/limitations.md)

Serve it locally with live reload:

```sh
uvx --with mkdocs-material mkdocs serve
```

For the rest of the CLI (auth, profiles, output formats, the full resource command
surface), run `orq --help` or any `orq <resource> --help` — every generated command
documents its own inputs and examples.

---

## Development

```sh
make build              # local dev binary at ./bin/orq
make install-local      # install to ~/.local/bin/orq
make completions        # generate shell completions into ./completions/
make tidy               # go mod tidy
make doctor             # run the doctor command
go test ./...           # unit tests, including the DSL engine under cli/custom/dsl/
```

Try the Workspace Stacks lifecycle end-to-end against an in-memory simulator (zero
production traffic):

```sh
go run ./cli/custom/dsl/simserver -port 7899 &
export ORQ_SERVER=http://127.0.0.1:7899
export ORQ_API_KEY=dry-run
./scripts/dsl-smoke.sh   # scripted validate/plan/apply/pull/destroy run
```

### Project layout

```
cmd/orq/main.go              entrypoint
cli/generated/                bartolo-generated OpenAPI commands (DO NOT edit)
cli/custom/
├── register.go               custom entrypoint: middleware + commands
├── auth/                     OAuth device-login client, session store, URL resolution
├── commands/                 cobra commands: auth, workspace, doctor, identity, stack
└── dsl/                      Workspace Stacks engine: load, interpolate, validate,
                               diff, apply, pull, state, simulator (see docs/architecture.md)
docs/                          MkDocs Material site (Workspace Stacks documentation)
npm/
├── cli/                      @orq-ai/cli wrapper (JS shim + optionalDependencies)
└── cli-<os>-<arch>/           per-platform binary containers
scripts/
├── build.sh                  local dev build
├── dsl-smoke.sh               scripted Workspace Stacks lifecycle against the simulator
├── install-local.sh           install to ~/.local/bin
└── release-build.sh           cross-compile all platforms + stamp version
install.sh                     curl | sh installer
.github/workflows/release.yml  CI release workflow
```

### Regenerating from OpenAPI

```sh
bartolo sync
```

This wipes `cli/generated/` and rebuilds it from `openapi.json`. **`cli/custom/` is
never touched** — bartolo detects the existing directory and skips the stub.

---

## License

MIT — see [LICENSE](./LICENSE).
