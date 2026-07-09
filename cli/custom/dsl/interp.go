package dsl

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"
)

var interpRe = regexp.MustCompile(`\$\{(var|env)\.([A-Za-z_][A-Za-z0-9_]*)\}`)

// ResolveVars merges variable sources, later wins:
// orq.yaml variables < --var-file < --var k=v < ORQ_VAR_k env.
func ResolveVars(cfg StackConfig, varFile string, cliVars []string) (map[string]string, error) {
	vars := map[string]string{}
	for k, v := range cfg.Variables {
		vars[k] = v
	}
	if varFile != "" {
		data, err := os.ReadFile(varFile)
		if err != nil {
			return nil, fmt.Errorf("--var-file: %w", err)
		}
		fileVars := map[string]string{}
		if err := yaml.Unmarshal(data, &fileVars); err != nil {
			return nil, fmt.Errorf("--var-file %s: %w", varFile, err)
		}
		for k, v := range fileVars {
			vars[k] = v
		}
	}
	for _, kv := range cliVars {
		k, v, ok := strings.Cut(kv, "=")
		if !ok || k == "" {
			return nil, fmt.Errorf("--var %q: expected name=value", kv)
		}
		vars[k] = v
	}
	for _, env := range os.Environ() {
		k, v, _ := strings.Cut(env, "=")
		if name, ok := strings.CutPrefix(k, "ORQ_VAR_"); ok && name != "" {
			vars[name] = v
		}
	}
	return vars, nil
}

// Interpolate resolves ${var.*}, ${env.*} and {$file: ...} nodes in every
// manifest's spec, in place. Returned errors are anchored to the manifest.
func Interpolate(ms []Manifest, vars map[string]string) []ValidationError {
	var errs []ValidationError
	for i := range ms {
		m := &ms[i]
		bad := func(format string, a ...any) {
			errs = append(errs, ValidationError{File: m.File, Line: m.Line, Msg: fmt.Sprintf(format, a...)})
		}
		out, ok := interpNode(m.Spec, m, vars, bad)
		if ok {
			m.Spec, _ = out.(map[string]any)
		}
	}
	return errs
}

// interpNode rewrites one node. ok=false means the node produced an error and
// should be left as-is (error already reported via bad).
func interpNode(node any, m *Manifest, vars map[string]string, bad func(string, ...any)) (any, bool) {
	switch v := node.(type) {
	case map[string]any:
		// {$file: rel} — exactly one key — becomes the file's content.
		if raw, has := v["$file"]; has {
			if len(v) != 1 {
				bad("$file must be the only key of its object, found %d keys", len(v))
				return node, false
			}
			rel, _ := raw.(string)
			if rel == "" {
				bad("$file value must be a non-empty string")
				return node, false
			}
			p := rel
			if !filepath.IsAbs(p) {
				p = filepath.Join(filepath.Dir(m.absFile()), rel)
			}
			data, err := os.ReadFile(p)
			if err != nil {
				bad("$file %s: %v", rel, err)
				return node, false
			}
			return string(data), true
		}
		for k, child := range v {
			if out, ok := interpNode(child, m, vars, bad); ok {
				v[k] = out
			}
		}
		return v, true
	case []any:
		for i, child := range v {
			if out, ok := interpNode(child, m, vars, bad); ok {
				v[i] = out
			}
		}
		return v, true
	case string:
		return interpString(v, m, vars, bad)
	default:
		return node, true
	}
}

func interpString(s string, m *Manifest, vars map[string]string, bad func(string, ...any)) (any, bool) {
	failed := false
	out := interpRe.ReplaceAllStringFunc(s, func(match string) string {
		groups := interpRe.FindStringSubmatch(match)
		scope, name := groups[1], groups[2]
		switch scope {
		case "var":
			val, ok := vars[name]
			if !ok {
				bad("${var.%s} undefined — pass --var %s=… or add it to orq.yaml variables", name, name)
				failed = true
				return match
			}
			return val
		default: // env
			val := os.Getenv(name)
			if val == "" {
				bad("${env.%s} is not set in the environment", name)
				failed = true
				return match
			}
			m.HasSecrets = true
			return val
		}
	})
	if failed {
		return s, false
	}
	return out, true
}

// absFile resolves the manifest's source path for $file resolution. m.File is
// stack-relative when the stack dir is known; fall back to as-is.
func (m *Manifest) absFile() string {
	if m.absDir != "" {
		return filepath.Join(m.absDir, m.File)
	}
	return m.File
}
