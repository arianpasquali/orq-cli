# Quickstart

Zero to a reconciled workspace in five minutes. Every step below also runs against a
local simulator with zero production traffic — see [Risk-free: the simulator](#risk-free-the-simulator).

## 1. Build the CLI

Workspace Stacks ship inside the `orq` CLI (branch `feat/dsl`). Build from source:

```console
$ go build -o bin/orq ./cmd/orq
$ bin/orq stack --help
```

!!! note
    `orq dsl` is a permanent alias for `orq stack` — existing scripts keep working.

## 2. Authenticate

Same auth as every other `orq` command:

```console
$ export ORQ_API_KEY=<your orq.ai API key>
```

`ORQ_TOKEN` and `ORQ_AUTHORIZATION` are accepted as fallbacks. The base URL resolves
from `--server` / `ORQ_SERVER` / your CLI session, default `https://api.orq.ai`.

!!! note
    `orq stack validate` runs fully offline — no credentials needed. Only `plan`,
    `apply`, `pull`, `destroy`, and `state list` talk to the API.

## 3. Scaffold a stack

```console
$ orq stack init quickstart
? orq project resources live in (must already exist; the one your API key is scoped to): quickstart
created  agents/example-agent.yaml
created  orq.yaml
created  vars/example.yaml
$ cd quickstart
```

`init <stack>` scaffolds a directory named after the stack (an explicit `-f` wins).

`orq.yaml` names the stack (the ownership scope) and sets defaults:

```yaml
stack: quickstart

defaults:
  # metadata.path fallback for every manifest (project[/folder]).
  path: quickstart

variables:
  default_model: mistral/mistral-large-latest
```

The project (`defaults.path`, first segment of every `metadata.path`) must already
exist — API keys are minted inside a project in the UI, so create the project there
first and point the stack at it (`--project` skips the prompt). The stack manages
resources *inside* the project and never calls the projects API.

!!! note "Stack-owned projects need a workspace-scoped key"
    Declaring a `kind: Project` manifest makes the stack create/delete the project
    itself via `/v2/projects` — that endpoint returns HTTP 403 for project-scoped
    API keys. Only declare one if your key is workspace-scoped.

## 4. Write manifests

Replace the example agent with a knowledge base and an agent that references it.
Delete `agents/example-agent.yaml`, then:

```yaml title="knowledge-bases/support-kb.yaml"
apiVersion: orq.ai/v1
kind: KnowledgeBase
metadata:
  key: support-kb
spec:
  type: internal
  description: Product reference docs.
  embedding_model: mistral/mistral-embed
```

```yaml title="agents/support-agent.yaml"
apiVersion: orq.ai/v1
kind: Agent
metadata:
  key: support-agent
spec:
  role: Assistant
  description: Answers support questions from the knowledge base.
  instructions: |
    Answer using only retrieved context. Say "I don't know" otherwise.
  model: ${var.default_model}
  settings:
    max_iterations: 10
    tools:
      - type: query_knowledge_base
  knowledge_bases:
    - ref: support-kb          # by key; the engine translates to the server id
```

Both resources omit `metadata.path`, so they land in `quickstart` (the default from
`orq.yaml`). The first path segment names the pre-existing project from step 3.

## 5. Validate (offline)

```console
$ orq stack validate -f .
✓ 2 manifests · 2 kinds · schema ok · refs ok · vars ok
```

## 6. Plan

```console
$ orq stack plan -f .
stack: quickstart · 0 live · state rev 0

wave 1  + KnowledgeBase/support-kb
wave 2  + Agent/support-agent
          └─ needs KnowledgeBase/support-kb

Plan: 2 to create, 0 to update, 0 to delete, 0 to replace.

Run `orq stack apply -f .` to execute.
```

The wave gutter shows execution order — the dependency graph, topologically
sorted. `apply` runs exactly these waves.

`plan` exits `2` when changes are pending, `0` when clean — CI can gate on it.

## 7. Apply

```console
$ orq stack apply -f .
...
? Apply these 2 changes? Yes

wave 1  + KnowledgeBase/support-kb  created knowledgebase_0002 (0s)
wave 2  + Agent/support-agent  created agent_0003 (0s)

Apply complete: 2 created, 0 updated, 0 deleted, 0 replaced.
```

Waves follow dependency order: leaves first, the referencing agent last. The agent
body sent to the API carries `knowledge_bases: [{knowledge_id: "…"}]` — the `ref:`
was translated to the id created one wave earlier.

## 8. Plan again — converged

```console
$ orq stack plan -f .
stack: quickstart · 2 live · state rev 2

No changes. Workspace matches the manifests.
```

Exit code `0`. Re-running `apply` is a no-op; editing a manifest shows up as `~` with
the changed field paths; deleting a manifest file plans a `−` delete (scoped to what
the stack owns). That is the whole loop.

## Risk-free: the simulator

The repo ships an in-memory workspace simulator that speaks enough of the v2 API for
the full lifecycle. Point the CLI at it and nothing you do can touch a real workspace:

```console
$ go run ./cli/custom/dsl/simserver -port 7899 &
2026/07/09 08:54:06 workspace simulator listening on http://127.0.0.1:7899 (in-memory, throwaway)

$ export ORQ_SERVER=http://127.0.0.1:7899
$ export ORQ_API_KEY=dry-run
```

Every command in this quickstart now runs against the simulator — the console output
above is real simulator output (against a live workspace, only the server ids differ).
Kill the process and the "workspace" evaporates.

## Where next

- [Manifest reference](manifests/index.md) — the envelope, identity per kind, refs and variables
- [CLI reference](cli.md) — all seven commands, flags, exit codes
- [New stack guide](guide/new-stack.md) — layout conventions for a real stack
- [Migrating an existing workspace](guide/migrate-pull.md) — `pull` and adopt
