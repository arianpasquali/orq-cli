# Limitations & platform asks

What v1 deliberately doesn't do, what it can't do yet, and the platform changes that
would remove each caveat. Nothing here blocks the nine shipped kinds.

## Deployment is gated

The public v2 API exposes list/get-config/invoke for deployments — **no create or
update**. The kind is designed (identity `key`, tier 2) but declaring
`kind: Deployment` is a validate-time error naming this gap:

```
✗ kind Deployment is not provisionable: Deployments have no public
  create/update API (platform ask #4); manage them in the UI for now
```

## Path is write-only on four kinds

Prompt, Dataset, MemoryStore, and Evaluator reads never return `path`. The engine
sends the declared path on create and records it in stack state; it never diffs path
against live for these kinds. Consequences:

- Moving one of these resources to another folder in the UI is **invisible drift**.
- `pull` recovers their paths from state when available, else writes the default with
  a warning (`path not recoverable from the API (platform gap)`).

## Dataset has no unique identity

No user-settable key, and display names are not unique — the platform happily holds
two datasets named `golden-set`. Stack state (the recorded `server_id`) is the only
authoritative identity once applied; before that, an ambiguous match fails the plan
and asks you to clean up. See [Dataset](manifests/dataset.md).

## State writes have no CAS

The advisory revision guard catches concurrent applies by re-reading before writing,
but a true race inside that window can interleave. Serialize applies per stack in CI;
re-running always converges. See [State internals](state-internals.md#the-revision-guard).

## No templating logic

`${var.*}`, `${env.*}`, `$file` — that's the entire language. No loops, no
conditionals, no functions, no cross-manifest expressions. This is a feature (manifests
stay diffable and reviewable); if you need twenty near-identical agents, generate the
YAML with your own tooling and let the DSL own reconciliation.

## Also out of scope in v1

- **Data plane** — KB document ingestion, dataset rows (companion tooling, not DSL).
- **Admin plane** — members, groups, SSO/SCIM, API keys, budgets.
- **Runtime** — experiment runs, logs, traces, invocations.
- **Version promotion** — updates take whatever version behavior the public update
  endpoint has; no Draft→Live orchestration (possible `orq promote` later).

## Platform asks

The four changes that would erase the caveats above, in impact order:

| # | Ask | Removes |
|---|---|---|
| 1 | User-settable `key` on **Prompt, Dataset, Skill** | the `path + display_name` identity fallback; rename-is-replace on three kinds; the dataset duplicate ambiguity |
| 2 | Native **`/v2/stacks`** inventory API (with CAS) | the reserved-Skill state mechanism and the advisory-only guard; document shape migrates as-is |
| 3 | **Labels** on all kinds | enables `--selector` filtered plans and label-based ownership queries |
| 4 | Public **deployment write** endpoints | the Deployment gate — the kind is already designed and registered |

None of them block v1; each lands as a strict simplification of what already ships.
