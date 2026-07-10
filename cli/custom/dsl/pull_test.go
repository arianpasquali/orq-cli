package dsl

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPullRoundTrip(t *testing.T) {
	t.Setenv("LINEAR_TOKEN_FOR_TEST", "sekret")

	// Apply the fixture stack + an mcp tool with a secret header.
	sim := NewSimulator()
	dir, cfg := planStack(t)
	writeFile(t, dir, "tool.yaml", `apiVersion: orq.ai/v1
kind: Tool
metadata: { key: linear-mcp }
spec:
  type: mcp
  description: Linear MCP
  status: live
  mcp:
    server_url: https://mcp.linear.app/mcp
    connection_type: http
    headers:
      Authorization:
        value: "Bearer ${env.LINEAR_TOKEN_FOR_TEST}"
        encrypted: true
`)
	ms, _, errs := Validate(dir, "", nil)
	if len(errs) != 0 {
		t.Fatal(errs)
	}
	c := simClient(t, sim)
	plan, err := BuildPlan(ms, cfg, c, nil, "")
	if err != nil {
		t.Fatal(err)
	}
	var sink strings.Builder
	if err := Execute(c, plan, &sink, fixedNow()); err != nil {
		t.Fatalf("%v\n%s", err, sink.String())
	}

	// Pull into a fresh dir.
	outDir := t.TempDir()
	st, _, err := LoadState(c, "test-stack")
	if err != nil {
		t.Fatal(err)
	}
	report, err := Pull(c, "demo-project", outDir, st, "demo-project")
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Written) != 4 { // agent, evaluator, kb, tool — no project, no state skill
		t.Fatalf("written: %v", report.Written)
	}

	// Secret redacted, not present in the file.
	toolFile, err := os.ReadFile(filepath.Join(outDir, "tools", "linear-mcp.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(toolFile), "sekret") {
		t.Fatal("secret leaked into pulled file")
	}
	if !strings.Contains(string(toolFile), "${env.LINEAR_MCP_AUTHORIZATION}") {
		t.Fatalf("redaction placeholder missing:\n%s", string(toolFile))
	}
	warned := false
	for _, w := range report.Warnings {
		warned = warned || strings.Contains(w, "LINEAR_MCP_AUTHORIZATION")
	}
	if !warned {
		t.Errorf("no redaction warning: %v", report.Warnings)
	}

	// Round-trip: pulled dir + orq.yaml must plan to zero changes.
	writeFile(t, outDir, "orq.yaml", "stack: test-stack\ndefaults:\n  path: demo-project\n")
	// Project manifest is not pulled; add it so the identity set matches state.
	writeFile(t, outDir, "projects/demo-project.yaml", `apiVersion: orq.ai/v1
kind: Project
metadata: { name: demo-project }
spec: { description: Demo }
`)
	t.Setenv("LINEAR_MCP_AUTHORIZATION", "Bearer sekret") // redaction round-trip
	ms2, cfg2, errs2 := Validate(outDir, "", nil)
	if len(errs2) != 0 {
		t.Fatalf("pulled stack invalid: %v", errs2)
	}
	st2, stateID2, err := LoadState(c, "test-stack")
	if err != nil {
		t.Fatal(err)
	}
	plan2, err := BuildPlan(ms2, cfg2, c, st2, stateID2)
	if err != nil {
		t.Fatal(err)
	}
	if plan2.HasChanges() {
		var buf strings.Builder
		RenderPlan(&buf, plan2, false)
		t.Fatalf("round-trip not clean:\n%s", buf.String())
	}
}

func TestInitScaffoldAndRefuse(t *testing.T) {
	dir := t.TempDir()
	files, err := Init(dir, "my-stack", "shared-project")
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 3 {
		t.Fatalf("files: %v", files)
	}
	cfg, err := LoadStack(dir)
	if err != nil || cfg.Stack != "my-stack" {
		t.Fatalf("cfg %+v err %v", cfg, err)
	}
	if cfg.Defaults.Path != "shared-project" {
		t.Fatalf("defaults.path %q, want the given project", cfg.Defaults.Path)
	}
	ms, errs := LoadManifests(dir, cfg)
	if len(errs) != 0 || len(ms) != 1 {
		t.Fatalf("scaffold manifests: %d errs %v", len(ms), errs)
	}
	if _, err := Init(dir, "my-stack", ""); err == nil {
		t.Fatal("second init must refuse")
	}
	if _, err := Init(t.TempDir(), "Bad Name", ""); err == nil {
		t.Fatal("bad stack name must refuse")
	}
}
