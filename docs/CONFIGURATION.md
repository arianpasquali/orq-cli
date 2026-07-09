---
title: Configuration
---
<!-- generated-by: gsd-doc-writer -->

# Configuration

`orq` has two configuration surfaces:

- **CLI-wide** — auth, server, profile, output format. Shared by every `orq` command
  (`orq prompts list`, `orq agents list`, `orq stack apply`, ...).
- **Stack-specific** — `orq.yaml` and its `variables:` block, read only by `orq stack
  *` (declarative provisioning). See [Manifest reference](manifests/index.md#stack-config-orqyaml)
  for how variables are consumed inside manifests.

Both resolve the same way: flag beats environment variable beats persisted default.
Nothing that authenticates a request is ever read from a manifest file.

## Environment variables

| Variable | Required | Default | Description |
|---|---|---|---|
| `ORQ_API_KEY` | Yes¹ | — | API key sent as `Authorization: Bearer <key>`. Required for every command that talks to the API unless an OAuth session exists (`orq auth login`). |
| `ORQ_TOKEN` | No | — | Fallback credential, checked after `ORQ_API_KEY` (stack commands only — see `cli/custom/dsl/client.go`). |
| `ORQ_AUTHORIZATION` | No | — | Second fallback credential, checked after `ORQ_TOKEN`. |
| `ORQ_VAR_<NAME>` | No | — | Overrides stack variable `<name>` (e.g. `ORQ_VAR_default_model`). Highest-precedence variable source — see [Variable resolution](#variable-resolution). |
| `ORQ_SERVER` | No | `https://api.orq.ai` | Overrides the API base URL for generated resource commands and `orq stack *`. Same effect as `--server`. |
| `ORQ_API_BASE_URL` | No | `https://api.orq.ai` | Overrides the base URL used by `auth login`, `whoami`, and `workspace` commands. |
| `ORQ_V1_BASE_URL` | No | derived from `ORQ_API_BASE_URL` | Overrides the v1 API base URL (advanced/local dev). |
| `ORQ_PROFILE_BASE_URL` | No | `<v1_base_url>/me` | Overrides the profile endpoint (advanced/local dev). |
| `ORQ_PROFILE` | No | `default` | Active credentials profile. Same effect as `--profile <name>`. |
| `ORQ_OUTPUT_FORMAT` | No | `toon` | Default output format (`json`, `yaml`, `toon`). Same effect as `-o/--output-format`. |
| `ORQ_JSON` | No | `false` | Shortcut boolean equivalent to `--output-format json`. |
| `ORQ_QUERY` | No | — | JMESPath filter applied to output. Same effect as `-q`. |
| `ORQ_RAW` | No | `false` | Print query results unescaped. Same effect as `--raw`. |
| `ORQ_VERBOSE` | No | `false` | Enable verbose log output. Same effect as `--verbose`. |
| `ORQ_CLI_VERSION` | No | latest release | Version to install, read by `install.sh` only (not by the `orq` binary itself). |
| `ORQ_CLI_INSTALL_DIR` | No | `~/.orq/bin` | Install directory, read by `install.sh` only. |

¹ Not required for `orq stack validate`, which runs fully offline, or for any command
once `orq auth login` has stored a session.

Every persistent global flag is also readable as `ORQ_<FLAG_NAME_UPPERCASED>` (dashes
become underscores) — the list above covers the ones worth knowing; the mechanism
itself is generic (`viper.AutomaticEnv()` with prefix `ORQ`, wired in the vendored
[`bartolo`](https://github.com/orq-ai/bartolo) CLI framework).

### `.env` files

`.env` and `.env.local` in the current working directory are loaded automatically
before environment variables are read, without requiring `export`. Existing exported
variables always win — a `.env` value never overrides one already set in the shell.
Copy `.env.example` to `.env` to get started:

```console
$ cp .env.example .env
$ cat .env.example
# Copy this file to .env or export the variables in your shell.
ORQ_API_KEY=replace-me
```

## Stack configuration: `orq.yaml`

Every stack directory has exactly one `orq.yaml` at its root — `orq stack init`
scaffolds it, and every `orq stack *` command reads it via `-f/--file` (default `.`):

```yaml
stack: acme-platform          # ownership scope + state skill name; lowercase kebab
defaults:
  path: acme                  # metadata.path fallback for manifests that omit it
variables:
  default_model: mistral/mistral-large-latest
```

| Key | Required | Description |
|---|---|---|
| `stack` | Yes | Stack name. Must match `^[a-z][a-z0-9-]*$` (lowercase kebab-case). Names the server-side state skill (`orq_dsl_state_<stack, dashes→underscores>`) and scopes ownership for `apply`/`destroy`. |
| `defaults.path` | No¹ | Fallback for `metadata.path` on any manifest that omits it. Its first path segment names the project. |
| `variables` | No | `name: value` map of default stack variables, referenced in manifests as `${var.name}`. |

¹ Required in practice for every manifest except `kind: Project`, unless every
non-Project manifest sets its own `metadata.path` explicitly.

Workspace credentials never live in `orq.yaml` or any manifest — auth always comes
from the CLI environment (`ORQ_API_KEY`, a profile, or `--server`).

## Variable resolution

Stack variables (`${var.name}` in manifests) merge from four sources, **later wins**:

1. `orq.yaml` → `variables:` block
2. `--var-file <path>` — a flat YAML file of `name: value` pairs
3. `--var name=value` — repeatable CLI flag
4. `ORQ_VAR_<name>` environment variable

```console
$ orq stack plan -f . --var-file vars/prod.yaml --var default_model=mistral/mistral-small-latest
```

A `${var.name}` with no value from any source fails validation with
`${var.name} undefined — pass --var name=… or add it to orq.yaml variables`.

`${env.NAME}` interpolation is separate: it reads directly from the process
environment at plan/apply time (never through `--var`/`--var-file`), and an unset
`${env.NAME}` fails validation the same way. Values are never written to state, plan
output, or `pull`-generated files — see [Multi-environment](guide/multi-env.md#secrets-env-never-var-files).

### Variable files

Var-files are plain flat YAML (`name: value`), conventionally kept in a `vars/`
subdirectory — the manifest loader skips `vars/` by name, so files there are never
parsed as manifests:

```yaml title="vars/example.yaml"
# Per-environment overrides: orq stack plan -f . --var-file vars/example.yaml
default_model: mistral/mistral-large-latest
```

## Required vs. optional settings

| Setting | Required when | Failure mode if missing |
|---|---|---|
| `orq.yaml` in the stack directory | Every `orq stack *` command except `init` | `no orq.yaml in <dir> (run \`orq dsl init\`)` |
| `orq.yaml` → `stack` | Always (inside `orq.yaml`) | `\`stack\` is required` / name-format error |
| `orq.yaml` → `defaults.path` | Any manifest omits `metadata.path` (non-Project kinds) | `metadata.path is empty and orq.yaml sets no defaults.path` |
| `ORQ_API_KEY` (or `ORQ_TOKEN`, `ORQ_AUTHORIZATION`, or an OAuth session) | `plan`, `apply`, `pull`, `destroy`, `state list` | `missing API key: set ORQ_API_KEY (or ORQ_TOKEN / ORQ_AUTHORIZATION)` |
| Each `${var.name}` reference | Resolved at `validate` time | Validation error naming the missing variable |
| Each `${env.NAME}` reference | Resolved at `validate` time | Validation error naming the missing env var |

`orq stack validate` needs none of the credential settings above — it is fully
offline (schema, refs, and variable checks only).

## Defaults

| Setting | Default | Overridden by |
|---|---|---|
| API server | `https://api.orq.ai` | `--server`, `ORQ_SERVER`, or the host stored in an active OAuth session |
| Credentials profile | `default` | `--profile`, `ORQ_PROFILE` |
| Output format | `toon` | `-o/--output-format`, `ORQ_OUTPUT_FORMAT`, `orq default-format <fmt>` (persisted) |
| Stack directory (`orq stack *`) | `.` | `-f/--file` |
| `pull` output directory | `.` | `--out` |
| `apply`/`destroy` confirmation | interactive prompt | `--auto-approve` (skips it) |

## Per-environment overrides

Same manifests, different workspaces: dev/staging/prod are the same stack applied
with different credentials and different variable values.

```
workspace/
├── orq.yaml                 # variables: block holds the dev-friendly defaults
├── agents/ …
└── vars/
    ├── dev.yaml
    ├── staging.yaml
    └── prod.yaml
```

- **Variables** differ via `--var-file vars/<env>.yaml` (see [Variable resolution](#variable-resolution)).
- **Workspace target** differs via the credential, not the files — swap `ORQ_API_KEY`
  or `--profile` per environment. Each profile keeps its own session
  (`~/.orq/sessions/<profile>.json`) and can point at a different host via
  `--api-base-url` on `orq auth login`.
- **State** is per-workspace even when the stack name is identical: each workspace
  gets its own server-side state skill, so dev/staging/prod reconcile independently.

```console
$ ORQ_API_KEY=$DEV_KEY     orq stack apply -f . --var-file vars/dev.yaml
$ ORQ_API_KEY=$STAGING_KEY orq stack apply -f . --var-file vars/staging.yaml
$ ORQ_API_KEY=$PROD_KEY    orq stack apply -f . --var-file vars/prod.yaml
```

Full walkthrough: [Multi-environment](guide/multi-env.md) (layout and variable precedence) and
[CI / GitOps](guide/ci-gitops.md) (pipeline stages, including the per-team project-scoped key pattern).

## Persisted CLI state (`~/.orq/`)

These files are managed by the CLI itself — they are not meant to be hand-edited,
but knowing what lives where is useful for debugging (`orq doctor` reports the active
paths):

| File | Written by | Contents |
|---|---|---|
| `~/.orq/sessions/<profile>.json` | `orq auth login` | OAuth refresh token, bootstrap/workspace access tokens, resolved base URLs, active workspace. |
| `~/.orq/credentials.json` | `orq auth add-profile apikey <name> <key>` | Named API-key profiles. |
| `~/.orq/config.json` | `orq server set/use/clear`, `orq default-format <fmt>` | Persisted flag defaults (server override, server index, output format). |
| `~/.orq/cache.json` | CLI runtime | Internal cache, created empty on first run. |

## Where next

- [Quickstart](quickstart.md) — zero to a reconciled workspace in five minutes
- [Manifest reference](manifests/index.md) — `orq.yaml`, variables, secrets, and `$file` includes in context
- [CLI reference](cli.md) — every `orq stack` command, its flags, and exit codes
- [Multi-environment](guide/multi-env.md) — dev/staging/prod with one set of manifests
- [CI / GitOps](guide/ci-gitops.md) — pipeline stages and secret handling
- [State internals](state-internals.md) — how the server-side state skill works
