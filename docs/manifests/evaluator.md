# Evaluator

LLM-as-judge or Python scorers, attachable to agents as guardrails/evaluators. Two
creatable types: `llm_eval` and `python_eval` (`spec.type` is the discriminator).

```yaml
apiVersion: orq.ai/v1
kind: Evaluator
metadata:
  key: faithfulness
  path: thales-onboarding/evals
spec:
  type: llm_eval
  mode: single                    # or `jury` with jury.judges[]
  model: mistral/mistral-large-latest
  output_type: number             # boolean | categorical | number | string
  prompt: { $file: ./faithfulness-judge.md }
---
apiVersion: orq.ai/v1
kind: Evaluator
metadata:
  key: no-emoji
  path: thales-onboarding/evals
spec:
  type: python_eval
  output_type: boolean            # boolean | number for python
  code: { $file: ./no_emoji.py }
  guardrail_config: { type: boolean, value: true, alert_on_failure: true }
```

## Identity

| | |
|---|---|
| Identity | `metadata.key` |
| API | `/v2/evaluators` — matched by `key` via list |
| Create requires | `type`, plus per-type fields below |
| Immutable | — |

Per-type requirements, enforced offline by `validate`:

| `type` | Required |
|---|---|
| `llm_eval`, `mode: single` (default) | `prompt`, `model` |
| `llm_eval`, `mode: jury` | `prompt`, `jury` (judges list) |
| `python_eval` | `code` |

## Gotchas

!!! warning "llm_eval and python_eval only"
    `http_eval` exists in API *responses* but has no public create endpoint — declaring
    it is a validation error. The message says so explicitly.

!!! note "key ↔ display_name server rename"
    The Evaluator create API stores the manifest's `key` as the entity's display name
    server-side, and reads return it as `key` again. The engine sends `key` and matches
    on `key`; you never see the rename. Practical consequence: an evaluator's key and
    its UI display name are the same string.

- Path is write-only for this kind — reads don't return it; stack state holds the
  declared value, and UI moves won't show as drift.
- Updates must re-send the `type` discriminator; the engine injects it into every
  PATCH automatically.
- Judge prompts and Python code belong in `$file` includes — reviewable diffs, syntax
  highlighting, no YAML escaping.
- Attach to agents with `settings.guardrails: [{ref: no-emoji, execute_on: input}]` or
  `settings.evaluators: [{ref: faithfulness}]` — see [Agent](agent.md).
