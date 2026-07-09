package dsl

import (
	"reflect"
	"strings"
	"testing"
)

func TestDiffSpecBasics(t *testing.T) {
	desired := map[string]any{
		"role":  "Assistant",
		"model": "mistral/large",
		"settings": map[string]any{
			"max_iterations": 10,
			"tools":          []any{map[string]any{"type": "current_date"}},
		},
	}
	live := map[string]any{
		"role":  "Assistant",
		"model": "openai/gpt-4o", // changed
		"settings": map[string]any{
			"max_iterations": float64(10), // yaml int vs json float: equal
			"tools":          []any{map[string]any{"type": "current_date"}},
			"max_cost":       float64(0), // live-only: ignored
		},
		"version": "1.2.3", // live-only: ignored
	}
	got := DiffSpec(desired, live, "")
	want := []string{"model"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("DiffSpec = %v, want %v", got, want)
	}
}

func TestDiffSpecNestedAndSlices(t *testing.T) {
	desired := map[string]any{
		"settings": map[string]any{
			"max_iterations": 5,
			"tools":          []any{map[string]any{"type": "a"}, map[string]any{"type": "b"}},
		},
	}
	live := map[string]any{
		"settings": map[string]any{
			"max_iterations": float64(9),
			"tools":          []any{map[string]any{"type": "b"}, map[string]any{"type": "a"}}, // order matters
		},
	}
	got := DiffSpec(desired, live, "")
	want := []string{"settings.max_iterations", "settings.tools"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v", got)
	}
}

func TestDiffSpecMissingKey(t *testing.T) {
	got := DiffSpec(map[string]any{"ttl": 60}, map[string]any{}, "")
	if !reflect.DeepEqual(got, []string{"ttl"}) {
		t.Fatalf("got %v", got)
	}
}

func TestClassify(t *testing.T) {
	info, _ := Lookup("MemoryStore")
	m := &Manifest{Kind: "MemoryStore", Metadata: Metadata{Key: "mem"},
		Spec: map[string]any{"description": "d", "embedding_config": map[string]any{"model": "new-model"}}}

	if ch := Classify(m, nil, "", info, ""); ch.Op != OpCreate {
		t.Errorf("create: %+v", ch)
	}

	same := map[string]any{"description": "d", "embedding_config": map[string]any{"model": "new-model"}, "_id": "x", "created": "t"}
	if ch := Classify(m, same, "x", info, ""); ch.Op != OpNoop {
		t.Errorf("noop: %+v", ch)
	}

	desc := map[string]any{"description": "old", "embedding_config": map[string]any{"model": "new-model"}}
	if ch := Classify(m, desc, "x", info, ""); ch.Op != OpUpdate || len(ch.Paths) != 1 {
		t.Errorf("update: %+v", ch)
	}

	emb := map[string]any{"description": "d", "embedding_config": map[string]any{"model": "old-model"}}
	ch := Classify(m, emb, "x", info, "")
	if ch.Op != OpReplace || !strings.Contains(ch.Reason, "embedding_config") {
		t.Errorf("replace: %+v", ch)
	}
}

func TestClassifyEnvelopeDrift(t *testing.T) {
	info, _ := Lookup("MemoryStore") // reads lack path → state is the target
	m := &Manifest{Kind: "MemoryStore", Metadata: Metadata{Key: "mem", Path: "proj/new"},
		Spec: map[string]any{"description": "d"}}
	live := map[string]any{"description": "d"}

	if ch := Classify(m, live, "x", info, "proj/old"); ch.Op != OpUpdate || ch.Paths[0] != "metadata.path" {
		t.Errorf("path drift via state: %+v", ch)
	}
	if ch := Classify(m, live, "x", info, "proj/new"); ch.Op != OpNoop {
		t.Errorf("same state path: %+v", ch)
	}
	if ch := Classify(m, live, "x", info, ""); ch.Op != OpNoop { // unknown state path: no noise
		t.Errorf("no state path: %+v", ch)
	}

	agentInfo, _ := Lookup("Agent") // reads carry path + optional display_name
	a := &Manifest{Kind: "Agent", Metadata: Metadata{Key: "a", Path: "p", DisplayName: "Nice Name"},
		Spec: map[string]any{"role": "r"}}
	aliveSame := map[string]any{"role": "r", "path": "p", "display_name": "Nice Name"}
	if ch := Classify(a, aliveSame, "x", agentInfo, ""); ch.Op != OpNoop {
		t.Errorf("agent noop: %+v", ch)
	}
	aliveDrift := map[string]any{"role": "r", "path": "p", "display_name": "Renamed In UI"}
	if ch := Classify(a, aliveDrift, "x", agentInfo, ""); ch.Op != OpUpdate || ch.Paths[0] != "metadata.display_name" {
		t.Errorf("agent display drift: %+v", ch)
	}
}

func TestSpecHashStable(t *testing.T) {
	m1 := &Manifest{Kind: "Agent", Metadata: Metadata{Key: "a", Path: "p"}, Spec: map[string]any{"n": 1}}
	m2 := &Manifest{Kind: "Agent", Metadata: Metadata{Key: "a", Path: "p"}, Spec: map[string]any{"n": float64(1)}}
	if SpecHash(m1) == "" || SpecHash(m1) != SpecHash(m2) {
		t.Errorf("hash unstable: %q vs %q", SpecHash(m1), SpecHash(m2))
	}
	m3 := &Manifest{Kind: "Agent", Metadata: Metadata{Key: "a", Path: "p2"}, Spec: map[string]any{"n": 1}}
	if SpecHash(m1) == SpecHash(m3) {
		t.Error("path change should change hash")
	}
}

func TestSymbolizeLiveAgent(t *testing.T) {
	idMap := map[string]string{
		"kb_1":  "KnowledgeBase/eng-kb",
		"evl_1": "Evaluator/prompt-injection",
	}
	mcp := map[string]mcpDiscovered{
		"disc_1": {ParentIdentity: "Tool/linear", Name: "list_issues"},
		"disc_2": {ParentIdentity: "Tool/linear", Name: "get_issue"},
	}
	live := map[string]any{
		"knowledge_bases": []any{map[string]any{"knowledge_id": "kb_1"}, map[string]any{"knowledge_id": "kb_unknown"}},
		"settings": map[string]any{
			"guardrails": []any{map[string]any{"id": "evl_1", "execute_on": "input", "sample_rate": float64(100)}},
			"tools": []any{
				map[string]any{"type": "current_date"},
				map[string]any{"type": "http", "key": "jira-lookup"},
				map[string]any{"type": "mcp", "tool_id": "disc_1"},
				map[string]any{"type": "mcp", "tool_id": "disc_2"},
			},
		},
	}
	out := symbolizeLive(live, "Agent", idMap, mcp)

	kbs := out["knowledge_bases"].([]any)
	if !reflect.DeepEqual(kbs[0], map[string]any{"ref": "eng-kb"}) {
		t.Errorf("kb not symbolized: %v", kbs[0])
	}
	if _, hasID := kbs[1].(map[string]any)["knowledge_id"]; !hasID {
		t.Errorf("unknown kb should stay as id: %v", kbs[1])
	}

	settings := out["settings"].(map[string]any)
	g := settings["guardrails"].([]any)[0].(map[string]any)
	if g["ref"] != "prompt-injection" || g["execute_on"] != "input" {
		t.Errorf("guardrail: %v", g)
	}
	if _, hasID := g["id"]; hasID {
		t.Errorf("guardrail id not removed: %v", g)
	}

	tools := settings["tools"].([]any)
	if len(tools) != 3 { // current_date + http + one grouped mcp
		t.Fatalf("tools = %v", tools)
	}
	httpTool := tools[1].(map[string]any)
	if httpTool["ref"] != "jira-lookup" {
		t.Errorf("http tool: %v", httpTool)
	}
	mcpTool := tools[2].(map[string]any)
	if mcpTool["ref"] != "linear" || !reflect.DeepEqual(mcpTool["tools"], []any{"list_issues", "get_issue"}) {
		t.Errorf("mcp group: %v", mcpTool)
	}

	// original untouched
	if _, has := live["settings"].(map[string]any)["guardrails"].([]any)[0].(map[string]any)["ref"]; has {
		t.Error("symbolizeLive mutated input")
	}
}
