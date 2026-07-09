# Dataset

A named container for evaluation/experiment rows. The DSL manages the container; rows
(datapoints) are data plane and out of scope.

```yaml
apiVersion: orq.ai/v1
kind: Dataset
metadata:
  display_name: triage-golden-set   # identity: path + display_name (ask #1)
  path: thales-onboarding/datasets
spec: {}                            # create body is display_name+path only; rows are data plane
```

## Identity

| | |
|---|---|
| Identity | `metadata.path` + `metadata.display_name` |
| API | `/v2/datasets` — matched by `display_name` via list |
| Create requires | — |
| Immutable | — (rename/move = replace) |

## Gotchas

!!! note "spec is empty — on purpose"
    The public create body takes nothing beyond `display_name` and `path`, so
    `spec: {}` is the complete, correct manifest. It is not a placeholder. Fill rows
    with the SDK/UI/companion tooling.

!!! warning "No unique identity on the platform — state is authoritative"
    Datasets have no key, and the platform doesn't enforce display-name uniqueness:
    **two datasets named `triage-golden-set` can coexist.** Once applied, stack state
    pins the exact server id, so the stack always addresses the right one. But when a
    manifest is *not* yet in state (first plan, adopted after pull) and multiple live
    datasets match the name, plan fails:

    ```
    Dataset/…|triage-golden-set: 2 live resources match identity "triage-golden-set"
    — the workspace has duplicates; delete the extras or import the right one into state
    ```

    Delete the duplicates, or rename yours to something unique before adopting.

- Reads don't return `path` — the declared value lives in stack state (write-only).
- Renaming a dataset replaces it: the new container starts empty, and rows stay on
  the deleted one. Treat dataset renames as intentional data operations.
