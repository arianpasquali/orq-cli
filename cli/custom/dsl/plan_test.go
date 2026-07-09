package dsl

import (
	"bytes"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// planStack writes a 4-resource stack: project + kb + evaluator + agent(refs).
func planStack(t *testing.T) (string, StackConfig) {
	dir, _ := stackDir(t)
	writeFile(t, dir, "project.yaml", `apiVersion: orq.ai/v1
kind: Project
metadata: { name: demo-project }
spec: { description: Demo }
`)
	writeFile(t, dir, "kb.yaml", `apiVersion: orq.ai/v1
kind: KnowledgeBase
metadata: { key: eng-kb }
spec:
  type: internal
  description: docs
  embedding_model: mistral/mistral-embed
`)
	writeFile(t, dir, "eval.yaml", `apiVersion: orq.ai/v1
kind: Evaluator
metadata: { key: guard }
spec:
  type: python_eval
  output_type: boolean
  code: "def evaluate(): return True"
`)
	writeFile(t, dir, "agent.yaml", `apiVersion: orq.ai/v1
kind: Agent
metadata: { key: companion, display_name: Companion }
spec:
  role: Assistant
  description: d
  instructions: i
  model: ${var.model}
  settings:
    max_iterations: 5
    guardrails:
      - ref: guard
        execute_on: input
  knowledge_bases:
    - ref: eng-kb
`)
	cfg, err := LoadStack(dir)
	if err != nil {
		t.Fatal(err)
	}
	return dir, cfg
}

func simClient(t *testing.T, sim *Simulator) *Client {
	t.Helper()
	srv := httptest.NewServer(sim.Handler())
	t.Cleanup(srv.Close)
	t.Setenv("ORQ_API_KEY", "sim-key")
	c := newClientWithBase(srv.URL)
	c.sleep = func(time.Duration) {}
	return c
}

func TestBuildPlanAllCreates(t *testing.T) {
	dir, cfg := planStack(t)
	ms, _, errs := Validate(dir, "", nil)
	if len(errs) != 0 {
		t.Fatalf("validate: %v", errs)
	}
	_ = cfg

	c := simClient(t, NewSimulator())
	plan, err := BuildPlan(ms, cfg, c, nil, "")
	if err != nil {
		t.Fatal(err)
	}
	if plan.Creates != 4 || plan.Updates+plan.Deletes+plan.Replaces != 0 {
		t.Fatalf("counts: %+v", plan)
	}
	// wave order: Project → leaves → Agent
	if plan.Waves[0][0].Kind != "Project" {
		t.Errorf("wave0: %+v", plan.Waves[0])
	}
	last := plan.Waves[len(plan.Waves)-1]
	if last[0].Kind != "Agent" {
		t.Errorf("last wave: %+v", last)
	}

	var buf bytes.Buffer
	RenderPlan(&buf, plan, false)
	out := buf.String()
	for _, want := range []string{"+ Project/demo-project", "+ Agent/companion", "Plan: 4 to create, 0 to update, 0 to delete, 0 to replace."} {
		if !strings.Contains(out, want) {
			t.Errorf("render missing %q:\n%s", want, out)
		}
	}
}

func TestBuildPlanUpdateDriftAndDelete(t *testing.T) {
	dir, cfg := planStack(t)
	ms, _, errs := Validate(dir, "", nil)
	if len(errs) != 0 {
		t.Fatal(errs)
	}

	sim := NewSimulator()
	// Seed live counterparts: project, kb, evaluator identical; agent drifted.
	sim.Seed("/v2/projects", map[string]any{"name": "demo-project", "description": "Demo"})
	kb := sim.Seed("/v2/knowledge", map[string]any{"key": "eng-kb", "type": "internal", "description": "docs",
		"embedding_model": "mistral/mistral-embed", "path": "demo-project"})
	ev := sim.Seed("/v2/evaluators", map[string]any{"key": "guard", "type": "python_eval", "output_type": "boolean",
		"code": "def evaluate(): return True"})
	agent := sim.Seed("/v2/agents", map[string]any{"key": "companion", "display_name": "Companion",
		"role": "Assistant", "description": "d", "instructions": "OLD INSTRUCTIONS",
		"model": "mistral/mistral-large-latest", "path": "demo-project",
		"settings": map[string]any{
			"max_iterations": float64(5),
			"guardrails":     []any{map[string]any{"id": ev["_id"], "execute_on": "input"}},
		},
		"knowledge_bases": []any{map[string]any{"knowledge_id": kb["_id"]}},
	})

	// State knows everything + one stale tool to delete.
	st := &StateDoc{Version: 1, Stack: "test-stack", Revision: 3}
	st.Upsert(StateResource{Kind: "KnowledgeBase", Identity: "KnowledgeBase/eng-kb", ServerID: kb["_id"].(string), Path: "demo-project"})
	st.Upsert(StateResource{Kind: "Evaluator", Identity: "Evaluator/guard", ServerID: ev["_id"].(string), Path: "demo-project"})
	st.Upsert(StateResource{Kind: "Agent", Identity: "Agent/companion", ServerID: agent["_id"].(string), Path: "demo-project"})
	st.Upsert(StateResource{Kind: "Tool", Identity: "Tool/old-tool", ServerID: "tool_gone", Path: "demo-project"})

	c := simClient(t, sim)
	plan, err := BuildPlan(ms, cfg, c, st, "sk_state")
	if err != nil {
		t.Fatal(err)
	}
	// Expect: 1 update (agent instructions + model var differ? model matches var default) — instructions only.
	// kb, evaluator, project noop. 1 delete (Tool/old-tool). Project exists → noop.
	if plan.Updates != 1 || plan.Deletes != 1 || plan.Creates != 0 || plan.Replaces != 0 {
		var buf bytes.Buffer
		RenderPlan(&buf, plan, false)
		t.Fatalf("counts: c=%d u=%d d=%d r=%d\n%s", plan.Creates, plan.Updates, plan.Deletes, plan.Replaces, buf.String())
	}
	var agentChange *Change
	for _, w := range plan.Waves {
		for i := range w {
			if w[i].Kind == "Agent" {
				agentChange = &w[i]
			}
		}
	}
	if agentChange == nil || agentChange.Op != OpUpdate {
		t.Fatalf("agent change: %+v", agentChange)
	}
	if len(agentChange.Paths) != 1 || agentChange.Paths[0] != "instructions" {
		t.Errorf("agent paths: %v (want [instructions] — refs must diff clean)", agentChange.Paths)
	}
}

func TestBuildPlanUnresolvedRef(t *testing.T) {
	dir, cfg := stackDir(t)
	writeFile(t, dir, "agent.yaml", `apiVersion: orq.ai/v1
kind: Agent
metadata: { key: a }
spec:
  role: r
  description: d
  instructions: i
  model: m
  settings:
    guardrails:
      - ref: ghost-evaluator
        execute_on: input
`)
	ms, _, errs := Validate(dir, "", nil)
	if len(errs) != 0 {
		t.Fatal(errs)
	}
	c := simClient(t, NewSimulator())
	_, err := BuildPlan(ms, cfg, c, nil, "")
	if err == nil || !strings.Contains(err.Error(), "unresolved ref") {
		t.Fatalf("err = %v", err)
	}
}
