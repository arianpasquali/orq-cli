# Skill

Reusable instruction blocks agents can load — playbooks, procedures, domain knowledge.

```yaml
apiVersion: orq.ai/v1
kind: Skill
metadata:
  display_name: ticket_triage_playbook   # letters/digits/underscores only
  path: thales-onboarding/skills
spec:
  description: How to triage support tickets.
  tags: [support, triage]
  instructions: { $file: ./triage-playbook.md }
```

## Identity

| | |
|---|---|
| Identity | `metadata.path` + `metadata.display_name` |
| API | `/v2/skills` — GET/PATCH/DELETE address the **name** directly; reads include `path` |
| Create requires | — |
| Immutable | — (rename/move = replace) |

## Gotchas

!!! warning "Strict name charset"
    Skill names allow letters, digits and underscores only, starting with a letter:
    `^[A-Za-z][A-Za-z0-9_]*$`. No dashes, no spaces. The platform enforces
    workspace-wide uniqueness on the name (which is why the name is directly
    addressable — and why the DSL's own state hides in a Skill).

!!! warning "orq_dsl_state_* is reserved"
    Names starting with `orq_dsl_state_` are reserved for [stack state](../state-internals.md)
    and rejected at validation. State skills are likewise invisible to `pull`, `plan`,
    and `destroy` inventories (destroy removes the stack's own state skill last, as
    bookkeeping).

- No user-settable key (platform ask #1): identity is `path + display_name`; renaming
  is a replace.
- `instructions` is the payload — keep it in a Markdown file via `$file` so the
  playbook diffs like prose, not YAML.
