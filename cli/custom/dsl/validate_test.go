package dsl

import (
	"strings"
	"testing"
)

func TestRegistryIntegrity(t *testing.T) {
	for kind, info := range Registry() {
		if info.Gated != "" {
			continue
		}
		if info.Plural == "" || info.BasePath == "" || info.IdentityMode == "" {
			t.Errorf("%s: incomplete registry entry: %+v", kind, info)
		}
		if info.IDField == "" && !info.GetByIdentity {
			t.Errorf("%s: needs IDField or GetByIdentity", kind)
		}
	}
	if _, err := Lookup("Nope"); err == nil || !strings.Contains(err.Error(), "unknown kind") {
		t.Error("unknown kind should error")
	}
	if info, _ := Lookup("Deployment"); info.Gated == "" {
		t.Error("Deployment must be gated")
	}
}

func validStack(t *testing.T) string {
	dir, _ := stackDir(t)
	writeFile(t, dir, "kb.yaml", `apiVersion: orq.ai/v1
kind: KnowledgeBase
metadata: { key: eng-kb }
spec:
  embedding_model: mistral/mistral-embed
`)
	writeFile(t, dir, "agent.yaml", `apiVersion: orq.ai/v1
kind: Agent
metadata: { key: companion }
spec:
  role: Assistant
  description: d
  instructions: i
  model: ${var.model}
  settings:
    max_iterations: 5
  knowledge_bases:
    - ref: eng-kb
`)
	writeFile(t, dir, "eval.yaml", `apiVersion: orq.ai/v1
kind: Evaluator
metadata: { key: faith }
spec:
  type: llm_eval
  mode: single
  model: mistral/mistral-large-latest
  prompt: judge it
`)
	return dir
}

func TestValidateOK(t *testing.T) {
	dir := validStack(t)
	ms, cfg, errs := Validate(dir, "", nil)
	if len(errs) != 0 {
		t.Fatalf("unexpected: %v", errs)
	}
	if len(ms) != 3 || cfg.Stack != "test-stack" {
		t.Fatalf("ms=%d cfg=%+v", len(ms), cfg)
	}
}

func TestValidateBadStack(t *testing.T) {
	dir, _ := stackDir(t)
	writeFile(t, dir, "bad.yaml", `apiVersion: orq.ai/v1
kind: Agent
metadata: { key: dup }
spec: { role: r, description: d, instructions: i, model: m, settings: {} }
---
apiVersion: orq.ai/v1
kind: Agent
metadata: { key: dup }
spec: { role: r, description: d, instructions: i, model: m, settings: {} }
---
apiVersion: orq.ai/v1
kind: Evaluator
metadata: { key: broken }
spec: { type: llm_eval, mode: single, prompt: p }
---
apiVersion: orq.ai/v1
kind: MemoryStore
metadata: { key: has-dashes }
spec: { description: d, embedding_config: { model: m } }
---
apiVersion: orq.ai/v1
kind: Deployment
metadata: { key: nope }
spec: {}
---
apiVersion: orq.ai/v1
kind: Prompt
metadata: { display_name: p1 }
spec:
  prompt:
    messages:
      - role: system
        content: ${var.missing_one}
`)
	_, _, errs := Validate(dir, "", nil)
	if len(errs) != 5 {
		t.Fatalf("want 5 errors, got %d:\n%v", len(errs), errs)
	}
	wants := []string{"duplicate Agent/dup", "requires spec.model", "dashes are not allowed", "not provisionable", "${var.missing_one}"}
	for _, w := range wants {
		found := false
		for _, e := range errs {
			if strings.Contains(e.Msg, w) {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("no error containing %q in %v", w, errs)
		}
	}
}

func TestValidateToolPayloadAndReservedSkill(t *testing.T) {
	dir, _ := stackDir(t)
	writeFile(t, dir, "t.yaml", `apiVersion: orq.ai/v1
kind: Tool
metadata: { key: broken-tool }
spec: { type: mcp, description: d }
---
apiVersion: orq.ai/v1
kind: Skill
metadata: { display_name: orq_dsl_state_evil }
spec: {}
---
apiVersion: orq.ai/v1
kind: Tool
metadata: { key: bad-ref }
spec:
  type: http
  description: d
  http: { blueprint: { url: u, method: GET } }
  extra: [ { ref: "" } ]
`)
	_, _, errs := Validate(dir, "", nil)
	if len(errs) != 3 {
		t.Fatalf("want 3, got %d: %v", len(errs), errs)
	}
	for _, w := range []string{"requires spec.mcp", "reserved for stack state", "non-empty string"} {
		found := false
		for _, e := range errs {
			found = found || strings.Contains(e.Msg, w)
		}
		if !found {
			t.Errorf("missing error %q: %v", w, errs)
		}
	}
}
