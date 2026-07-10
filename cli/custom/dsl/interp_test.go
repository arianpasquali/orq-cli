package dsl

import (
	"strings"
	"testing"
)

func TestResolveVarsPrecedence(t *testing.T) {
	dir := t.TempDir()
	varFile := writeFile(t, dir, "prod.yaml", "a: from-file\nb: from-file\nc: from-file\n")
	cfg := StackConfig{Variables: map[string]string{"a": "from-cfg", "b": "from-cfg", "c": "from-cfg", "d": "from-cfg"}}
	t.Setenv("ORQ_VAR_a", "from-env")
	vars, err := ResolveVars(cfg, varFile, []string{"b=from-cli"})
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]string{"a": "from-env", "b": "from-cli", "c": "from-file", "d": "from-cfg"}
	for k, w := range want {
		if vars[k] != w {
			t.Errorf("vars[%s] = %q, want %q", k, vars[k], w)
		}
	}
}

func TestResolveVarsBadCLI(t *testing.T) {
	if _, err := ResolveVars(StackConfig{}, "", []string{"noequals"}); err == nil {
		t.Fatal("expected error")
	}
}

func interpOne(t *testing.T, spec map[string]any, vars map[string]string) (Manifest, []ValidationError) {
	t.Helper()
	ms := []Manifest{{Kind: "Agent", Metadata: Metadata{Key: "a"}, Spec: spec, File: "agents/a.yaml", Line: 1}}
	errs := Interpolate(ms, vars)
	return ms[0], errs
}

func TestInterpolateVarAndEnv(t *testing.T) {
	t.Setenv("SECRET_TOKEN", "s3cr3t")
	m, errs := interpOne(t, map[string]any{
		"model": "${var.model}",
		"settings": map[string]any{
			"tools": []any{map[string]any{"header": "Bearer ${env.SECRET_TOKEN}"}},
		},
		"count": 5,
	}, map[string]string{"model": "mistral/large"})
	if len(errs) != 0 {
		t.Fatalf("errs: %v", errs)
	}
	if m.Spec["model"] != "mistral/large" {
		t.Errorf("var not substituted: %v", m.Spec["model"])
	}
	tool := m.Spec["settings"].(map[string]any)["tools"].([]any)[0].(map[string]any)
	if tool["header"] != "Bearer s3cr3t" {
		t.Errorf("env not substituted: %v", tool["header"])
	}
	if m.Spec["count"] != 5 {
		t.Errorf("non-string touched: %v", m.Spec["count"])
	}
	if !m.HasSecrets {
		t.Error("HasSecrets not marked")
	}
}

func TestInterpolateMissingVar(t *testing.T) {
	_, errs := interpOne(t, map[string]any{"model": "${var.nope}"}, nil)
	if len(errs) != 1 || !strings.Contains(errs[0].Msg, "${var.nope}") || errs[0].File != "agents/a.yaml" {
		t.Fatalf("errs: %v", errs)
	}
}

func TestInterpolateMissingEnv(t *testing.T) {
	_, errs := interpOne(t, map[string]any{"h": "${env.DEFINITELY_UNSET_VAR_42}"}, nil)
	if len(errs) != 1 {
		t.Fatalf("errs: %v", errs)
	}
}

func TestFileInclude(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "instructions.md", "You are helpful.\n")
	ms := []Manifest{{
		Kind:     "Agent",
		Metadata: Metadata{Key: "a"},
		Spec:     map[string]any{"instructions": map[string]any{"$file": "instructions.md"}},
		File:     "a.yaml", Line: 1, absDir: dir,
	}}
	if errs := Interpolate(ms, nil); len(errs) != 0 {
		t.Fatalf("errs: %v", errs)
	}
	if ms[0].Spec["instructions"] != "You are helpful.\n" {
		t.Errorf("include failed: %q", ms[0].Spec["instructions"])
	}
}

func TestFileIncludeErrors(t *testing.T) {
	// missing file
	ms := []Manifest{{Kind: "Agent", Spec: map[string]any{"x": map[string]any{"$file": "gone.md"}}, File: "a.yaml", absDir: t.TempDir()}}
	if errs := Interpolate(ms, nil); len(errs) != 1 {
		t.Fatalf("missing file: %v", errs)
	}
	// $file beside other keys
	ms = []Manifest{{Kind: "Agent", Spec: map[string]any{"x": map[string]any{"$file": "f.md", "extra": 1}}, File: "a.yaml"}}
	if errs := Interpolate(ms, nil); len(errs) != 1 || !strings.Contains(errs[0].Msg, "only key") {
		t.Fatalf("extra keys: %v", errs)
	}
}

func TestBuiltinVarsAndIdentityInterpolation(t *testing.T) {
	cfg := StackConfig{Stack: "acme-platform"}
	vars, err := ResolveVars(cfg, "", nil)
	if err != nil {
		t.Fatal(err)
	}
	if vars["stack"] != "acme-platform" {
		t.Fatalf("stack builtin: %q", vars["stack"])
	}
	u := vars["unique"]
	if len(u) != 8 || u != uniqueString("acme-platform") {
		t.Fatalf("unique builtin not deterministic: %q", u)
	}
	if uniqueString("other") == u {
		t.Fatal("unique must differ per seed")
	}
	// builtins cannot be shadowed
	cfg.Variables = map[string]string{"stack": "evil"}
	vars, _ = ResolveVars(cfg, "", nil)
	if vars["stack"] != "acme-platform" {
		t.Fatalf("builtin shadowed: %q", vars["stack"])
	}

	ms := []Manifest{{Kind: "Agent", Metadata: Metadata{Key: "${var.stack}-agent-${var.unique}", Path: "${var.stack}"}, Spec: map[string]any{}}}
	if errs := Interpolate(ms, vars); len(errs) != 0 {
		t.Fatalf("interp errs: %v", errs)
	}
	want := "acme-platform-agent-" + u
	if ms[0].Metadata.Key != want {
		t.Fatalf("key %q, want %q", ms[0].Metadata.Key, want)
	}
	if ms[0].Metadata.Path != "acme-platform" {
		t.Fatalf("path %q", ms[0].Metadata.Path)
	}
}
