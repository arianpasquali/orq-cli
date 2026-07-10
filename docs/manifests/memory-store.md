# MemoryStore

Long-term memory for agents (per-user context, conversation history distillation).

```yaml
apiVersion: orq.ai/v1
kind: MemoryStore
metadata:
  key: user_context               # immutable; NO dashes ([A-Za-z][A-Za-z0-9._]*)
  path: thales-onboarding/memory
spec:
  description: Per-user conversation memory.
  embedding_config: { model: mistral/mistral-embed }   # immutable
  ttl: 2592000
```

## Identity

| | |
|---|---|
| Identity | `metadata.key` — **no dashes**: `[A-Za-z]([A-Za-z0-9]*([._][A-Za-z0-9]+)*)?` |
| API | `/v2/memory-stores` — GET/PATCH/DELETE address the **key** directly |
| Create requires | `description`, `embedding_config` |
| Immutable | `embedding_config` → `±` replace |

## Gotchas

!!! warning "No dashes in the key"
    Unlike agent keys, memory-store keys allow letters, digits, dots and underscores
    only — `user-context` fails validation, `user_context` passes. This is a platform
    rule, caught offline:

    ```
    ✗ memory-stores/user-context.yaml:4  memory store key "user-context":
      letters/digits/dots/underscores only — dashes are not allowed
    ```

!!! warning "embedding_config is immutable"
    Changing `embedding_config` (e.g. the model) plans a `±` **replace** — delete +
    create. Stored memories live on the old entity and do not survive.

- Path is write-only — reads don't return it; the declared value lives in stack state.
- Agents attach stores by key, as plain strings: `memory_stores: [user_context]` — the
  API takes keys here, so there is no `ref:` wrapper and no id translation. The engine
  still validates the target exists and orders the store before the agent.
- `ttl` is seconds (the example is 30 days) and updates in place.
