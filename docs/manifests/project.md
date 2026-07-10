# Project

The container everything else lives in. Tier 0 of every apply: projects are created
before any resource whose `metadata.path` starts with their name.

!!! warning "Requires a workspace-scoped API key"
    API keys minted in the UI are **project-scoped** and get HTTP 403 on every
    `/v2/projects` call — a stack declaring `kind: Project` cannot plan or apply
    with one. The default is therefore to declare **no** Project manifest: `orq
    stack init` asks for a pre-existing project and the stack only manages
    resources inside it. Declare a Project only with a workspace-scoped key
    (`project_scope: all`, mintable via a Management Key on `POST /v2/api-keys`).

```yaml
apiVersion: orq.ai/v1
kind: Project
metadata:
  name: thales-onboarding          # identity: name — no key, no path
spec:
  description: Thales POC assets
```

## Identity

| | |
|---|---|
| Identity | `metadata.name` — Projects have **no key and no path** |
| API | `POST/GET/PATCH/DELETE /v2/projects` (matched by `name` via list) |
| Create requires | nothing beyond `name` |
| Immutable | — (but renaming = new identity = delete + create) |

Setting `metadata.key` or `metadata.display_name` on a Project is a validation error —
the identity is `name`, alone.

## How other manifests bind to it

There is no `project:` field anywhere. The **first segment of `metadata.path`** is the
project name:

```yaml
metadata:
  path: thales-onboarding/agents   # project "thales-onboarding", folder "agents"
```

When a stack declares a Project manifest, every resource whose path starts with that
name gets an implicit dependency edge — the project is created first, and on destroy
it is deleted last. Paths can also point at projects the stack does *not* manage
(pre-existing ones); folders below the project are auto-created either way.

!!! note "Pull never emits Project manifests"
    `orq stack pull` serializes resources, not projects — `--project` is a scope filter.
    When adopting a pulled workspace into a stack, add the Project manifest by hand if
    you want the stack to own the project itself (and its deletion on `destroy`).

!!! warning "Deleting a Project deletes its contents"
    A `−` on a Project (manifest removed, or `destroy`) removes the project on the
    platform, taking its folders and contents with it. Deletes run reverse-topo, so
    stack-owned resources inside are removed first anyway — but anything *unmanaged*
    inside the project goes down with the ship. Keep shared projects out of stack
    ownership: reference them by path without declaring a Project manifest.
