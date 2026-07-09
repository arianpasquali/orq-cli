package dsl

import (
	"os"
	"path/filepath"
	"testing"
)

func writeFile(t *testing.T, dir, name, content string) string {
	t.Helper()
	p := filepath.Join(dir, name)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func stackDir(t *testing.T) (string, StackConfig) {
	t.Helper()
	dir := t.TempDir()
	writeFile(t, dir, "orq.yaml", "stack: test-stack\ndefaults:\n  path: demo-project\nvariables:\n  model: mistral/mistral-large-latest\n")
	cfg, err := LoadStack(dir)
	if err != nil {
		t.Fatal(err)
	}
	return dir, cfg
}

func TestLoadStack(t *testing.T) {
	dir, cfg := stackDir(t)
	if cfg.Stack != "test-stack" || cfg.Defaults.Path != "demo-project" || cfg.Variables["model"] == "" || cfg.Dir != dir {
		t.Fatalf("unexpected cfg: %+v", cfg)
	}
}

func TestLoadStackMissing(t *testing.T) {
	if _, err := LoadStack(t.TempDir()); err == nil {
		t.Fatal("expected error for missing orq.yaml")
	}
}

func TestLoadStackBadName(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "orq.yaml", "stack: Bad_Name\n")
	if _, err := LoadStack(dir); err == nil {
		t.Fatal("expected error for invalid stack name")
	}
}

func TestLoadManifestsSingleAndMulti(t *testing.T) {
	dir, cfg := stackDir(t)
	writeFile(t, dir, "agents/a.yaml", `apiVersion: orq.ai/v1
kind: Agent
metadata:
  key: a1
spec:
  role: Assistant
  settings:
    max_iterations: 5
`)
	writeFile(t, dir, "evals/multi.yaml", `apiVersion: orq.ai/v1
kind: Evaluator
metadata: { key: e1, path: p1 }
spec: { type: python_eval, code: "def evaluate(): pass" }
---
apiVersion: orq.ai/v1
kind: Evaluator
metadata: { key: e2 }
spec: { type: llm_eval, mode: single, model: m, prompt: hi }
---
apiVersion: orq.ai/v1
kind: Evaluator
metadata: { key: e3 }
spec: { type: python_eval, code: x }
`)
	ms, errs := LoadManifests(dir, cfg)
	if len(errs) != 0 {
		t.Fatalf("unexpected errors: %v", errs)
	}
	if len(ms) != 4 {
		t.Fatalf("want 4 manifests, got %d", len(ms))
	}
	byID := map[string]Manifest{}
	for _, m := range ms {
		byID[m.Identity()] = m
	}
	// default path injected for a1 and e2/e3; explicit p1 kept for e1
	if byID["Agent/a1"].Metadata.Path != "demo-project" {
		t.Errorf("default path not injected: %+v", byID["Agent/a1"].Metadata)
	}
	if byID["Evaluator/e1"].Metadata.Path != "p1" {
		t.Errorf("explicit path clobbered: %+v", byID["Evaluator/e1"].Metadata)
	}
	// multi-doc line tracking: e2 starts after e1's doc
	if byID["Evaluator/e2"].Line <= byID["Evaluator/e1"].Line {
		t.Errorf("doc lines not increasing: e1=%d e2=%d", byID["Evaluator/e1"].Line, byID["Evaluator/e2"].Line)
	}
	// nested spec preserved
	settings, ok := byID["Agent/a1"].Spec["settings"].(map[string]any)
	if !ok || settings["max_iterations"] != 5 {
		t.Errorf("nested spec lost: %#v", byID["Agent/a1"].Spec)
	}
	// file is relative to the stack dir
	if byID["Agent/a1"].File != filepath.Join("agents", "a.yaml") {
		t.Errorf("file not relative: %q", byID["Agent/a1"].File)
	}
}

func TestLoadManifestsEnvelopeErrors(t *testing.T) {
	dir, cfg := stackDir(t)
	writeFile(t, dir, "bad.yaml", `apiVersion: wrong/v9
metadata: { key: x }
spec: {}
`)
	ms, errs := LoadManifests(dir, cfg)
	if len(ms) != 0 {
		t.Fatalf("bad manifest accepted: %+v", ms)
	}
	if len(errs) != 2 { // wrong apiVersion + missing kind
		t.Fatalf("want 2 errors, got %d: %v", len(errs), errs)
	}
	for _, e := range errs {
		if e.File != "bad.yaml" || e.Line == 0 {
			t.Errorf("error not anchored: %+v", e)
		}
	}
}

func TestLoadManifestsSkipsVarsDir(t *testing.T) {
	dir, cfg := stackDir(t)
	writeFile(t, dir, "vars/prod.yaml", "model: something\n")
	ms, errs := LoadManifests(dir, cfg)
	if len(ms) != 0 || len(errs) != 0 {
		t.Fatalf("vars dir not skipped: %d manifests %v", len(ms), errs)
	}
}
