# Use cases

Three audiences drove the design. Each maps to a workflow that exists today.

## Enterprise: per-team GitOps

A platform team runs the orq workspace; product teams ship agents. The isolation
primitive is the **project-scoped API key** (the pattern built with Thales):

- Platform team creates one project per product team, hands each a project-scoped key.
- Each team keeps a stack in its own repo: manifests reviewed like code, `validate`
  on every PR (offline, no credentials in the PR context), `plan` output as the review
  artifact, `apply --auto-approve` on merge.
- The scoped key confines every read and write to the team's project — a team's
  pipeline *cannot* touch another team's assets, by credential rather than convention.
- The platform team keeps a workspace-scoped stack for shared assets (common
  evaluators, org-wide tools) that team stacks reference by key as pre-existing,
  unmanaged resources.

Audit trail = git history. Rollback = `git revert` + apply. Onboarding a new team =
one project, one key, one repo template. Full pipeline in [CI / GitOps](guide/ci-gitops.md).

## Internal: demo and workspace seeding

Demos, PoCs, and onboarding workspaces are built, mutated during the meeting, and left
to rot. As a stack:

```console
$ orq stack apply -f demos/support-triage --auto-approve    # identical workspace, every time
$ orq stack destroy -f demos/support-triage --auto-approve  # zero residue afterwards
```

- A demo is a directory: project, agents, evaluators, KBs, tools — reproducible on any
  workspace in one command, version-controlled so "the demo that worked last month"
  is a git tag.
- Vertical templates (support triage, doc QA, engineering companion) become copyable
  stack directories — a stack registry in a git repo.
- `pull` works the other way: a workspace improvised during a customer session gets
  serialized into files before it's lost ("iterated in the UI, now commit it").
- The [simulator](quickstart.md#risk-free-the-simulator) lets you rehearse the whole
  lifecycle with zero production traffic.

## SDK developers: prompts and agents next to the code

The app calls agents and prompts by key; the stack puts the definitions in the same repo
as the callers:

```
repo/
├── src/app.py                 # invokes agent "eng-companion" via the SDK
└── workspace/
    ├── orq.yaml
    ├── agents/eng-companion.yaml
    └── prompts/classify-ticket.yaml
```

- One PR changes the prompt *and* the code that consumes it — reviewed together,
  merged together, deployed together (`apply` in the same pipeline).
- `spec` mirrors the v2 API body verbatim, so nothing new to learn beyond the SDK's
  own field names; instructions and judge prompts live in plain Markdown via `$file`.
- Environments follow the app's own promotion flow: same manifests, per-env var-files
  and keys ([multi-environment](guide/multi-env.md)).
- Drift is visible: if someone tunes the agent in the UI, the next `plan` in CI exits
  `2` and shows exactly which fields moved — the repo stays the source of truth.
