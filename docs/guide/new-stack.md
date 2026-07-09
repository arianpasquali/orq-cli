# New stack from scratch

Greenfield: nothing exists yet, files will define everything. (For an existing
workspace, start with [pull](migrate-pull.md) instead.)

## 1. Scaffold and lay out

```console
$ mkdir acme-platform && cd acme-platform
$ orq dsl init --stack acme-platform
```

Grow into the conventional layout — one directory per kind, one file per resource,
prose next to the manifests that include it:

```
acme-platform/
├── orq.yaml                    # stack name, defaults.path, variables
├── project.yaml                # kind: Project
├── agents/
│   ├── eng-companion.yaml
│   └── eng-companion.instructions.md    # pulled in via $file
├── evaluators/
│   ├── faithfulness.yaml
│   ├── faithfulness-judge.md
│   └── no_emoji.py
├── knowledge-bases/
│   └── engineering-kb.yaml
├── tools/
│   └── linear-readonly.yaml
└── vars/
    ├── dev.yaml                # never loaded as manifests (vars/ is excluded)
    └── prod.yaml
```

The layout is convention, not requirement — the loader walks everything recursively
and multi-doc files work. But `pull` writes this shape, so matching it keeps
greenfield and migrated stacks looking alike.

## 2. Bottom-up authoring order

Write manifests in dependency order — the same order apply runs them:

1. **Project** — the container; everything's `metadata.path` starts with its name.
2. **Leaves** — KnowledgeBase, MemoryStore, Tool, Evaluator, Prompt, Dataset, Skill.
   No refs between them in v1, so author freely.
3. **Agents** — reference the leaves by key (`ref:`), own their complete
   tool/guardrail/KB/memory set.

Each kind's page in the [manifest reference](../manifests/index.md) has a canonical
example to copy. Keep long text (`instructions`, judge prompts, Python code) in
sibling files via `$file` — they diff like code, not YAML strings.

## 3. Validate early, validate often

```console
$ orq dsl validate -f .
✓ 12 manifests · 9 kinds · schema ok · refs ok · vars ok
```

Offline and instant — wire it into a pre-commit hook or editor task. It catches the
whole class of "would have failed mid-apply" mistakes: missing required fields, dashed
memory-store keys, evaluator type mismatches, unresolved `${var.*}`, missing `$file`
targets, duplicate identities.

## 4. Plan, apply, iterate

```console
$ orq dsl plan -f . --var-file vars/dev.yaml     # read the diff
$ orq dsl apply -f . --var-file vars/dev.yaml    # confirm, execute
```

The loop from here:

- **Edit a manifest** → plan shows `~` with exact field paths → apply.
- **Add a manifest** → `+` create, ordered automatically after its dependencies.
- **Delete a manifest file** → `−` delete on the next plan. Removal is reconciliation,
  not garbage: scoped strictly to what the stack owns.
- **Someone edited the UI** → drift appears as `~` on declared fields; files win.

## 5. Tear down when done

```console
$ orq dsl destroy -f .
```

Reverse dependency order, typed stack-name confirmation, state cleaned up. Ideal for
demo and test workspaces that should leave nothing behind.

!!! note "One stack, one directory, one scope"
    The stack name in `orq.yaml` is the ownership boundary. Two stacks never fight
    over a resource as long as each resource is declared in exactly one stack. Split
    big platforms into multiple stacks (per team, per product) rather than one giant
    directory — see [CI / GitOps](ci-gitops.md) for the per-team pattern.
