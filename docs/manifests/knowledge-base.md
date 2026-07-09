# KnowledgeBase

RAG retrieval configuration — internal (platform-embedded) or external (bring your own
retrieval API). Document ingestion is data plane and out of the DSL's scope.

```yaml
apiVersion: orq.ai/v1
kind: KnowledgeBase
metadata:
  key: engineering-kb             # immutable
  path: thales-onboarding/knowledge
spec:
  type: internal                  # or `external` + external_config{name,api_url,api_key}
  description: Product reference docs.
  embedding_model: mistral/mistral-embed   # create-time; no auto re-embed
  retrieval_settings:
    retrieval_type: hybrid_search
    top_k: 8
    rerank_config: { rerank_model: cohere/rerank-v3.5, top_k: 5 }
  # document ingestion out of scope (data plane)
```

## Identity

| | |
|---|---|
| Identity | `metadata.key` |
| API | `/v2/knowledge` — matched by `key` via list; reads include `path` |
| Create requires | `embedding_model` (internal) or `external_config` (external) |
| Immutable | `embedding_model` → `±` replace |

## Internal vs external

| `type` | Required | Notes |
|---|---|---|
| `internal` (default) | `embedding_model` | platform stores and embeds documents |
| `external` | `external_config` `{name, api_url, api_key}` | platform calls your retrieval API |

## Gotchas

!!! warning "embedding_model is create-time"
    The platform does not re-embed existing documents on model change, so the DSL
    marks `embedding_model` immutable: changing it plans a `±` **replace** — delete +
    create. Ingested documents live on the old entity and are lost with it; re-ingest
    after the replace. `retrieval_settings` by contrast update in place freely.

!!! note "external_config.api_key is a secret"
    Write it as `api_key: ${env.RETRIEVAL_API_KEY}` — resolved at apply time, never
    stored in state. On `pull`, the field is redacted to an `${env.*}` placeholder
    with a warning naming it; set the variable before the next apply.

- Attach to agents with `knowledge_bases: [{ref: engineering-kb}]` plus the
  `query_knowledge_base` tool — the engine translates the ref to the server's
  `knowledge_id`. See [Agent](agent.md).
- Populate documents with the SDK, the UI, or companion tooling; the manifest only
  pins the retrieval configuration.
