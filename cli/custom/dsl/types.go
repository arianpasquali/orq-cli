// Package dsl implements declarative workspace provisioning: YAML manifests
// describing orq resources, reconciled against the live workspace through the
// public v2 API (validate / plan / apply / pull / destroy).
package dsl

// Manifest is one YAML document: an apiVersion/kind/metadata envelope around a
// spec that mirrors the v2 API create/update body verbatim.
type Manifest struct {
	APIVersion string         `yaml:"apiVersion"`
	Kind       string         `yaml:"kind"`
	Metadata   Metadata       `yaml:"metadata"`
	Spec       map[string]any `yaml:"spec"`

	// Source position, for error messages. Not part of the document.
	File string `yaml:"-"`
	Line int    `yaml:"-"`
	// HasSecrets marks manifests that referenced ${env.*} at load time.
	HasSecrets bool `yaml:"-"`
	// absDir is the stack root File is relative to (for $file resolution).
	absDir string
}

// Metadata carries the declarative identity. Exactly one identity field is
// set, depending on the kind's identity mode: key (Agent, Evaluator,
// KnowledgeBase, Tool, MemoryStore), display_name (Prompt, Dataset, Skill) or
// name (Project).
type Metadata struct {
	Key         string `yaml:"key,omitempty"`
	Name        string `yaml:"name,omitempty"`
	DisplayName string `yaml:"display_name,omitempty"`
	Path        string `yaml:"path,omitempty"`
}

// StackConfig is orq.yaml at the stack root.
type StackConfig struct {
	Stack    string `yaml:"stack"`
	Defaults struct {
		Path string `yaml:"path"`
	} `yaml:"defaults"`
	Variables map[string]string `yaml:"variables"`

	// Dir is the directory orq.yaml was loaded from.
	Dir string `yaml:"-"`
}

// Identity is the stable declarative address of a resource within the stack:
// "<Kind>/<key>" for key kinds, "<Kind>/<path>|<display_name>" for
// display-name kinds (path disambiguates folders), "Project/<name>".
func (m Manifest) Identity() string {
	switch m.Kind {
	case "Project":
		return m.Kind + "/" + m.Metadata.Name
	case "Prompt", "Dataset", "Skill":
		return m.Kind + "/" + m.Metadata.Path + "|" + m.Metadata.DisplayName
	default:
		return m.Kind + "/" + m.Metadata.Key
	}
}

// IdentityValue is the bare identity field value sent to / matched against
// the API (no kind prefix, no path qualifier).
func (m Manifest) IdentityValue() string {
	switch m.Kind {
	case "Project":
		return m.Metadata.Name
	case "Prompt", "Dataset", "Skill":
		return m.Metadata.DisplayName
	default:
		return m.Metadata.Key
	}
}

// ValidationError is an offline-detectable problem, anchored to a source file.
type ValidationError struct {
	File string
	Line int
	Msg  string
}

func (e ValidationError) Error() string {
	if e.File == "" {
		return e.Msg
	}
	return e.File + ":" + itoa(e.Line) + "  " + e.Msg
}

// itoa avoids strconv import spreading through error paths.
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var b [20]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		b[i] = '-'
	}
	return string(b[i:])
}
