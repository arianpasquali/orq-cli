# Multi-environment

Same manifests, different workspaces: dev, staging, prod are the **same stack**
applied with different credentials and different variable values. Nothing is
duplicated; environments differ only where a `${var.*}` says they may.

## Layout

```
workspace/
├── orq.yaml                 # variables: block holds the dev-friendly defaults
├── agents/ …                # one set of manifests, parameterized
└── vars/
    ├── dev.yaml
    ├── staging.yaml
    └── prod.yaml
```

```yaml title="orq.yaml"
stack: acme-platform         # same stack name everywhere — state lives per workspace
defaults:
  path: acme
variables:
  default_model: mistral/mistral-small-latest    # cheap default for dev
  reranker: ""
```

```yaml title="vars/prod.yaml"
default_model: mistral/mistral-large-latest
reranker: cohere/rerank-v3.5
```

Variable precedence, later wins: `orq.yaml` `variables:` → `--var-file` → `--var k=v`
→ `ORQ_VAR_<name>` env var. So `orq.yaml` holds defaults, var-files hold per-env
values, `--var`/`ORQ_VAR_*` handle one-off overrides.

## Target an environment

The workspace is selected by the **credential**, not by anything in the files:

```console
$ ORQ_API_KEY=$DEV_KEY     orq stack apply -f . --var-file vars/dev.yaml
$ ORQ_API_KEY=$STAGING_KEY orq stack apply -f . --var-file vars/staging.yaml
$ ORQ_API_KEY=$PROD_KEY    orq stack apply -f . --var-file vars/prod.yaml
```

Each workspace gets its own state skill (`orq_dsl_state_acme_platform`), so the three
environments reconcile independently — the same stack name never collides across
workspaces.

## Secrets: `${env.*}`, never var-files

Var-files are committed; secrets are not. Anything sensitive — MCP auth headers,
external KB api keys — goes through `${env.*}`:

```yaml
mcp:
  headers:
    Authorization: { value: "Bearer ${env.LINEAR_API_KEY}", encrypted: true }
```

`${env.*}` values are read from the process environment at plan/apply time and are
never written to state, plan output, or pulled files. Each environment injects its own
value (`LINEAR_API_KEY` from the dev vault vs the prod vault) without the manifests
changing at all. An unset variable fails validation before anything is written.

!!! note "What can vary, what cannot"
    `${var.*}` substitutes **scalar values** only — model ids, temperatures, top_k,
    descriptions. Keys, kinds, paths-as-identity and field names stay literal, so an
    agent is recognizably the same resource in every environment. If two environments
    need *structurally* different resources, that is two manifests (or two stacks),
    not a variable.

!!! warning "Keep var-files out of the manifest walk"
    The loader skips the `vars/` directory by name. Put var-files there — a var-file
    elsewhere ending in `.yaml` would be parsed as a manifest and fail envelope
    validation.

## In CI

One job per environment, keyed by branch or environment gate — dev applies on merge,
prod behind an approval — all running the identical `apply -f . --var-file
vars/<env>.yaml --auto-approve`. See [CI / GitOps](ci-gitops.md) for the workflow
skeletons.
