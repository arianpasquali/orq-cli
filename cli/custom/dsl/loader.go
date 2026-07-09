package dsl

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"
)

// stackNameRe: lowercase kebab — the stack name feeds the state skill's
// display_name (dashes become underscores there).
var stackNameRe = regexp.MustCompile(`^[a-z][a-z0-9-]*$`)

// LoadStack reads orq.yaml from dir. The file is required: it names the stack
// (the ownership scope every other command hangs off).
func LoadStack(dir string) (StackConfig, error) {
	var cfg StackConfig
	p := filepath.Join(dir, "orq.yaml")
	data, err := os.ReadFile(p)
	if err != nil {
		return cfg, fmt.Errorf("no orq.yaml in %s (run `orq dsl init`): %w", dir, err)
	}
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return cfg, fmt.Errorf("%s: %w", p, err)
	}
	if cfg.Stack == "" {
		return cfg, fmt.Errorf("%s: `stack` is required", p)
	}
	if !stackNameRe.MatchString(cfg.Stack) {
		return cfg, fmt.Errorf("%s: stack %q must match %s", p, cfg.Stack, stackNameRe)
	}
	if cfg.Variables == nil {
		cfg.Variables = map[string]string{}
	}
	cfg.Dir = dir
	return cfg, nil
}

// LoadManifests walks dir recursively for *.yaml / *.yml (excluding orq.yaml
// and anything under vars/), parses multi-document files, applies the stack's
// default path, and reports envelope-level problems as ValidationErrors.
func LoadManifests(dir string, cfg StackConfig) ([]Manifest, []ValidationError) {
	var (
		manifests []Manifest
		errs      []ValidationError
	)
	_ = filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			errs = append(errs, ValidationError{File: path, Msg: err.Error()})
			return nil
		}
		if d.IsDir() {
			if d.Name() == "vars" || strings.HasPrefix(d.Name(), ".") && path != dir {
				return filepath.SkipDir
			}
			return nil
		}
		ext := filepath.Ext(d.Name())
		if ext != ".yaml" && ext != ".yml" || d.Name() == "orq.yaml" {
			return nil
		}
		ms, es := parseManifestFile(path, cfg)
		manifests = append(manifests, ms...)
		errs = append(errs, es...)
		return nil
	})
	return manifests, errs
}

func parseManifestFile(path string, cfg StackConfig) ([]Manifest, []ValidationError) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, []ValidationError{{File: path, Msg: err.Error()}}
	}
	rel := path
	if r, err := filepath.Rel(cfg.Dir, path); err == nil && cfg.Dir != "" {
		rel = r
	}

	var (
		out  []Manifest
		errs []ValidationError
	)
	dec := yaml.NewDecoder(strings.NewReader(string(data)))
	for {
		var node yaml.Node
		err := dec.Decode(&node)
		if err != nil {
			if err.Error() == "EOF" {
				break
			}
			errs = append(errs, ValidationError{File: rel, Msg: "yaml: " + err.Error()})
			break
		}
		if node.Kind == 0 || (node.Kind == yaml.DocumentNode && len(node.Content) == 0) {
			continue // empty document
		}
		var m Manifest
		if err := node.Decode(&m); err != nil {
			errs = append(errs, ValidationError{File: rel, Line: node.Line, Msg: "yaml: " + err.Error()})
			continue
		}
		m.File, m.Line = rel, docLine(&node)
		if es := checkEnvelope(&m, cfg); len(es) > 0 {
			errs = append(errs, es...)
			continue
		}
		out = append(out, m)
	}
	return out, errs
}

// docLine reports the first content line of a document node.
func docLine(n *yaml.Node) int {
	if n.Kind == yaml.DocumentNode && len(n.Content) > 0 {
		return n.Content[0].Line
	}
	if n.Line > 0 {
		return n.Line
	}
	return 1
}

func checkEnvelope(m *Manifest, cfg StackConfig) []ValidationError {
	var errs []ValidationError
	bad := func(format string, a ...any) {
		errs = append(errs, ValidationError{File: m.File, Line: m.Line, Msg: fmt.Sprintf(format, a...)})
	}
	if m.APIVersion != "orq.ai/v1" {
		bad("apiVersion must be %q, got %q", "orq.ai/v1", m.APIVersion)
	}
	if m.Kind == "" {
		bad("kind is required")
		return errs
	}
	if m.Spec == nil {
		m.Spec = map[string]any{}
	}
	// Default path from stack config; Project carries no path.
	if m.Kind != "Project" && m.Metadata.Path == "" {
		if cfg.Defaults.Path == "" {
			bad("metadata.path is empty and orq.yaml sets no defaults.path")
		} else {
			m.Metadata.Path = cfg.Defaults.Path
		}
	}
	return errs
}
