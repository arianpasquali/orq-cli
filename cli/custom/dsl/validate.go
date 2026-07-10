package dsl

import (
	"fmt"
	"io"
	"strings"
)

// RenderValidationErrors prints every error at once — red ✗, aligned file:line
// column, then a bold summary line. Mirrors RenderPlan's palette handling.
func RenderValidationErrors(w io.Writer, errs []ValidationError, color bool) {
	pal := colors(color)
	width := 0
	locs := make([]string, len(errs))
	for i, e := range errs {
		if e.File != "" {
			locs[i] = e.File
			if e.Line > 0 {
				locs[i] += ":" + itoa(e.Line)
			}
		}
		if len(locs[i]) > width {
			width = len(locs[i])
		}
	}
	for i, e := range errs {
		fmt.Fprintf(w, "%s✗%s %s%-*s%s  %s\n", pal.del, pal.reset, pal.head, width, locs[i], pal.reset, e.Msg)
	}
	noun := "errors"
	if len(errs) == 1 {
		noun = "error"
	}
	fmt.Fprintf(w, "\n%s%d %s, 0 warnings.%s\n", pal.head, len(errs), noun, pal.reset)
}

// RenderValidateOK prints the one-line success summary (green ✓).
func RenderValidateOK(w io.Writer, manifests, kinds int, color bool) {
	pal := colors(color)
	fmt.Fprintf(w, "%s✓%s %d manifests · %d kinds · schema ok · refs ok · vars ok\n",
		pal.add, pal.reset, manifests, kinds)
}

// Validate runs the full offline pipeline: load stack + manifests, resolve and
// interpolate variables, then check every manifest against the kind registry.
// It needs no credentials and never touches the network.
func Validate(dir, varFile string, cliVars []string) ([]Manifest, StackConfig, []ValidationError) {
	cfg, err := LoadStack(dir)
	if err != nil {
		return nil, cfg, []ValidationError{{File: dir, Msg: err.Error()}}
	}
	ms, errs := LoadManifests(dir, cfg)

	vars, verr := ResolveVars(cfg, varFile, cliVars)
	if verr != nil {
		errs = append(errs, ValidationError{Msg: verr.Error()})
		return ms, cfg, errs
	}
	errs = append(errs, Interpolate(ms, vars)...)

	seen := map[string]string{} // identity -> file:line
	for i := range ms {
		m := &ms[i]
		errs = append(errs, validateManifest(m)...)
		id := m.Identity()
		if prev, dup := seen[id]; dup {
			errs = append(errs, ValidationError{File: m.File, Line: m.Line,
				Msg: fmt.Sprintf("duplicate %s — also defined in %s", id, prev)})
		} else {
			seen[id] = fmt.Sprintf("%s:%d", m.File, m.Line)
		}
	}
	return ms, cfg, errs
}

func validateManifest(m *Manifest) []ValidationError {
	var errs []ValidationError
	bad := func(format string, a ...any) {
		errs = append(errs, ValidationError{File: m.File, Line: m.Line, Msg: fmt.Sprintf(format, a...)})
	}

	info, err := Lookup(m.Kind)
	if err != nil {
		bad("%v", err)
		return errs
	}
	if info.Gated != "" {
		bad("kind %s is not provisionable: %s", m.Kind, info.Gated)
		return errs
	}

	// Identity field must match the kind's identity mode.
	switch info.IdentityMode {
	case "name":
		if m.Metadata.Name == "" {
			bad("%s requires metadata.name", m.Kind)
		}
		if m.Metadata.Key != "" || m.Metadata.DisplayName != "" {
			bad("%s identity is metadata.name — remove key/display_name", m.Kind)
		}
	case "key":
		if m.Metadata.Key == "" {
			bad("%s requires metadata.key", m.Kind)
		}
	case "display_name":
		if m.Metadata.DisplayName == "" {
			bad("%s requires metadata.display_name", m.Kind)
		}
	}

	// Platform identity charsets.
	switch m.Kind {
	case "Agent":
		if m.Metadata.Key != "" && !agentKeyRe.MatchString(m.Metadata.Key) {
			bad("agent key %q: must match %s", m.Metadata.Key, agentKeyRe)
		}
	case "MemoryStore":
		if m.Metadata.Key != "" && !memoryStoreKeyRe.MatchString(m.Metadata.Key) {
			bad("memory store key %q: letters/digits/dots/underscores only — dashes are not allowed", m.Metadata.Key)
		}
	case "Skill":
		if m.Metadata.DisplayName != "" && !skillNameRe.MatchString(m.Metadata.DisplayName) {
			bad("skill name %q: letters, digits and underscores only", m.Metadata.DisplayName)
		}
		if strings.HasPrefix(m.Metadata.DisplayName, stateSkillPrefix) {
			bad("skill names starting with %q are reserved for stack state", stateSkillPrefix)
		}
	}

	// Required spec fields.
	for _, req := range info.Required {
		if !hasPath(m.Spec, req) {
			bad("spec.%s is required for %s", req, m.Kind)
		}
	}

	errs = append(errs, validateKindSpecific(m)...)
	errs = append(errs, validateRefSyntax(m)...)
	return errs
}

func validateKindSpecific(m *Manifest) []ValidationError {
	var errs []ValidationError
	bad := func(format string, a ...any) {
		errs = append(errs, ValidationError{File: m.File, Line: m.Line, Msg: fmt.Sprintf(format, a...)})
	}
	switch m.Kind {
	case "Evaluator":
		typ, _ := m.Spec["type"].(string)
		switch typ {
		case "llm_eval":
			mode, _ := m.Spec["mode"].(string)
			if mode == "" {
				mode = "single"
			}
			if _, ok := m.Spec["prompt"]; !ok {
				bad("llm_eval requires spec.prompt (the judge prompt)")
			}
			switch mode {
			case "single":
				if _, ok := m.Spec["model"]; !ok {
					bad("llm_eval with mode single requires spec.model")
				}
			case "jury":
				if _, ok := m.Spec["jury"]; !ok {
					bad("llm_eval with mode jury requires spec.jury (judges list)")
				}
			default:
				bad("evaluator mode %q: must be single or jury", mode)
			}
		case "python_eval":
			if _, ok := m.Spec["code"]; !ok {
				bad("python_eval requires spec.code")
			}
		case "":
			// missing `type` already reported via Required
		default:
			bad("evaluator type %q: the public API creates llm_eval and python_eval only", typ)
		}
	case "Tool":
		typ, _ := m.Spec["type"].(string)
		payload := map[string]string{
			"function": "function", "json_schema": "json_schema", "http": "http",
			"mcp": "mcp", "code": "code_tool",
		}
		if typ != "" {
			key, ok := payload[typ]
			if !ok {
				bad("tool type %q: must be one of function, json_schema, http, mcp, code", typ)
			} else if _, has := m.Spec[key]; !has {
				bad("tool type %s requires spec.%s", typ, key)
			}
		}
	case "KnowledgeBase":
		typ, _ := m.Spec["type"].(string)
		if typ == "" || typ == "internal" {
			if _, ok := m.Spec["embedding_model"]; !ok {
				bad("internal knowledge base requires spec.embedding_model")
			}
		} else if typ == "external" {
			if _, ok := m.Spec["external_config"]; !ok {
				bad("external knowledge base requires spec.external_config {name, api_url, api_key}")
			}
		} else {
			bad("knowledge base type %q: internal or external", typ)
		}
	}
	return errs
}

// validateRefSyntax walks the spec for {ref: ...} nodes and rejects malformed
// ones. Whether the target exists is a plan-time question.
func validateRefSyntax(m *Manifest) []ValidationError {
	var errs []ValidationError
	var walk func(node any)
	walk = func(node any) {
		switch v := node.(type) {
		case map[string]any:
			if raw, has := v["ref"]; has {
				if s, ok := raw.(string); !ok || s == "" {
					errs = append(errs, ValidationError{File: m.File, Line: m.Line,
						Msg: "ref: value must be a non-empty string (the target's key)"})
				}
			}
			for _, child := range v {
				walk(child)
			}
		case []any:
			for _, child := range v {
				walk(child)
			}
		}
	}
	walk(m.Spec)
	return errs
}

// hasPath checks a dotted path exists in a nested map.
func hasPath(spec map[string]any, dotted string) bool {
	cur := any(spec)
	for _, seg := range strings.Split(dotted, ".") {
		mp, ok := cur.(map[string]any)
		if !ok {
			return false
		}
		cur, ok = mp[seg]
		if !ok {
			return false
		}
	}
	return true
}
