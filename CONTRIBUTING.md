<!-- generated-by: gsd-doc-writer -->

# Contributing to orq CLI

Thanks for your interest in improving `orq`, the official command-line interface for the
[orq.ai](https://orq.ai) API. This guide covers how to set up the project, the conventions
used in this repo, and how to submit changes.

## Development setup

See [README.md](README.md#build-from-source-required-for-orq-stack) for prerequisites (Go
1.25+) and the full build/install flow, and the [Development](README.md#development) section
for the day-to-day command set (`make build`, `make tidy`, `go test ./...`, and the
`orq stack` simulator workflow). This file only covers the parts specific to contributing —
coding standards, commit/PR conventions, and where in the tree to make changes.

Two areas of the tree have different edit rules:

- `cli/generated/` — produced by the `bartolo` code generator from `openapi.json`
  (`make sync` / `bartolo sync`). **Never hand-edit files here** — regeneration wipes and
  rebuilds the directory.
- `cli/custom/` — hand-written code (auth, workspace, doctor, identity, and the `orq stack`
  / `dsl` engine). `bartolo sync` detects this directory and skips it, so this is where
  contributions belong. The Workspace Stacks engine lives under `cli/custom/dsl/`; see
  [ARCHITECTURE.md](ARCHITECTURE.md) for its package layout and call chain.

## Coding standards

- **Formatting** — run `gofmt` (or `go fmt ./...`) on any changed Go files before committing.
  There is no repo-specific linter configuration (no `.golangci.yml`); standard `go vet ./...`
  and `gofmt -l .` producing no output is the bar.
- **Editor defaults** — `.editorconfig` at the repo root sets tab indentation (width 4) for
  `.go`, `.md`, `.yml`, `.yaml`, and `.sh` files, except Markdown body text, which uses
  2-space indentation. Most editors pick this up automatically.
- **Tests** — Go tests live alongside the code they cover, named `<file>_test.go` (see
  `cli/custom/dsl/*_test.go` for the pattern used throughout the `dsl` package: `loader_test.go`
  next to `loader.go`, `apply_test.go` next to `apply.go`, and so on). Run the full suite with:

  ```sh
  go test ./...
  ```

  For end-to-end coverage of the `orq stack` lifecycle against the in-memory simulator
  (zero production traffic), run `./scripts/dsl-smoke.sh` as described in the README's
  Development section.
- **Generated commands are out of scope for manual edits** — if you need to change generated
  CLI behavior, update `openapi.json` and regenerate with `make sync`, or make the change in
  `cli/custom/` middleware instead.

## Commit and PR conventions

There is no `.github/PULL_REQUEST_TEMPLATE.md` or `CODEOWNERS` file in this repo, so the
conventions below are inferred from recent commit history rather than an enforced template.

- **Commit messages** — recent history on this branch uses [Conventional Commits](https://www.conventionalcommits.org/)
  with a scope, e.g. `feat(dsl): apply, destroy and state list commands` or
  `test(dsl): dry smoke — real binary against workspace simulator, full lifecycle`. Use
  `feat`, `fix`, `test`, `docs`, `refactor`, or `chore` as the type, and `(dsl)` as the scope
  for changes to the Workspace Stacks engine. Older commits on `main` predate this convention
  and use a plain capitalized summary (e.g. `Add OAuth device login, profile-aware sessions,
  and rich doctor`) — follow the `type(scope):` style for new commits.
- **Branches** — the active feature branch is `feat/dsl`; no branch-naming convention is
  documented beyond that. If in doubt, mirror the commit convention: `feat/short-description`,
  `fix/short-description`.
- **Before opening a PR:**
  - Run `gofmt -l .` and `go vet ./...` and resolve any output.
  - Run `go test ./...` and confirm it passes.
  - If your change affects `orq stack` behavior, run `./scripts/dsl-smoke.sh` locally.
  - Keep the commit message(s) scoped and descriptive per the convention above.
  - Update the relevant docs (`README.md`, `ARCHITECTURE.md`, or the `docs/` MkDocs site)
    if your change alters user-facing behavior, flags, or environment variables.

## Reporting issues

There are no issue templates (`.github/ISSUE_TEMPLATE/`) configured in this repo yet. When
filing an issue on the [GitHub Issues page](https://github.com/orq-ai/orq-cli/issues), please
include:

- The `orq` version (`orq --version`) and how it was installed (npm, curl installer, or
  built from source).
- The exact command you ran and the full output (redact any API keys).
- What you expected to happen versus what actually happened.
- For `orq stack` issues, whether the problem reproduces against the local simulator
  (`go run ./cli/custom/dsl/simserver`) or only against a live workspace.

## License

By contributing, you agree that your contributions will be licensed under the project's
[MIT License](./LICENSE).
