# Tool

Workspace tool entities agents can call. Five variants via `spec.type`, each with its
own payload key.

```yaml
apiVersion: orq.ai/v1
kind: Tool
metadata:
  key: linear-readonly
  path: thales-onboarding/tools
spec:
  type: mcp                       # function | json_schema | http | mcp | code
  description: Linear tickets, read-only.
  status: live                    # live | draft | pending | published
  mcp:
    server_url: https://mcp.linear.app/mcp
    connection_type: http         # http | sse
    headers:
      Authorization: { value: "Bearer ${env.LINEAR_API_KEY}", encrypted: true }
```

## Identity

| | |
|---|---|
| Identity | `metadata.key` |
| API | `/v2/tools` — matched by `key` via list; reads include `path` |
| Create requires | `type`, `description`, + the matching payload key |
| Immutable | — |

## The five variants

`validate` checks that the payload key matching `type` is present:

| `spec.type` | Required payload key |
|---|---|
| `function` | `function` |
| `json_schema` | `json_schema` |
| `http` | `http` |
| `mcp` | `mcp` |
| `code` | **`code_tool`** — not `code` |

## Gotchas

!!! warning "code tools use spec.code_tool"
    The API's payload key for `type: code` is `code_tool`, not `code`. `validate`
    catches the mismatch offline: `tool type code requires spec.code_tool`.

!!! note "MCP headers are secrets"
    Every `mcp.headers.<name>.value` is treated as secret: write them as
    `${env.*}` references (with `encrypted: true` so the platform stores them
    encrypted). On `pull` each header value is redacted to a generated
    `${env.<KEY>_<HEADER>}` placeholder plus a warning — set the variable before the
    next apply.

- `status` (`live` / `draft` / `pending` / `published`) is the tool's lifecycle knob
  and a real managed field — declare it and the engine reconciles it.
- **MCP discovered tools:** after creating an MCP tool, the platform connects to the
  server and discovers its tools (`mcp.tools[]`, server-generated ids). The engine
  re-fetches right after create so agents in the same apply can select them by name:
  `{type: mcp, ref: linear-readonly, tools: [list_issues]}` — see [Agent](agent.md).
- Agents reference `http`/`code`/`function`/`json_schema` tools by key
  (`{type: http, ref: jira-lookup}`); the API accepts the key directly, no id
  translation needed.
