package dsl

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

// PullReport summarizes a pull run.
type PullReport struct {
	Written  []string // relative file paths
	Warnings []string
	Skipped  []string // identities skipped (project undetectable)
}

// Pull serializes live workspace resources into manifest files under outDir.
// projectName scopes the pull; resources whose project cannot be determined
// (reads without path/project) are kept only when the stack state knows them.
func Pull(c *Client, projectName, outDir string, st *StateDoc, defaultPath string) (PullReport, error) {
	var report PullReport

	projectID := ""
	if projectName != "" {
		items, err := c.ListAll("/v2/projects", "")
		if err != nil {
			return report, err
		}
		for _, p := range items {
			if name, _ := p["name"].(string); name == projectName {
				projectID, _ = p["project_id"].(string)
				break
			}
		}
		if projectID == "" {
			return report, fmt.Errorf("project %q not found in the workspace", projectName)
		}
	}

	kinds := make([]KindInfo, 0, len(Registry()))
	for _, info := range Registry() {
		if info.Gated == "" && info.Kind != "Project" {
			kinds = append(kinds, info)
		}
	}
	sort.Slice(kinds, func(a, b int) bool { return kinds[a].Kind < kinds[b].Kind })

	// Symbolization index: server ids → identities, so pulled agent specs
	// carry `ref:` shapes (round-trip invariant), not raw ids. Live objects
	// beat state entries; MCP discovered-tool ids come from the live Tools.
	idMap := st.IDToIdentity()
	mcpIdx := map[string]mcpDiscovered{}
	prefetch := map[string][]map[string]any{}
	for _, info := range kinds {
		items, err := c.ListAll(info.BasePath, "")
		if err != nil {
			return report, err
		}
		prefetch[info.Kind] = items
		for _, raw := range items {
			id := extractID(raw, info)
			if id == "" {
				continue
			}
			switch info.IdentityMode {
			case "key":
				if k, _ := raw["key"].(string); k != "" {
					idMap[id] = info.Kind + "/" + k
				}
			case "display_name":
				if d, _ := raw["display_name"].(string); d != "" {
					idMap[id] = info.Kind + "/" + d
				}
			}
			if info.Kind == "Tool" {
				if mcp, ok := raw["mcp"].(map[string]any); ok {
					if tools, ok := mcp["tools"].([]any); ok {
						for _, t := range tools {
							if tm, ok := t.(map[string]any); ok {
								name, _ := tm["name"].(string)
								tid, _ := tm["id"].(string)
								if name != "" && tid != "" {
									key, _ := raw["key"].(string)
									mcpIdx[tid] = mcpDiscovered{ParentIdentity: "Tool/" + key, Name: name}
								}
							}
						}
					}
				}
			}
		}
	}

	for _, info := range kinds {
		for _, raw := range prefetch[info.Kind] {
			m, keep, warn := liveToManifest(raw, info, projectName, projectID, st, defaultPath)
			if !keep {
				// Out-of-scope or anonymous resources are skipped silently —
				// warning about files never written is noise.
				if m != nil && m.IdentityValue() != "" {
					report.Skipped = append(report.Skipped, m.Identity())
				}
				continue
			}
			if warn != "" && m.IdentityValue() != "" {
				report.Warnings = append(report.Warnings, warn)
			}
			m.Spec = symbolizeLive(m.Spec, m.Kind, idMap, mcpIdx)
			redactions := redactSecrets(m, info)
			report.Warnings = append(report.Warnings, redactions...)

			rel := filepath.Join(info.Plural, fsSafe(m.IdentityValue())+".yaml")
			if err := writeManifestFile(filepath.Join(outDir, rel), m); err != nil {
				return report, err
			}
			report.Written = append(report.Written, rel)
		}
	}
	sort.Strings(report.Written)
	return report, nil
}

// liveToManifest converts a live object into a manifest (envelope + spec).
func liveToManifest(raw map[string]any, info KindInfo, projectName, projectID string, st *StateDoc, defaultPath string) (*Manifest, bool, string) {
	m := &Manifest{APIVersion: "orq.ai/v1", Kind: info.Kind}

	switch info.IdentityMode {
	case "key":
		m.Metadata.Key, _ = raw["key"].(string)
	case "display_name":
		m.Metadata.DisplayName, _ = raw["display_name"].(string)
	}
	// Reserved state skills never pull.
	if info.Kind == "Skill" && strings.HasPrefix(m.Metadata.DisplayName, stateSkillPrefix) {
		return m, false, ""
	}
	if m.IdentityValue() == "" {
		return nil, false, "" // anonymous live object — nothing to address it by
	}

	// display_name as optional label on key kinds.
	if info.IdentityMode == "key" {
		if dn, _ := raw["display_name"].(string); dn != "" && dn != m.Metadata.Key {
			m.Metadata.DisplayName = dn
		}
	}

	// Path resolution: live → state → default(+warning).
	warn := ""
	if p, _ := raw["path"].(string); p != "" {
		m.Metadata.Path = p
	} else if r := findStateByID(st, extractID(raw, info)); r != nil && r.Path != "" {
		m.Metadata.Path = r.Path
	} else {
		m.Metadata.Path = defaultPath
		warn = fmt.Sprintf("%s: path not recoverable from the API (platform gap) — wrote default %q", m.Identity(), defaultPath)
	}

	// Project scoping.
	if projectName != "" {
		inProject := false
		first, _, _ := strings.Cut(m.Metadata.Path, "/")
		if first == projectName {
			inProject = true
		}
		if pid, _ := raw["project_id"].(string); pid != "" && pid == projectID {
			inProject = true
		}
		if did, _ := raw["domain_id"].(string); did != "" && did == projectID {
			inProject = true
		}
		if !inProject {
			return m, false, ""
		}
	}

	spec := NormalizeLive(raw, info)
	m.Spec = spec
	return m, true, warn
}

func findStateByID(st *StateDoc, id string) *StateResource {
	if st == nil || id == "" {
		return nil
	}
	for i := range st.Resources {
		if st.Resources[i].ServerID == id {
			return &st.Resources[i]
		}
	}
	return nil
}

// redactSecrets replaces secret spec paths with ${env.*} placeholders and
// returns one warning per redaction.
func redactSecrets(m *Manifest, info KindInfo) []string {
	var warnings []string
	for _, path := range info.Secret {
		if strings.HasSuffix(path, ".*") {
			base := strings.TrimSuffix(path, ".*")
			node, ok := getPath(m.Spec, base).(map[string]any)
			if !ok {
				continue
			}
			for header, v := range node {
				hv, ok := v.(map[string]any)
				if !ok {
					continue
				}
				if _, has := hv["value"]; !has {
					continue
				}
				envName := envPlaceholder(m, header)
				hv["value"] = "${env." + envName + "}"
				warnings = append(warnings, fmt.Sprintf("%s: %s.%s.value redacted → ${env.%s} — set it before apply", m.Identity(), base, header, envName))
			}
			continue
		}
		parent, last := splitPath(path)
		node, ok := getPath(m.Spec, parent).(map[string]any)
		if !ok {
			continue
		}
		if _, has := node[last]; !has {
			continue
		}
		envName := envPlaceholder(m, last)
		node[last] = "${env." + envName + "}"
		warnings = append(warnings, fmt.Sprintf("%s: %s redacted → ${env.%s} — set it before apply", m.Identity(), path, envName))
	}
	return warnings
}

func envPlaceholder(m *Manifest, field string) string {
	up := func(s string) string {
		s = strings.ToUpper(s)
		return strings.Map(func(r rune) rune {
			if (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') {
				return r
			}
			return '_'
		}, s)
	}
	return up(m.IdentityValue()) + "_" + up(field)
}

func getPath(spec map[string]any, dotted string) any {
	if dotted == "" {
		return spec
	}
	cur := any(spec)
	for _, seg := range strings.Split(dotted, ".") {
		mp, ok := cur.(map[string]any)
		if !ok {
			return nil
		}
		cur = mp[seg]
	}
	return cur
}

func splitPath(dotted string) (string, string) {
	if i := strings.LastIndex(dotted, "."); i >= 0 {
		return dotted[:i], dotted[i+1:]
	}
	return "", dotted
}

func fsSafe(s string) string {
	return strings.Map(func(r rune) rune {
		switch r {
		case '/', '|', ':', ' ':
			return '_'
		}
		return r
	}, s)
}

func writeManifestFile(path string, m *Manifest) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	var buf strings.Builder
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(m); err != nil {
		return err
	}
	enc.Close()
	return os.WriteFile(path, []byte(buf.String()), 0o644)
}

// Init scaffolds a new stack directory. project names the pre-existing orq
// project resources live in (API keys are project-scoped, so the stack never
// creates it); empty means "same as the stack name".
func Init(dir, stack, project string) ([]string, error) {
	if stack == "" {
		abs, err := filepath.Abs(dir)
		if err != nil {
			return nil, err
		}
		stack = strings.ToLower(filepath.Base(abs))
	}
	if !stackNameRe.MatchString(stack) {
		return nil, fmt.Errorf("stack name %q must match %s (lowercase kebab)", stack, stackNameRe)
	}
	if project == "" {
		project = stack
	}
	if _, err := os.Stat(filepath.Join(dir, "orq.yaml")); err == nil {
		return nil, fmt.Errorf("orq.yaml already exists in %s", dir)
	}
	files := map[string]string{
		"orq.yaml": fmt.Sprintf(`# orq stack configuration.
stack: %s

defaults:
  # metadata.path fallback for every manifest (project[/folder]).
  # The project must already exist — create it in the UI when minting the
  # project-scoped API key. The stack manages resources inside it only.
  path: %s

variables:
  default_model: mistral/mistral-large-latest
`, stack, project),
		"agents/example-agent.yaml": `apiVersion: orq.ai/v1
kind: Agent
metadata:
  # Agent keys are WORKSPACE-unique. ${var.stack} is a builtin (the stack
  # name); ${var.unique} is a deterministic 8-char hash of it, bicep-style.
  key: ${var.stack}-example-agent
  display_name: Example Agent
spec:
  role: Assistant
  description: Minimal example agent scaffolded by orq stack init.
  instructions: |
    You are a helpful assistant.
  model: ${var.default_model}
  settings:
    max_iterations: 10
    tools:
      - type: current_date
`,
		"vars/example.yaml": `# Per-environment overrides: orq stack plan -f . --var-file vars/example.yaml
default_model: mistral/mistral-large-latest
`,
	}
	var written []string
	for rel, content := range files {
		p := filepath.Join(dir, rel)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			return written, err
		}
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			return written, err
		}
		written = append(written, rel)
	}
	sort.Strings(written)
	return written, nil
}
