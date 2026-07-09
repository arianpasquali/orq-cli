package dsl

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// skillStateServer simulates the /v2/skills surface the state store uses.
type skillStateServer struct {
	instructions string
	skillID      string
	patches      int
}

func (s *skillStateServer) handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /v2/skills/{name}", func(w http.ResponseWriter, r *http.Request) {
		if s.skillID == "" {
			w.WriteHeader(404)
			fmt.Fprint(w, `{"message":"skill not found"}`)
			return
		}
		json.NewEncoder(w).Encode(map[string]any{"skill": map[string]any{
			"skill_id": s.skillID, "display_name": r.PathValue("name"), "instructions": s.instructions,
		}})
	})
	mux.HandleFunc("POST /v2/skills", func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		json.NewDecoder(r.Body).Decode(&body)
		s.skillID = "sk_123"
		s.instructions, _ = body["instructions"].(string)
		json.NewEncoder(w).Encode(map[string]any{"skill": map[string]any{"skill_id": s.skillID}})
	})
	mux.HandleFunc("PATCH /v2/skills/{id}", func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		json.NewDecoder(r.Body).Decode(&body)
		s.instructions, _ = body["instructions"].(string)
		s.patches++
		fmt.Fprint(w, `{}`)
	})
	return mux
}

func stateClient(t *testing.T, s *skillStateServer) *Client {
	t.Helper()
	srv := httptest.NewServer(s.handler())
	t.Cleanup(srv.Close)
	t.Setenv("ORQ_API_KEY", "k")
	c := newClientWithBase(srv.URL)
	c.sleep = func(time.Duration) {}
	return c
}

func TestStateLifecycle(t *testing.T) {
	srv := &skillStateServer{}
	c := stateClient(t, srv)

	// absent → nil
	doc, id, err := LoadState(c, "my-stack")
	if err != nil || doc != nil || id != "" {
		t.Fatalf("absent state: doc=%v id=%q err=%v", doc, id, err)
	}

	// create
	doc = &StateDoc{Version: 1, Stack: "my-stack"}
	doc.Upsert(StateResource{Kind: "Agent", Identity: "Agent/a", ServerID: "agnt_1", Path: "p"})
	id, err = SaveState(c, doc, "", "default-project")
	if err != nil || id != "sk_123" || doc.Revision != 1 {
		t.Fatalf("create: id=%q rev=%d err=%v", id, doc.Revision, err)
	}

	// round-trip
	loaded, loadedID, err := LoadState(c, "my-stack")
	if err != nil || loadedID != "sk_123" {
		t.Fatalf("load: %v %q", err, loadedID)
	}
	if loaded.Revision != 1 || len(loaded.Resources) != 1 || loaded.Resources[0].ServerID != "agnt_1" {
		t.Fatalf("round-trip: %+v", loaded)
	}

	// update
	loaded.Upsert(StateResource{Kind: "Tool", Identity: "Tool/t", ServerID: "tool_1"})
	if _, err = SaveState(c, loaded, loadedID, ""); err != nil {
		t.Fatal(err)
	}
	if srv.patches != 1 {
		t.Fatalf("patches = %d", srv.patches)
	}

	// helpers
	if loaded.Find("Tool", "Tool/t") == nil || loaded.Find("Tool", "Tool/x") != nil {
		t.Error("Find misbehaves")
	}
	loaded.Remove("Tool", "Tool/t")
	if loaded.Find("Tool", "Tool/t") != nil {
		t.Error("Remove failed")
	}
	rev := loaded.IDToIdentity()
	if rev["agnt_1"] != "Agent/a" {
		t.Errorf("IDToIdentity: %v", rev)
	}
}

func TestStateConflict(t *testing.T) {
	srv := &skillStateServer{}
	c := stateClient(t, srv)

	doc := &StateDoc{Version: 1, Stack: "s-a"}
	id, err := SaveState(c, doc, "", "p")
	if err != nil {
		t.Fatal(err)
	}

	// Writer B loads rev 1 and saves (rev 2).
	b, _, _ := LoadState(c, "s-a")
	if _, err := SaveState(c, b, id, ""); err != nil {
		t.Fatal(err)
	}

	// Writer A still holds rev 1 and tries to save → conflict, revision restored.
	a := &StateDoc{Version: 1, Stack: "s-a", Revision: 1}
	_, err = SaveState(c, a, id, "")
	conflict, ok := err.(*ErrStateConflict)
	if !ok || conflict.Live != 2 || conflict.Parent != 1 || a.Revision != 1 {
		t.Fatalf("err = %v, rev=%d", err, a.Revision)
	}
}

func TestStateSkillName(t *testing.T) {
	if stateSkillName("thales-onboarding") != "orq_dsl_state_thales_onboarding" {
		t.Errorf("got %q", stateSkillName("thales-onboarding"))
	}
}

func TestLoadStateCorrupt(t *testing.T) {
	srv := &skillStateServer{skillID: "sk_9", instructions: "not json"}
	c := stateClient(t, srv)
	_, id, err := LoadState(c, "x")
	if err == nil || id != "sk_9" {
		t.Fatalf("corrupt state: id=%q err=%v", id, err)
	}
}
