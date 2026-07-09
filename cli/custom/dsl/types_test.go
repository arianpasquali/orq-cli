package dsl

import "testing"

func TestIdentity(t *testing.T) {
	cases := []struct {
		name string
		m    Manifest
		want string
	}{
		{"agent by key", Manifest{Kind: "Agent", Metadata: Metadata{Key: "eng-companion"}}, "Agent/eng-companion"},
		{"project by name", Manifest{Kind: "Project", Metadata: Metadata{Name: "thales-onboarding"}}, "Project/thales-onboarding"},
		{"prompt by path+display", Manifest{Kind: "Prompt", Metadata: Metadata{DisplayName: "classify-ticket", Path: "acme/prompts"}}, "Prompt/acme/prompts|classify-ticket"},
		{"dataset by path+display", Manifest{Kind: "Dataset", Metadata: Metadata{DisplayName: "golden", Path: "acme"}}, "Dataset/acme|golden"},
		{"memory store by key", Manifest{Kind: "MemoryStore", Metadata: Metadata{Key: "user_context"}}, "MemoryStore/user_context"},
	}
	for _, c := range cases {
		if got := c.m.Identity(); got != c.want {
			t.Errorf("%s: Identity() = %q, want %q", c.name, got, c.want)
		}
	}
}

func TestIdentityValue(t *testing.T) {
	if got := (Manifest{Kind: "Skill", Metadata: Metadata{DisplayName: "playbook", Path: "p"}}).IdentityValue(); got != "playbook" {
		t.Errorf("IdentityValue skill = %q", got)
	}
	if got := (Manifest{Kind: "Evaluator", Metadata: Metadata{Key: "faith"}}).IdentityValue(); got != "faith" {
		t.Errorf("IdentityValue evaluator = %q", got)
	}
}

func TestValidationErrorString(t *testing.T) {
	e := ValidationError{File: "agents/a.yaml", Line: 12, Msg: "boom"}
	if e.Error() != "agents/a.yaml:12  boom" {
		t.Errorf("got %q", e.Error())
	}
	if (ValidationError{Msg: "x"}).Error() != "x" {
		t.Error("fileless error should be bare msg")
	}
}
