# State internals

Full reconcile needs an inventory: what does this stack *own*, and which server id
does each declared identity map to? That inventory is the stack state. This page is
the maintenance manual.

## Where it lives: a reserved Skill

State is a JSON document stored in the `instructions` field of a Skill named

```
orq_dsl_state_<stack>        # dashes in the stack name become underscores
```

so stack `acme-platform` keeps its state in the skill `orq_dsl_state_acme_platform`
(skill names forbid dashes — hence the stack-name rule `^[a-z][a-z0-9-]*$`).

**Why a Skill?** The public API has no writable key/value store — RemoteConfigs looked
like the natural home but are **read-only through the public API**. Skills tick every
box a small state document needs:

- workspace-unique, DB-enforced `display_name`;
- directly addressable by that name (`GET /v2/skills/{name}` — no list-and-scan);
- a free-text `instructions` field that PATCHes in place.

Server-side state means no state file in git (no merge conflicts, no leaked ids),
identical behavior from laptops and CI, and one shared reference point per stack.
Platform ask #2 — a native `/v2/stacks` API — replaces this mechanism; the document
shape migrates as-is.

The state skill is invisible to the engine's own inventories: never pulled, never
planned, rejected as a manifest name (`orq_dsl_state_*` is reserved), and removed last
by `destroy`.

## The document

```json
{
 "version": 1,
 "stack": "acme-platform",
 "revision": 14,
 "resources": [
  {
   "kind": "Agent",
   "identity": "Agent/eng-companion",
   "server_id": "agent_0014",
   "path": "acme/agents",
   "spec_hash": "sha256:8090ec89b464f136",
   "applied_at": "2026-07-09T06:54:38Z"
  }
 ]
}
```

| Field | Role |
|---|---|
| `version` | document schema version (`1`) |
| `stack` | owning stack name |
| `revision` | monotonic counter, bumped on every save — the concurrency guard |
| `resources[].kind` / `identity` | the declarative address (`Manifest.Identity()`) |
| `resources[].server_id` | the live entity's id — the identity→id map that makes renames, deletes, and duplicate display names unambiguous |
| `resources[].path` | declared path at last apply — authoritative for kinds whose reads don't return `path` (Prompt, Dataset, MemoryStore, Evaluator) |
| `resources[].spec_hash` | sha256 fingerprint of the applied manifest (kind + identity + path + spec) — cheap change detection |
| `resources[].applied_at` | RFC3339, informational |

State is saved **after every successful operation**, not at the end of apply — an
interrupted run loses at most the in-flight operation, and re-running converges.

Inspect it anytime:

```console
$ orq dsl state list -f .            # -o json for the raw document
```

## The revision guard

The platform has no compare-and-swap, so writes use an **advisory** guard: before
PATCHing, the engine re-reads the live document; if its `revision` isn't the one this
run started from, someone else applied in the meantime:

```
state conflict: another apply moved the stack to revision 7 (this run started
from 5) — re-run to reconcile
```

The failing run stops without writing; re-running re-plans from fresh state and
converges. Advisory means a true race inside the read-check-write window can still
interleave — in practice, serialize applies per stack in CI (a `concurrency:` group)
and treat the guard as the backstop, not the lock. A CAS-capable state endpoint is
part of platform ask #2.

## Repair

Symptoms and remedies, in escalating order:

- **See what the stack thinks it owns:** `orq dsl state list -f .` — compare
  `server_id`s with reality before anything drastic.
- **A resource was deleted in the UI:** harmless. The stored id 404s, the engine falls
  through to rediscovery, and plan shows a `+` create. Apply heals it.
- **A resource was recreated by hand (new id, same key):** also self-healing —
  identity lookup finds it and the next apply re-records the id.
- **State skill holds invalid JSON** (someone edited it): `load state: skill
  orq_dsl_state_<stack> holds invalid JSON — repair or delete it`. Fix the JSON in
  place, or delete the skill and re-adopt (below).
- **Deleting the state skill:** this **orphans** the stack's resources — the DSL
  forgets it ever owned them — but **never deletes any user resource**. The next apply
  rediscovers live resources by identity and **adopts every unchanged match back into
  state** with no API writes (see [the migration guide](guide/migrate-pull.md)), so a
  single apply rebuilds the inventory. What is permanently lost: the recorded paths
  for kinds whose reads lack `path` (they reset to the manifest's declared path), and
  delete tracking for resources whose manifests were already removed.

!!! warning "Don't edit the state skill"
    Its description says so: `orq dsl stack state — managed by 'orq dsl', do not
    edit`. Every hand edit risks the invalid-JSON error above; there is nothing in the
    document worth editing that a re-apply doesn't compute more accurately.
