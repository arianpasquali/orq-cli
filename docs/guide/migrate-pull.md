# Migrate an existing workspace

You have a workspace built by hand — UI clicks, one-off scripts. `pull` turns it into
manifests; a little editing turns those into a managed stack.

## 1. Pull

```console
$ mkdir acme-stack && cd acme-stack
$ orq dsl pull --project acme --out .
written  agents/eng-companion.yaml
written  evaluators/faithfulness.yaml
written  knowledge-bases/engineering-kb.yaml
written  tools/linear-readonly.yaml
⚠ Tool/linear-readonly: mcp.headers.Authorization.value redacted → ${env.LINEAR_READONLY_AUTHORIZATION} — set it before apply
⚠ Prompt/classify-ticket: path not recoverable from the API (platform gap) — wrote default "acme"

pulled 4 resources → .
```

What pull did for you:

- Normalized away server noise (ids, timestamps, versions).
- Symbolized references — agent attachments came out as `ref:` keys and
  `{type: mcp, ref, tools: [...]}` groups, not raw ids.
- Redacted secrets to `${env.*}` placeholders (one warning each — export those
  variables before any apply).
- Recovered paths where the API returns them; kinds whose reads lack `path`
  (Prompt, Dataset, MemoryStore, Evaluator) got the state's value or the default,
  with a warning.

## 2. Make it a stack

Pull writes manifests, not a stack. Add `orq.yaml`:

```yaml
stack: acme-platform
defaults:
  path: acme
```

Optionally add a `project.yaml` if the stack should own the project itself
(pull never emits Project manifests — `--project` is only a filter).

Verify the round-trip before touching anything:

```console
$ orq dsl validate -f .
$ orq dsl plan -f .
stack: acme-platform · 4 live · state rev 0

No changes. Workspace matches the manifests.
```

**No changes** is the contract — pulled files describe exactly what is live.

## 3. Parameterize by hand

Pulled files are literal; variables cannot be reverse-inferred. Sweep through and
extract what should vary:

```yaml
# before (literal)                      # after (parameterized)
model: mistral/mistral-large-latest     model: ${var.default_model}
instructions: |                         instructions: { $file: ./eng-companion.instructions.md }
  You are a grounded companion...
```

Move long prose into `$file` includes, hoist repeated scalars into `orq.yaml`
`variables:`, re-run `plan` after each sweep — it must stay at *No changes*
(interpolation happens before diffing, so a faithful parameterization is invisible).

## 4. Adopt — the first apply takes ownership

!!! note "Unchanged resources are adopted, without API writes"
    `pull` reads; `apply` owns. On apply, any manifest that matches its live resource
    (a *no-op*) is **adopted**: recorded in stack state with its server id and declared
    path, with no API mutation issued. The output marks each one:

    ```console
    adopted  Agent/eng-companion  unchanged, now stack-owned
    ```

    So one `apply` after a faithful pull brings the whole set under management — from
    then on drift detection, file-removal `−` deletes, and `destroy` cover everything.
    Until that first apply, the stack owns nothing: deleting a manifest pre-adoption
    just makes the DSL forget the file; no delete is issued.

!!! warning "Datasets: check for display-name duplicates first"
    Datasets have no unique key on the platform — several can share a display name.
    Before adopting, plan will refuse ambiguous matches
    (`2 live resources match identity …`); delete or rename the extras so each pulled
    dataset matches exactly one live resource. Once in state, the recorded server id
    settles the question permanently. Details: [Dataset](../manifests/dataset.md).

## 5. Commit

```console
$ git init && git add -A && git commit -m "adopt acme workspace as code"
```

From here it's the standard loop — edit, plan, review, apply — and the
[CI / GitOps guide](ci-gitops.md) for automating it. The workspace you clicked
together is now reviewable, reproducible, and reversible.
