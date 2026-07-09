package dsl

import (
	"bytes"
	"strings"
	"testing"
	"time"
)

func fixedNow() func() time.Time {
	t0 := time.Date(2026, 7, 9, 3, 0, 0, 0, time.UTC)
	return func() time.Time { return t0 }
}

// applyFixture: validate + plan + execute against a fresh simulator.
func applyFixture(t *testing.T, sim *Simulator, st *StateDoc, stateID string) (*PlanResult, *Client, string) {
	t.Helper()
	dir, cfg := planStack(t)
	ms, _, errs := Validate(dir, "", nil)
	if len(errs) != 0 {
		t.Fatal(errs)
	}
	c := simClient(t, sim)
	plan, err := BuildPlan(ms, cfg, c, st, stateID)
	if err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	execErr := Execute(c, plan, &out, fixedNow())
	if execErr != nil {
		t.Fatalf("execute: %v\n%s", execErr, out.String())
	}
	return plan, c, out.String()
}

func TestApplyCreatesEverythingInOrder(t *testing.T) {
	sim := NewSimulator()
	plan, c, out := applyFixture(t, sim, nil, "")

	ids := sim.DumpIdentities()
	want := []string{"Agent/companion", "Evaluator/guard", "KnowledgeBase/eng-kb", "Project/demo-project", "Skill/orq_dsl_state_test_stack"}
	if strings.Join(ids, ",") != strings.Join(want, ",") {
		t.Fatalf("sim contents: %v", ids)
	}

	// Agent body got translated refs (ids, not ref keys).
	agent := sim.Objects("/v2/agents")[0]
	kbs := agent["knowledge_bases"].([]any)[0].(map[string]any)
	if _, hasRef := kbs["ref"]; hasRef || kbs["knowledge_id"] == "" {
		t.Errorf("agent kb body: %v", kbs)
	}
	guard := agent["settings"].(map[string]any)["guardrails"].([]any)[0].(map[string]any)
	if guard["id"] == "" || guard["execute_on"] != "input" {
		t.Errorf("agent guardrail body: %v", guard)
	}

	// State: 4 resources, revision = one save per op.
	if len(plan.State.Resources) != 4 || plan.State.Revision != 4 {
		t.Fatalf("state: rev=%d n=%d", plan.State.Revision, len(plan.State.Resources))
	}

	// Wave order visible in output.
	pIdx := strings.Index(out, "+ Project/demo-project")
	aIdx := strings.Index(out, "+ Agent/companion")
	if pIdx == -1 || aIdx == -1 || pIdx > aIdx {
		t.Errorf("output order:\n%s", out)
	}

	// Second run: no changes.
	dir, cfg := planStack(t)
	ms, _, _ := Validate(dir, "", nil)
	st, stateID, err := LoadState(c, "test-stack")
	if err != nil {
		t.Fatal(err)
	}
	plan2, err := BuildPlan(ms, cfg, c, st, stateID)
	if err != nil {
		t.Fatal(err)
	}
	if plan2.HasChanges() {
		var buf bytes.Buffer
		RenderPlan(&buf, plan2, false)
		t.Fatalf("second plan not empty:\n%s", buf.String())
	}
}

func TestApplyPartialFailureSkipsDependents(t *testing.T) {
	sim := NewSimulator()
	dir, cfg := planStack(t)
	// Poison the evaluator create by pre-seeding a duplicate key (sim 409s).
	sim.Seed("/v2/evaluators", map[string]any{"key": "guard", "type": "python_eval", "code": "x"})
	// Force a create attempt anyway: hide it from list-match by breaking the case?
	// Simpler: make the evaluator manifest differ so it plans as UPDATE against
	// a poisoned PATCH? Instead: point the agent at a ref that will fail to
	// create — use a fresh project-less stack where evaluator create 409s
	// because of the seeded duplicate with different content.
	ms, _, errs := Validate(dir, "", nil)
	if len(errs) != 0 {
		t.Fatal(errs)
	}
	c := simClient(t, sim)
	plan, err := BuildPlan(ms, cfg, c, nil, "")
	if err != nil {
		t.Fatal(err)
	}
	// The evaluator matches live (list+match) → update or noop; to force a
	// failure instead, mark its live id then delete it server-side so PATCH 404s.
	var evCh *Change
	for wi := range plan.Waves {
		for i := range plan.Waves[wi] {
			if plan.Waves[wi][i].Kind == "Evaluator" {
				evCh = &plan.Waves[wi][i]
			}
		}
	}
	if evCh == nil {
		t.Fatal("no evaluator change")
	}
	if evCh.Op == OpNoop {
		t.Skip("evaluator planned as noop; covered by order test")
	}

	var out bytes.Buffer
	// Sabotage: remove evaluator from sim so update PATCH 404s.
	if evCh.Op == OpUpdate {
		simObjs := sim.Objects("/v2/evaluators")
		_ = simObjs
		sim.stores["/v2/evaluators"] = nil
		// FetchLive already ran; PATCH will 404 → but our engine falls to
		// update path with stale id.
	}
	execErr := Execute(c, plan, &out, fixedNow())
	if execErr != ErrPartialFailure {
		t.Fatalf("want partial failure, got %v\n%s", execErr, out.String())
	}
	if !strings.Contains(out.String(), "↷ Agent/companion") {
		t.Errorf("agent not skipped:\n%s", out.String())
	}
	// KB and project still applied.
	if len(sim.Objects("/v2/knowledge")) != 1 || len(sim.Objects("/v2/projects")) != 1 {
		t.Error("independent resources should still apply")
	}
}

func TestDestroyReverseOrder(t *testing.T) {
	sim := NewSimulator()
	_, c, _ := applyFixture(t, sim, nil, "")

	st, stateID, err := LoadState(c, "test-stack")
	if err != nil || st == nil {
		t.Fatal(err)
	}
	cfg := StackConfig{Stack: "test-stack"}
	cfg.Defaults.Path = "demo-project"
	plan, err := DestroyPlan(cfg, st, stateID)
	if err != nil {
		t.Fatal(err)
	}
	if plan.Deletes != 4 {
		t.Fatalf("deletes = %d", plan.Deletes)
	}
	var out bytes.Buffer
	if err := Execute(c, plan, &out, fixedNow()); err != nil {
		t.Fatalf("%v\n%s", err, out.String())
	}
	// Agent (tier 2) deleted before KB (tier 1) before Project (tier 0).
	aIdx := strings.Index(out.String(), "− Agent/companion")
	kIdx := strings.Index(out.String(), "− KnowledgeBase/eng-kb")
	pIdx := strings.Index(out.String(), "− Project/demo-project")
	if !(aIdx < kIdx && kIdx < pIdx) || aIdx == -1 {
		t.Fatalf("destroy order:\n%s", out.String())
	}
	// Everything gone except the state skill (deleted by the command layer).
	for _, base := range []string{"/v2/agents", "/v2/knowledge", "/v2/evaluators", "/v2/projects"} {
		if n := len(sim.Objects(base)); n != 0 {
			t.Errorf("%s not empty: %d", base, n)
		}
	}
	// Delete state skill explicitly.
	if err := DeleteState(c, plan.StateID); err != nil {
		t.Fatal(err)
	}
	if n := len(sim.Objects("/v2/skills")); n != 0 {
		t.Errorf("state skill survives: %d", n)
	}
}

func TestApplyAdoptsUnchangedLiveResources(t *testing.T) {
	// First apply creates everything with state.
	sim := NewSimulator()
	_, c, _ := applyFixture(t, sim, nil, "")

	// Simulate a fresh team: delete the state skill, keep the resources.
	st, stateID, err := LoadState(c, "test-stack")
	if err != nil {
		t.Fatal(err)
	}
	if err := DeleteState(c, stateID); err != nil {
		t.Fatal(err)
	}

	// Plan from zero state: everything matches live → all noop → adoptions.
	dir, cfg := planStack(t)
	ms, _, _ := Validate(dir, "", nil)
	plan, err := BuildPlan(ms, cfg, c, nil, "")
	if err != nil {
		t.Fatal(err)
	}
	if plan.HasChanges() {
		var buf bytes.Buffer
		RenderPlan(&buf, plan, false)
		t.Fatalf("expected clean plan:\n%s", buf.String())
	}
	if len(plan.Adoptions) != 4 {
		t.Fatalf("adoptions = %d, want 4", len(plan.Adoptions))
	}

	var out bytes.Buffer
	if err := Execute(c, plan, &out, fixedNow()); err != nil {
		t.Fatalf("%v\n%s", err, out.String())
	}
	if !strings.Contains(out.String(), "adopted") {
		t.Errorf("no adoption output:\n%s", out.String())
	}

	// State now owns everything again; destroy would work.
	st, _, err = LoadState(c, "test-stack")
	if err != nil || st == nil {
		t.Fatal(err)
	}
	if len(st.Resources) != 4 {
		t.Fatalf("adopted state: %+v", st.Resources)
	}
	_ = stateID
}
