# Platform API coverage audit: candidate stack kinds and gaps

Sources: docs.orq.ai reference, repo `openapi.json`, and orquesta-web source
(platform-api routes, `publicopenapi/spec.go`, proto Connect services,
workspaces-api/traces-api/webhooks-api handlers). Date: 2026-07-09.

Key architectural fact: the web UI calls the same `/v2/*` paths as the public
API for most features; one token-validation middleware accepts API keys,
management keys, and session JWTs (`apps/platform-api/server/app.go:122-128`).
"Public" is decided by inclusion in `publicopenapi/spec.go` or a proto Connect
service without `private`/`x-cli-hidden` tags. Genuinely internal services:
workspaces-api (integrations), traces-api (annotation queues, human evals),
webhooks-api, control-tower assets.

## Tier A: public API today, implementable as stack kinds

| Candidate kind | Endpoints | Identity | Notes |
|---|---|---|---|
| RoutingRule | `/v2/routing-rules` CRUD | `display_name` (no key) | CEL `expression`, `models_config` (fallback/weighted/round_robin, `integration_id` refs), `priority`, optional `project_id` (null = workspace-global) |
| GuardrailRule | `/v2/guardrail-rules` CRUD | `display_name` | `guardrails[]` reference evaluator ids: `ref:` translation like Agent guardrails |
| Policy | `/v2/policies` CRUD | `display_name` | retry config, limits (budget/requests/tokens per period), evaluators |
| WorkspaceModel (model enabling) | `POST /v2/workspace-models`, `DELETE /v2/workspace-models/{model_id}` | `model_id` | Presence = enabled. **No GET**: read the enabled set via the `enabled` flag on `GET /v2/models` (that is how the UI does it, `models/routes.go:70-121`) |
| AutoRouter | `POST /v2/models/autorouter`, `PATCH /v2/models/autorouter/{id}` | user-settable `key` | `{key, strong_model, economical_model, profile: quality\|balanced\|cost}`; referenced as `<workspace>@orq/<key>`; stored as a Model doc + auto-enabled |
| CustomModel | `POST /v2/models`, `/v2/models/vertex`, `/v2/models/openai-like`, `/v2/models/aws-bedrock` (+ validate helpers, azure-foundry deployments) | key/`_id` per variant | BYOK-adjacent onboarding is public even though provider-key storage is not |
| Budget | `/v2/budgets` CRUD + reset/consumption (proto Connect) | server `_id`; uniqueness on `(scope_kind, scope_target_id)` | DSL identity would be the scope tuple; scope targets use stable business ids (identity external_id, model_id, project_id, api_key_id) |
| Notifier | `/v2/notifiers` CRUD (proto Connect) | `_id` + `display_name` | email / slack_webhook / webhook: the public alternative to internal Webhooks |
| Identity (contact) | `/v2/identities` CRUD (proto Connect) | user-settable `external_id` (unique) | cleanest identity of the lot |
| HumanEval / HumanEvalSet | `/v2/human-evals`, `/v2/human-eval-sets` CRUD | `display_name` (`key` is server-generated, not settable) | Present in the CLI's `openapi.json`, but platform source tags them `private` + `x-cli-hidden` (traces-api): confirm intended publicness before shipping a kind |

Implementation wrinkles shared by the rule kinds (RoutingRule, GuardrailRule,
Policy):

1. Identity is `display_name` with no path on the platform: same
   path+display_name fallback as Prompt/Dataset (platform ask #1 grows).
2. `project_id` is an id, not a name. Mapping `metadata.path` to it needs
   `GET /v2/projects` (403 for project-scoped keys). v1: support
   workspace-global rules (omit `project_id`), add project scoping when the
   key situation allows.

## Tier B: internal-only (UI works, public API cannot): platform asks

| Feature | Internal surface | Why it matters for the stack |
|---|---|---|
| **Integrations / BYOK** | `POST /v2/integrations/connect`, PATCH/DELETE (workspaces-api, session + role permissions, `x-cli-hidden`) | **The biggest gap.** Routing rules and custom models reference `integration_id`, but nothing public can create one. Gateway-as-code is incomplete without it |
| Departments | `/v2/assets/departments` CRUD (control-tower, not in spec.go) | Cost attribution as code; identity is user `name` (≤30) |
| Asset tags | `/v2/assets/tags` CRUD (control-tower) | Same; trace/contact tags are facets, not entities |
| Webhooks | `/v2/webhooks` CRUD (webhooks-api, internal) | Public Notifiers cover most cases; webhook entity itself is internal |
| Annotation queues | `/v2/annotation-queues` CRUD + items (traces-api, `private`) | Review workflows as code blocked |
| Environments | No first-class API at all: field-properties of the `environments` Field via v1 `/fields/:id/properties` (NestJS, Postgres) | Needs a real `/v2/environments` before any tooling can manage it |

## Cross-cutting gaps to raise with platform

1. `/v2/workspace-models` has no list endpoint; the enabled set is only
   readable via the `enabled` flag on `GET /v2/models` (which is also
   `x-speakeasy-ignore`, so SDK-hidden).
2. Identity: budgets, webhooks, annotation queues, human-eval-sets are
   addressable only by server `_id`; human-evals have a `key` that is not
   client-settable. User-settable keys exist only for identities
   (`external_id`), auto-routers (`key`), departments/tags (`name`).
3. Project scoping on rules uses `project_id` (an id): a name-based filter or
   project-key-relative semantics would let project-scoped API keys manage
   their own rules declaratively.
4. `openapi.json` in this repo is stale: missing projects, routing-rules,
   guardrail-rules, policies, workspace-models, budgets, notifiers. Refresh
   from the platform spec pipeline.

## Proposed kind waves

- **Wave 1 (rules):** RoutingRule, GuardrailRule, Policy: same list+match
  engine path as Dataset, evaluator `ref:` translation already exists.
- **Wave 2 (gateway):** AutoRouter (clean key identity), WorkspaceModel
  (presence kind; needs the models-list read path).
- **Wave 3 (org):** Identity, Notifier, Budget (scope-tuple identity).
- **Blocked on platform:** Integration (BYOK), Department, AssetTag,
  AnnotationQueue, Webhook, Environment.
