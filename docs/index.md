# orq Workspace Stacks

**Describe every asset in an orq.ai workspace as YAML files. Reconcile with one command.**

The `orq stack` command group turns a directory of Kubernetes-style manifests into a live orq workspace — and back:

```console
$ orq stack plan -f ./workspace --var-file vars/prod.yaml
stack: acme-platform · 12 live · state rev 14

  + Evaluator/citation-presence
  ~ Agent/eng-companion
      instructions
  − Tool/old-linear-mcp  removed from files · owned by stack

Plan: 1 to create, 1 to update, 1 to delete, 0 to replace.
```

## Why

Workspaces are otherwise assembled by clicking the UI or writing one-off scripts. That
doesn't review, doesn't reproduce, and doesn't scale past one team. Workspace Stacks give orq
what Terraform and Kubernetes gave infrastructure:

- **Files are the source of truth** — prompts, agents, evaluators, knowledge bases live
  in git, next to the code that uses them. Diffs get reviewed like any other change.
- **A plan you read before you commit** — `plan` shows creates/updates/deletes/replaces
  with field-level detail, and exit codes CI can gate on.
- **A reconciler that converges** — `apply` is idempotent; re-running after a partial
  failure finishes the job. Removing a manifest removes the resource (scoped to what
  the stack owns — never your colleagues' work).
- **Two-way** — `pull` serializes a live workspace into manifests: the migration path
  for existing workspaces, and the "iterated in the UI, now commit it" workflow.

## The shape of it

```yaml
apiVersion: orq.ai/v1
kind: Agent
metadata:
  key: eng-companion
  path: acme/agents          # project/folder — folders auto-created
spec:                        # ← verbatim v2 API body, nothing invented
  role: Assistant
  instructions: { $file: ./instructions.md }
  model: ${var.default_model}
  settings:
    tools:
      - type: query_knowledge_base
  knowledge_bases:
    - ref: engineering-kb    # refs by key; engine translates to ids
```

One rule keeps the language small: **`spec` mirrors the public v2 API create/update body,
snake_case, verbatim.** The language adds exactly three constructs — the envelope,
`ref:` references, and `${var}` / `${env}` / `$file` interpolation.

## Nine kinds in v1

Project · Prompt · Agent · Evaluator · KnowledgeBase · Dataset · Tool · MemoryStore · Skill

(Deployment is designed but gated: the public API has no deployment write endpoints yet.)

## Where to go next

- [Quickstart](quickstart.md) — zero to reconciled in five minutes
- [Manifest reference](manifests/index.md) — one page per kind, every field grounded in the API
- [CLI reference](cli.md) — all seven commands with worked examples
- [Architecture](architecture.md) — how the engine actually works
