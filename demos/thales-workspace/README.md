# Thales Engineering Companion — as a stack

The [orq-thales-demo-workspace](https://github.com/orq-ai/orq-thales-demo-workspace)
onboarding pipeline, restated declaratively. What used to be seven imperative,
order-sensitive scripts is now one stack: `orq stack apply` reconciles the whole
workspace, `plan` shows drift (a colleague toggling tools in the UI), `destroy`
replaces `workspace_teardown.py`.

## Mapping

| Original (imperative) | Here (declarative) |
|---|---|
| `thales-onboarding` project, created by hand in the UI | `project.yaml` — stack-owned, created first, deleted last |
| `agents_provision.py` + `workspace/agents/*.yaml` | `agents/*.yaml` |
| `mcp_provision.py` phase 1 (create servers) | `tools/*.yaml` |
| `mcp_provision.py` phase 2 `--attach` (allowlists) | `settings.tools: [{type: mcp, ref, tools: [...]}]` on the agent |
| `guardrails_assign.py` (merge into live settings) | `settings.guardrails` / `settings.evaluators` refs on the agent |
| `evaluators_provision.py` + `workspace/evaluators/*.yaml` | `evaluators/*.yaml` (prompts and code as `$file` includes) |
| `knowledge_bases_provision.py` | `knowledge/engineering-kb.yaml` |
| `datasets_provision.py` (container part) | `datasets/golden.yaml` |
| the "re-provisioning clobbers MCP attachments" gotcha | gone — the agent manifest owns its complete tool set |

The original's worst failure mode — `make provision-agent` silently dropping the MCP
tools, fixed by remembering to re-run `make provision-mcp-attach` — cannot happen here:
every apply writes the full desired agent.

## Deliberately out of scope (data plane)

- **KB document ingestion** — the 5 product markdown files, 1500-char chunking
  (`knowledge_bases_ingest.py` still does this after apply).
- **Dataset rows** — the 16 golden datapoints from `evals/golden_dataset.jsonl`
  (`datasets_provision.py` loader, SDK, or UI).
- **The Jira demo board** — `jira_provision.py` provisions Jira Cloud, not orq.
- **Experiments** — `experiments_run.py` runs them; the stack manages the scorers it uses.
- **Jira MCP allowlist** — `tools/jira-mcp.yaml` creates the server, but no agent
  references it yet (the original's allowlist was empty pending tool-name discovery).

## Run it

Note: this stack declares a `kind: Project` manifest (stack-owned project), which
needs a **workspace-scoped** API key — UI-minted project-scoped keys 403 on
`/v2/projects`. With a project-scoped key, delete `project.yaml` and point
`defaults.path` at the key's project.

```console
$ export ORQ_API_KEY=...          # workspace auth
$ export LINEAR_API_KEY=...      # baked (encrypted) into the Linear MCP header
$ export GITHUB_TOKEN=...        # read-only PAT for the GitHub MCP header
$ export JIRA_API_KEY=...        # Atlassian MCP header

$ orq stack validate               # offline — no credentials needed
$ orq stack plan                   # diff against the live workspace
$ orq stack apply                  # reconcile
```

Apply order is computed from references: project → tools + evaluators + KB + dataset →
agents. After apply, ingest the KB documents and dataset rows with the original repo's
data-plane scripts.
