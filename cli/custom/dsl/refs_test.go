package dsl

import (
	"reflect"
	"strings"
	"testing"
)

func agentManifest(key string, spec map[string]any) *Manifest {
	if spec == nil {
		spec = map[string]any{}
	}
	return &Manifest{Kind: "Agent", Metadata: Metadata{Key: key, Path: "proj/agents"}, Spec: spec}
}

func TestExtractRefs(t *testing.T) {
	m := agentManifest("a", map[string]any{
		"knowledge_bases": []any{map[string]any{"ref": "kb1"}},
		"memory_stores":   []any{"mem1"},
		"team_of_agents":  []any{map[string]any{"key": "helper"}},
		"settings": map[string]any{
			"guardrails": []any{map[string]any{"ref": "guard", "execute_on": "input"}},
			"tools": []any{
				map[string]any{"type": "current_date"},
				map[string]any{"type": "http", "ref": "jira"},
				map[string]any{"type": "mcp", "ref": "linear", "tools": []any{"list_issues"}},
			},
		},
	})
	refs := extractRefs(m)
	want := map[string]string{ // Kind → Key (one each in this fixture)
		"KnowledgeBase": "kb1", "MemoryStore": "mem1", "Agent": "helper", "Evaluator": "guard",
	}
	toolRefs := 0
	for _, r := range refs {
		if r.Kind == "Tool" {
			toolRefs++
			continue
		}
		if want[r.Kind] != r.Key {
			t.Errorf("unexpected ref %+v", r)
		}
	}
	if toolRefs != 2 || len(refs) != 6 {
		t.Errorf("refs = %+v", refs)
	}
}

func TestBuildWavesOrdering(t *testing.T) {
	kb := Change{Op: OpCreate, Kind: "KnowledgeBase", Identity: "KnowledgeBase/kb1",
		Manifest: &Manifest{Kind: "KnowledgeBase", Metadata: Metadata{Key: "kb1", Path: "proj"}}}
	proj := Change{Op: OpCreate, Kind: "Project", Identity: "Project/proj",
		Manifest: &Manifest{Kind: "Project", Metadata: Metadata{Name: "proj"}}}
	agent := Change{Op: OpCreate, Kind: "Agent", Identity: "Agent/a",
		Manifest: agentManifest("a", map[string]any{
			"knowledge_bases": []any{map[string]any{"ref": "kb1"}},
		})}
	del := Change{Op: OpDelete, Kind: "Tool", Identity: "Tool/old"}
	delAgent := Change{Op: OpDelete, Kind: "Agent", Identity: "Agent/gone"}
	noop := Change{Op: OpNoop, Kind: "Skill", Identity: "Skill/s"}

	waves, err := BuildWaves([]Change{agent, del, kb, noop, proj, delAgent})
	if err != nil {
		t.Fatal(err)
	}
	var shape [][]string
	for _, w := range waves {
		var ids []string
		for _, ch := range w {
			ids = append(ids, string(ch.Op)+":"+ch.Identity)
		}
		shape = append(shape, ids)
	}
	want := [][]string{
		{"create:Project/proj"},
		{"create:KnowledgeBase/kb1"},
		{"create:Agent/a"},
		{"delete:Agent/gone"}, // deletes reverse-tier: Agent (2) before Tool (1)
		{"delete:Tool/old"},
	}
	if !reflect.DeepEqual(shape, want) {
		t.Fatalf("waves = %v, want %v", shape, want)
	}
}

func TestBuildWavesCycle(t *testing.T) {
	a := Change{Op: OpCreate, Kind: "Agent", Identity: "Agent/a",
		Manifest: agentManifest("a", map[string]any{"team_of_agents": []any{map[string]any{"key": "b"}}})}
	b := Change{Op: OpCreate, Kind: "Agent", Identity: "Agent/b",
		Manifest: agentManifest("b", map[string]any{"team_of_agents": []any{map[string]any{"key": "a"}}})}
	_, err := BuildWaves([]Change{a, b})
	if err == nil || !strings.Contains(err.Error(), "cycle") {
		t.Fatalf("err = %v", err)
	}
}

func TestResolveRefs(t *testing.T) {
	r := newRefResolver()
	r.put("KnowledgeBase", "kb1", "kb_id_1")
	r.put("Evaluator", "guard", "evl_id_1")
	r.putMCP("linear", map[string]string{"list_issues": "d1", "get_issue": "d2"})

	m := agentManifest("a", map[string]any{
		"knowledge_bases": []any{map[string]any{"ref": "kb1"}},
		"settings": map[string]any{
			"guardrails": []any{map[string]any{"ref": "guard", "execute_on": "input", "sample_rate": 100}},
			"tools": []any{
				map[string]any{"type": "current_date"},
				map[string]any{"type": "http", "ref": "jira"},
				map[string]any{"type": "mcp", "ref": "linear", "tools": []any{"list_issues", "get_issue"}},
			},
		},
	})
	spec, err := ResolveRefs(m, r)
	if err != nil {
		t.Fatal(err)
	}
	kb := spec["knowledge_bases"].([]any)[0]
	if !reflect.DeepEqual(kb, map[string]any{"knowledge_id": "kb_id_1"}) {
		t.Errorf("kb: %v", kb)
	}
	settings := spec["settings"].(map[string]any)
	g := settings["guardrails"].([]any)[0].(map[string]any)
	if g["id"] != "evl_id_1" || g["execute_on"] != "input" {
		t.Errorf("guardrail: %v", g)
	}
	tools := settings["tools"].([]any)
	if len(tools) != 4 { // current_date + http + 2 expanded mcp
		t.Fatalf("tools: %v", tools)
	}
	if tools[1].(map[string]any)["key"] != "jira" {
		t.Errorf("http tool: %v", tools[1])
	}
	if tools[2].(map[string]any)["tool_id"] != "d1" || tools[3].(map[string]any)["tool_id"] != "d2" {
		t.Errorf("mcp expansion: %v %v", tools[2], tools[3])
	}
	// original manifest keeps refs
	origTools := m.Spec["settings"].(map[string]any)["tools"].([]any)
	if origTools[2].(map[string]any)["ref"] != "linear" {
		t.Error("ResolveRefs mutated the manifest")
	}
}

func TestResolveRefsErrors(t *testing.T) {
	r := newRefResolver()
	m := agentManifest("a", map[string]any{
		"knowledge_bases": []any{map[string]any{"ref": "ghost"}},
	})
	if _, err := ResolveRefs(m, r); err == nil || !strings.Contains(err.Error(), "unresolved ref") {
		t.Fatalf("err = %v", err)
	}

	r.putMCP("linear", map[string]string{"only_tool": "d9"})
	m2 := agentManifest("b", map[string]any{
		"settings": map[string]any{"tools": []any{
			map[string]any{"type": "mcp", "ref": "linear", "tools": []any{"nope"}},
		}},
	})
	_, err := ResolveRefs(m2, r)
	if err == nil || !strings.Contains(err.Error(), "available: only_tool") {
		t.Fatalf("err = %v", err)
	}
}
