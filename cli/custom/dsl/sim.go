package dsl

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"sync"
)

// Simulator is an in-memory stand-in for the public v2 API, faithful to the
// identity semantics the engine relies on (key-addressed agents and memory
// stores, name-or-id skills, wrapped project/skill GETs, list envelopes).
// It backs unit tests and the dry-smoke server; it is NOT part of the CLI.
type Simulator struct {
	mu     sync.Mutex
	seq    int
	stores map[string][]map[string]any // BasePath → objects
}

func NewSimulator() *Simulator {
	return &Simulator{stores: map[string][]map[string]any{}}
}

// simKindByBase maps URL base paths back to registry entries.
func simKindByBase() map[string]KindInfo {
	out := map[string]KindInfo{}
	for _, info := range Registry() {
		if info.Gated == "" {
			out[info.BasePath] = info
		}
	}
	return out
}

func (s *Simulator) nextID(prefix string) string {
	s.seq++
	return fmt.Sprintf("%s_%04d", prefix, s.seq)
}

// Seed inserts an object directly (tests).
func (s *Simulator) Seed(basePath string, obj map[string]any) map[string]any {
	s.mu.Lock()
	defer s.mu.Unlock()
	info := simKindByBase()[basePath]
	idField := info.IDField
	if idField == "" {
		idField = "_id"
	}
	if _, ok := obj[idField]; !ok {
		obj[idField] = s.nextID(strings.ToLower(info.Kind))
	}
	s.stores[basePath] = append(s.stores[basePath], obj)
	return obj
}

// Objects returns a snapshot of one kind's store (tests).
func (s *Simulator) Objects(basePath string) []map[string]any {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]map[string]any(nil), s.stores[basePath]...)
}

func (s *Simulator) Handler() http.Handler {
	kinds := simKindByBase()
	mux := http.NewServeMux()
	for base, info := range kinds {
		base, info := base, info
		mux.HandleFunc("GET "+base, func(w http.ResponseWriter, r *http.Request) { s.list(w, r, base) })
		mux.HandleFunc("POST "+base, func(w http.ResponseWriter, r *http.Request) { s.create(w, r, base, info) })
		mux.HandleFunc("GET "+base+"/{id}", func(w http.ResponseWriter, r *http.Request) { s.getOne(w, r, base, info) })
		mux.HandleFunc("PATCH "+base+"/{id}", func(w http.ResponseWriter, r *http.Request) { s.patch(w, r, base, info) })
		mux.HandleFunc("DELETE "+base+"/{id}", func(w http.ResponseWriter, r *http.Request) { s.delete(w, r, base, info) })
	}
	return mux
}

func (s *Simulator) list(w http.ResponseWriter, r *http.Request, base string) {
	s.mu.Lock()
	items := append([]map[string]any(nil), s.stores[base]...)
	s.mu.Unlock()
	// The engine always paginates with limit=100; a simulator workspace stays
	// far below that, so one page with has_more=false suffices.
	json.NewEncoder(w).Encode(map[string]any{"object": "list", "data": items, "has_more": false})
}

func (s *Simulator) create(w http.ResponseWriter, r *http.Request, base string, info KindInfo) {
	var body map[string]any
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, `{"message":"invalid json"}`, 400)
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	// Uniqueness on the identity field, mirroring platform behavior.
	idValField := map[string]string{"name": "name", "key": "key", "display_name": "display_name"}[info.IdentityMode]
	idVal, _ := body[idValField].(string)
	for _, existing := range s.stores[base] {
		if ev, _ := existing[idValField].(string); ev != "" && strings.EqualFold(ev, idVal) && info.Kind != "Dataset" {
			http.Error(w, `{"message":"duplicate `+idValField+`"}`, 409)
			return
		}
	}

	idField := info.IDField
	if idField == "" {
		idField = "_id"
	}
	body[idField] = s.nextID(strings.ToLower(info.Kind))
	// MCP tools get discovered sub-tools on create, like the platform's sync.
	if info.Kind == "Tool" {
		if body["type"] == "mcp" {
			if mcp, ok := body["mcp"].(map[string]any); ok {
				mcp["tools"] = []any{
					map[string]any{"id": s.nextID("disc"), "name": "list_issues", "description": "List issues"},
					map[string]any{"id": s.nextID("disc"), "name": "get_issue", "description": "Get one issue"},
				}
			}
		}
	}
	s.stores[base] = append(s.stores[base], body)
	json.NewEncoder(w).Encode(wrapSim(body, info))
}

func (s *Simulator) find(base string, info KindInfo, idOrIdentity string) (int, map[string]any) {
	idField := info.IDField
	if idField == "" {
		idField = "_id"
	}
	for i, obj := range s.stores[base] {
		if v, _ := obj[idField].(string); v == idOrIdentity {
			return i, obj
		}
		if info.GetByIdentity {
			field := map[string]string{"name": "name", "key": "key", "display_name": "display_name"}[info.IdentityMode]
			if v, _ := obj[field].(string); v == idOrIdentity {
				return i, obj
			}
		}
	}
	return -1, nil
}

func (s *Simulator) getOne(w http.ResponseWriter, r *http.Request, base string, info KindInfo) {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, obj := s.find(base, info, r.PathValue("id"))
	if obj == nil {
		http.Error(w, `{"message":"not found"}`, 404)
		return
	}
	json.NewEncoder(w).Encode(wrapSim(obj, info))
}

func (s *Simulator) patch(w http.ResponseWriter, r *http.Request, base string, info KindInfo) {
	var body map[string]any
	json.NewDecoder(r.Body).Decode(&body)
	s.mu.Lock()
	defer s.mu.Unlock()
	i, obj := s.find(base, info, r.PathValue("id"))
	if obj == nil {
		http.Error(w, `{"message":"not found"}`, 404)
		return
	}
	// Shallow merge — matches the platform's $set/spread semantics closely
	// enough for the engine (which always sends whole top-level fields).
	for k, v := range body {
		if k == "versionIncrement" || k == "versionDescription" {
			continue
		}
		obj[k] = v
	}
	s.stores[base][i] = obj
	json.NewEncoder(w).Encode(wrapSim(obj, info))
}

func (s *Simulator) delete(w http.ResponseWriter, r *http.Request, base string, info KindInfo) {
	s.mu.Lock()
	defer s.mu.Unlock()
	i, obj := s.find(base, info, r.PathValue("id"))
	if obj == nil {
		http.Error(w, `{"message":"not found"}`, 404)
		return
	}
	s.stores[base] = append(s.stores[base][:i], s.stores[base][i+1:]...)
	w.WriteHeader(204)
}

func wrapSim(obj map[string]any, info KindInfo) map[string]any {
	if info.Wrap == "" {
		return obj
	}
	return map[string]any{info.Wrap: obj}
}

// DumpIdentities lists "Kind/identityValue" for everything stored (tests).
func (s *Simulator) DumpIdentities() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []string
	for base, objs := range s.stores {
		info := simKindByBase()[base]
		field := map[string]string{"name": "name", "key": "key", "display_name": "display_name"}[info.IdentityMode]
		for _, o := range objs {
			v, _ := o[field].(string)
			out = append(out, info.Kind+"/"+v)
		}
	}
	sort.Strings(out)
	return out
}
