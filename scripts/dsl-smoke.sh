#!/usr/bin/env bash
# Dry smoke: exercise the REAL orq binary against the in-memory workspace
# simulator. Zero production traffic (ORQ_SERVER points at localhost).
set -uo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
WORK="${1:-$(mktemp -d)}"
PORT="${PORT:-7899}"
BIN="$ROOT/bin/orq"
STACK_DIR="$WORK/stack"
PULL_DIR="$WORK/pulled"

step() { printf '\n————— %s —————\n' "$*"; }
expect_exit() { # expect_exit <want> <cmd...>
  local want="$1"; shift
  "$@"; local got=$?
  if [ "$got" != "$want" ]; then echo "EXPECTED exit $want, got $got: $*"; exit 1; fi
  echo "[exit $got — as expected]"
}

step "build"
(cd "$ROOT" && go build -o bin/orq ./cmd/orq) || exit 1

step "start simulator :$PORT"
(cd "$ROOT" && exec go run ./cli/custom/dsl/simserver -port "$PORT") &
SIM_PID=$!
trap 'kill $SIM_PID 2>/dev/null; pkill -f "simserver -port $PORT" 2>/dev/null' EXIT
sleep 2
export ORQ_SERVER="http://127.0.0.1:$PORT"
export ORQ_API_KEY="dry-smoke-key"
export SMOKE_LINEAR_KEY="not-a-real-token"

step "init"
mkdir -p "$STACK_DIR"
expect_exit 0 "$BIN" stack init -f "$STACK_DIR" --stack orq-dsl-smoke --project orq-dsl-smoke
rm "$STACK_DIR/agents/example-agent.yaml"

step "write smoke stack (9 kinds)"
cat > "$STACK_DIR/orq.yaml" <<'YAML'
stack: orq-dsl-smoke
defaults:
  path: orq-dsl-smoke
variables:
  default_model: mistral/mistral-large-latest
YAML
cat > "$STACK_DIR/project.yaml" <<'YAML'
apiVersion: orq.ai/v1
kind: Project
metadata: { name: orq-dsl-smoke }
spec: { description: Dry-smoke throwaway project }
YAML
cat > "$STACK_DIR/prompt.yaml" <<'YAML'
apiVersion: orq.ai/v1
kind: Prompt
metadata: { display_name: smoke-classify }
spec:
  prompt:
    model: mistral/mistral-large-latest
    messages:
      - role: system
        content: Classify the ticket.
YAML
cat > "$STACK_DIR/kb.yaml" <<'YAML'
apiVersion: orq.ai/v1
kind: KnowledgeBase
metadata: { key: smoke-kb }
spec:
  type: internal
  description: Smoke KB
  embedding_model: mistral/mistral-embed
YAML
cat > "$STACK_DIR/dataset.yaml" <<'YAML'
apiVersion: orq.ai/v1
kind: Dataset
metadata: { display_name: smoke-golden }
spec: {}
YAML
cat > "$STACK_DIR/memory.yaml" <<'YAML'
apiVersion: orq.ai/v1
kind: MemoryStore
metadata: { key: smoke_memory }
spec:
  description: Smoke memory store
  embedding_config: { model: mistral/mistral-embed }
  ttl: 3600
YAML
cat > "$STACK_DIR/skill.yaml" <<'YAML'
apiVersion: orq.ai/v1
kind: Skill
metadata: { display_name: smoke_playbook }
spec:
  description: Smoke playbook
  tags: [smoke]
  instructions: Always be smoking (tests).
YAML
cat > "$STACK_DIR/evals.yaml" <<'YAML'
apiVersion: orq.ai/v1
kind: Evaluator
metadata: { key: smoke-judge }
spec:
  type: llm_eval
  mode: single
  model: ${var.default_model}
  output_type: number
  prompt: "Rate 0..1: {{log.output}}"
---
apiVersion: orq.ai/v1
kind: Evaluator
metadata: { key: smoke-python-guard }
spec:
  type: python_eval
  output_type: boolean
  code: "def evaluate(log): return True"
  guardrail_config: { type: boolean, value: true }
YAML
cat > "$STACK_DIR/tools.yaml" <<'YAML'
apiVersion: orq.ai/v1
kind: Tool
metadata: { key: smoke-http }
spec:
  type: http
  description: HTTP lookup
  http:
    blueprint: { url: "https://example.com/{{id}}", method: GET }
    arguments:
      id: { type: string, description: The id }
---
apiVersion: orq.ai/v1
kind: Tool
metadata: { key: smoke-linear }
spec:
  type: mcp
  description: Linear MCP (simulated)
  mcp:
    server_url: https://mcp.example.com/mcp
    connection_type: http
    headers:
      Authorization: { value: "Bearer ${env.SMOKE_LINEAR_KEY}", encrypted: true }
YAML
cat > "$STACK_DIR/agent.yaml" <<'YAML'
apiVersion: orq.ai/v1
kind: Agent
metadata:
  key: smoke-companion
  display_name: Smoke Companion
spec:
  role: Assistant
  description: Dry-smoke agent exercising every ref type.
  instructions: |
    You are the smoke-test companion.
  model: ${var.default_model}
  settings:
    max_iterations: 5
    tools:
      - type: current_date
      - type: http
        ref: smoke-http
      - type: mcp
        ref: smoke-linear
        tools: [list_issues]
    guardrails:
      - ref: smoke-python-guard
        execute_on: input
        sample_rate: 100
    evaluators:
      - ref: smoke-judge
        execute_on: output
        sample_rate: 50
  knowledge_bases:
    - ref: smoke-kb
  memory_stores: [smoke_memory]
YAML

step "validate"
expect_exit 0 "$BIN" stack validate -f "$STACK_DIR"

step "validate via legacy alias (orq dsl)"
expect_exit 0 "$BIN" dsl validate -f "$STACK_DIR"

step "plan #1 — everything is new (expect exit 2)"
expect_exit 2 "$BIN" stack plan -f "$STACK_DIR"

step "apply #1"
expect_exit 0 "$BIN" stack apply -f "$STACK_DIR" --auto-approve

step "plan #2 — idempotence (expect exit 0, no changes)"
expect_exit 0 "$BIN" stack plan -f "$STACK_DIR"

step "mutate: agent instructions + evaluator prompt"
perl -pi -e 's/You are the smoke-test companion./You are the UPDATED smoke-test companion./' "$STACK_DIR/agent.yaml"
perl -pi -e 's/Rate 0..1:/Rate strictly 0..1:/' "$STACK_DIR/evals.yaml"

step "plan #3 — drift (expect exit 2, two updates)"
expect_exit 2 "$BIN" stack plan -f "$STACK_DIR"

step "apply #2"
expect_exit 0 "$BIN" stack apply -f "$STACK_DIR" --auto-approve
expect_exit 0 "$BIN" stack plan -f "$STACK_DIR"

step "pull → fresh dir, then round-trip plan (expect no changes)"
mkdir -p "$PULL_DIR"
expect_exit 0 "$BIN" stack pull --project orq-dsl-smoke --stack orq-dsl-smoke --out "$PULL_DIR"
cp "$STACK_DIR/orq.yaml" "$PULL_DIR/orq.yaml"
cp "$STACK_DIR/project.yaml" "$PULL_DIR/project.yaml"
export SMOKE_LINEAR_AUTHORIZATION="Bearer not-a-real-token"
export SMOKE_COMPANION_DUMMY=1
# redaction placeholder is ${env.SMOKE_LINEAR_AUTHORIZATION}
expect_exit 0 "$BIN" stack plan -f "$PULL_DIR"

step "remove dataset manifest → plan shows delete → apply"
rm "$STACK_DIR/dataset.yaml"
expect_exit 2 "$BIN" stack plan -f "$STACK_DIR"
expect_exit 0 "$BIN" stack apply -f "$STACK_DIR" --auto-approve

step "state list"
expect_exit 0 "$BIN" stack state list -f "$STACK_DIR"

step "destroy"
expect_exit 0 "$BIN" stack destroy -f "$STACK_DIR" --auto-approve

step "DONE — dry smoke passed"
exit 0
