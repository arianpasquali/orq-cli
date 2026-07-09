# Agent

The kind that ties a stack together — and the only kind holding references in v1. An
agent manifest owns its **complete** tool, guardrail, knowledge-base, and memory set:
one apply writes the whole desired agent. No provision→attach→assign phases, no
clobbering of attachments on re-apply.

```yaml
apiVersion: orq.ai/v1
kind: Agent
metadata:
  key: eng-companion
  path: thales-onboarding/agents
  display_name: Engineering Companion
spec:
  role: Assistant
  description: Grounded companion for Thales engineers.
  instructions: { $file: ./instructions.md }
  model:
    id: ${var.default_model}
    parameters: { temperature: 0.3 }
  fallback_models: [mistral/mistral-medium-latest]
  settings:
    max_iterations: 10
    max_execution_time: 300
    tool_approval_required: respect_tool
    tools:
      - type: current_date                      # built-in
      - type: query_knowledge_base              # built-in
      - type: http                              # workspace Tool by key
        ref: jira-lookup
      - type: mcp                               # DSL sugar: engine resolves
        ref: linear-readonly                    #   discovered-tool names
        tools: [list_issues, get_issue]         #   -> tool_id list
    guardrails:
      - ref: prompt-injection                   # -> evaluator id
        execute_on: input
        sample_rate: 100
  knowledge_bases:
    - ref: engineering-kb                       # -> {knowledge_id: ...}
  memory_stores: [user_context]                 # by key (API takes keys)
```

## Identity

| | |
|---|---|
| Identity | `metadata.key` — dashes allowed: `[A-Za-z][A-Za-z0-9]*([._-][A-Za-z0-9]+)*` |
| API | `/v2/agents` — GET/PATCH/DELETE address the **key** directly |
| Create requires | `role`, `description`, `instructions`, `model`, `settings` |
| Immutable | — |
| Apply tier | 2 — created/updated after everything it references |

`metadata.display_name` is an optional label; drift on it is detected and reconciled.

## Ref sites

Every reference an agent can hold, and what the engine sends to the API:

| Manifest | Target | API body |
|---|---|---|
| `knowledge_bases: [{ref: k}]` | KnowledgeBase | `[{knowledge_id: <id>}]` |
| `settings.guardrails: [{ref: k, execute_on, sample_rate}]` | Evaluator | `[{id: <id>, execute_on, sample_rate}]` |
| `settings.evaluators: [{ref: k, …}]` | Evaluator | `[{id: <id>, …}]` |
| `settings.tools: [{type: http/code/function/json_schema, ref: k}]` | Tool | `[{type: …, key: <k>}]` |
| `settings.tools: [{type: mcp, ref: k, tools: [names]}]` | Tool (mcp) | one `{type: mcp, tool_id: <id>}` per name |
| `memory_stores: [key, …]` | MemoryStore | keys, verbatim |
| `team_of_agents: [{key: k, …}]` | Agent | keys, verbatim (ordering edge only) |

Built-in tools (`current_date`, `query_knowledge_base`, …) carry no `ref` and pass
through untouched.

## Gotchas

!!! note "MCP tool selection is by name"
    `{type: mcp, ref: linear-readonly, tools: [list_issues]}` selects discovered tools
    by name from the referenced [Tool](tool.md); the engine expands each to its
    server-generated `tool_id`. Omit `tools:` to attach **everything** the MCP server
    exposes. A wrong name fails at apply time with the list of available names.

!!! note "Settings update as a unit"
    When any `settings.*` field changes, plan lists the precise paths but apply sends
    the full top-level `settings` object (the managed-fields rule works on top-level
    API fields). Keep the whole desired settings in the manifest — which the required
    fields force anyway.

- `team_of_agents` gives sub-agents an ordering edge (leader applied after members);
  the field content itself is passed through as the API defines it.
- `instructions` is required — `$file` keeps long system prompts out of the YAML.
- Agents are the natural place to check drift: a colleague toggling tools in the UI
  shows up as `~ Agent/... settings.tools` on the next plan, and files win on apply.
