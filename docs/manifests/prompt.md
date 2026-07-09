# Prompt

A versioned prompt template: model, parameters, and messages.

```yaml
apiVersion: orq.ai/v1
kind: Prompt
metadata:
  display_name: classify-ticket   # identity: path + display_name (ask #1)
  path: thales-onboarding/prompts
spec:
  prompt:
    model: mistral/mistral-large-latest
    temperature: 0.2
    response_format: { type: json_object }
    messages:
      - role: system
        content: { $file: ./classify-system.md }
      - role: user
        content: "{{ticket_body}}"
  metadata: { use_cases: [Classification], language: English }
```

## Identity

| | |
|---|---|
| Identity | `metadata.path` + `metadata.display_name` (rendered `Prompt/<path>\|<name>`) |
| API | `/v2/prompts` — matched by `display_name` via list (case-insensitive) |
| Create requires | `prompt.messages` |
| Immutable | — (renaming or moving = replace, see below) |

## Gotchas

!!! warning "No user-settable key (platform ask #1)"
    The Prompt create body takes only `display_name` + `path`, so identity falls back
    to `path + display_name`. Renaming the prompt or changing its path is a
    **replace** — the old prompt (and its version history) is deleted and a new one
    created. Platform ask #1 (a real `key` on Prompt) removes this fallback.

!!! note "Path is write-only"
    Prompt reads don't return `path`. The engine sends it on create and remembers the
    declared value in stack state; it never diffs path against live for this kind.
    Consequence: a prompt moved to another folder in the UI won't show as drift.

- `spec.prompt` is the whole model configuration — the same shape the
  `/v2/prompts` API documents (`model`, `temperature`, `messages`, `response_format`,
  …). Nothing is renamed.
- `{{variable}}` placeholders inside message content are **platform** template syntax,
  passed through verbatim. Only `${var.*}` / `${env.*}` / `$file` are DSL constructs.
- Long system messages read best as `$file` includes — the file's content is inlined
  as a string at plan time.
- Versioning: updates go through the public update endpoint; the DSL does not
  orchestrate Draft→Live promotion in v1.
