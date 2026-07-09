# CI / GitOps

The commands were designed as pipeline stages: `validate` needs no credentials,
`plan` exits `2` when changes are pending, `apply --auto-approve` executes without a
TTY. A stack in git plus two workflows = GitOps for your workspace.

## PR: validate offline, plan as a gate

```yaml title=".github/workflows/pr.yaml"
name: workspace-pr
on: pull_request

jobs:
  validate:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with: { go-version: "1.25" }
      - run: go build -o bin/orq ./cmd/orq
      # No ORQ_API_KEY here — validate is fully offline.
      - run: bin/orq dsl validate -f ./workspace

  plan:
    runs-on: ubuntu-latest
    needs: validate
    environment: production          # scoped secret lives here
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with: { go-version: "1.25" }
      - run: go build -o bin/orq ./cmd/orq
      - name: plan (exit 2 = changes pending)
        env:
          ORQ_API_KEY: ${{ secrets.ORQ_API_KEY }}
        run: |
          set +e
          bin/orq dsl plan -f ./workspace --var-file vars/prod.yaml | tee plan.txt
          code=$?
          set -e
          # 0 = no changes, 2 = changes pending (fine on a PR), 1 = real error
          [ "$code" -eq 1 ] && exit 1
          exit 0
```

Post `plan.txt` as a PR comment and reviewers read the exact creates/updates/deletes —
the same review ergonomics as `terraform plan`. To *require* converged manifests on
main, run plan on a schedule and alert on exit code `2` (drift).

## Merge: apply

```yaml title=".github/workflows/deploy.yaml"
name: workspace-apply
on:
  push:
    branches: [main]
    paths: ["workspace/**"]

concurrency: workspace-apply       # one apply at a time (see note below)

jobs:
  apply:
    runs-on: ubuntu-latest
    environment: production
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with: { go-version: "1.25" }
      - run: go build -o bin/orq ./cmd/orq
      - name: apply
        env:
          ORQ_API_KEY: ${{ secrets.ORQ_API_KEY }}
          LINEAR_API_KEY: ${{ secrets.LINEAR_API_KEY }}   # ${env.*} refs in manifests
        run: bin/orq dsl apply -f ./workspace --var-file vars/prod.yaml --auto-approve
```

- `--auto-approve` replaces the interactive confirmation — gate it with branch
  protection and environment approvals instead.
- Every `${env.*}` reference in the manifests must be exported in this step; unset
  ones fail validation before anything is written.
- A failed apply exits `1` after finishing all independent work — **re-running the
  job converges**; no cleanup step needed.

!!! note "Serialize applies per stack"
    State updates use an advisory revision guard, not CAS
    ([details](../state-internals.md#the-revision-guard)): a concurrent apply fails
    fast with a state-conflict error rather than corrupting anything, but the polite
    pattern is a `concurrency:` group so applies to one stack queue instead of racing.

## Per-team, project-scoped keys

The enterprise pattern (built for a Thales-style org): the platform team creates one
project per product team and hands each team a **project-scoped API key**. Each team's
repo carries its own stack; the key confines everything — paths, reads, writes — to
that project:

```
team-search/workspace/     + ORQ_API_KEY (scoped: project search)    → manages search only
team-support/workspace/    + ORQ_API_KEY (scoped: project support)   → manages support only
platform/workspace/        + workspace-scoped key                    → shared/cross-project assets
```

A team's pipeline physically cannot touch another team's project — isolation comes
from the credential, not from convention. Stack state lives server-side (a reserved
Skill per stack), so nothing is shared between repos and laptops except the workspace
itself.
