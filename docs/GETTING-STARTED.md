---
title: Getting Started
---
<!-- generated-by: gsd-doc-writer -->

# Getting Started

This page gets the base `orq` CLI installed and authenticated, and walks through the
first commands you'll run against your orq.ai workspace. If you're here for
**Workspace Stacks** (`orq stack` / `orq dsl`, the declarative provisioning layer on
this branch), finish the [Prerequisites](#prerequisites) and [Authenticate](#2-authenticate)
sections below, then jump straight to the [Quickstart](quickstart.md) — it covers
scaffolding, validating, planning, and applying a stack in detail.

## Prerequisites

| Install method | Requirement |
|---|---|
| `npm install -g @orq-ai/cli` | Node.js `>= 14` |
| `curl \| sh` | macOS or Linux (`curl`, `uname`, `mktemp`, `chmod`, `mv` — present on virtually every system). Windows is not supported by the installer; use npm instead. |
| Build from source (required for `orq stack`) | Go `1.25` or newer, `git` |

An orq.ai account is required either way — `orq auth login` opens a browser to
authenticate, or you can use an API key from your workspace settings.

## 1. Install

Pick one:

```sh
# npm (recommended for the base CLI)
npm install -g @orq-ai/cli
```

```sh
# curl | sh — raw binary to ~/.orq/bin/orq
curl -fsSL https://raw.githubusercontent.com/orq-ai/orq-cli/main/install.sh | sh
```

> The `npm` and `curl` installs track `main` and do not yet include `orq stack` /
> `orq dsl` — Workspace Stacks are still on the `feat/dsl` feature branch. To use
> `orq stack`, build from source instead:
>
> ```sh
> git clone https://github.com/orq-ai/orq-cli.git
> cd orq-cli
> git checkout feat/dsl
> make build
> ./bin/orq stack --help
> ```
>
> See the [README](../README.md#installation) for the full set of install options,
> including pinning a version and choosing a custom install directory.

Verify the binary is on your `PATH`:

```console
$ orq --version
```

## 2. Authenticate

```console
$ orq auth login
Open: https://accounts.orq.ai/activate?user_code=ABCD-1234
Code: ABCD-1234
Waiting for browser approval...
```

This starts an OAuth device login: it prints a verification URL and code, opens your
browser automatically (pass `--no-open` to skip that), and waits for you to approve
the sign-in. If your account belongs to more than one workspace, you'll be prompted
to pick the active one (or pass `--workspace <key>` up front).

Alternatively, set an API key instead of logging in interactively:

```sh
export ORQ_API_KEY=<your orq.ai API key>
```

Confirm you're authenticated:

```console
$ orq whoami
```

## 3. First commands

```sh
orq whoami               # current user + active workspace
orq workspace list       # every workspace your account can access
orq workspace use <key>  # switch the active workspace
orq doctor                # config, auth state, and endpoint reachability
orq prompts list          # example generated resource command
```

Every resource command (`orq prompts`, `orq agents`, `orq evaluators`, `orq
knowledge-bases`, `orq deployments`, ...) is generated from the orq.ai OpenAPI spec —
run `orq --help` or `orq <resource> --help` to see the full surface, including
per-command examples.

Output defaults to `toon` (a compact, human-scannable format); pass
`-o json` or `-o yaml` for machine-readable output, or `-q '<jmespath>'` to filter a
result. See [Configuration](CONFIGURATION.md) for the full list of environment
variables and flags.

## Common setup issues

| Symptom | Cause | Fix |
|---|---|---|
| `you are not logged in` on `orq whoami` / `orq workspace list` | No session file yet | Run `orq auth login` first |
| `missing API key: set ORQ_API_KEY (or ORQ_TOKEN / ORQ_AUTHORIZATION)` on a `orq stack` command | No `ORQ_API_KEY` and no OAuth session | `orq auth login`, or `export ORQ_API_KEY=<key>` |
| `orq stack: command not found` (or `unknown command "stack"`) after `npm install` / `curl \| sh` | Workspace Stacks aren't published yet — those installs track `main` | Build from source on `feat/dsl` (see [Install](#1-install)) |
| `orq: command not found` after the `curl \| sh` installer | `~/.orq/bin` isn't on your `PATH` | Add the `export PATH=...` line the installer prints to your shell profile |
| `npm install -g @orq-ai/cli` fails on an old Node | Node.js `< 14` | Upgrade Node.js to `>= 14` |

Run `orq doctor` any time to check config resolution, session state, and API
reachability in one shot.

## Next steps

- [Quickstart](quickstart.md) — scaffold, validate, plan, and apply your first
  Workspace Stack, including a zero-risk local simulator
- [Configuration](CONFIGURATION.md) — every environment variable, `orq.yaml`, and
  variable-resolution rule
- [Architecture](architecture.md) — how the Workspace Stacks reconciliation engine
  works
- [CLI reference](cli.md) — all seven `orq stack` commands, flags, and exit codes
- [Manifest reference](manifests/index.md) — one page per resource kind
