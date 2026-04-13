package commands

import (
	"encoding/json"

	bartolocli "github.com/orq-ai/bartolo/cli"
)

// emit normalizes the value through encoding/json so the configured Formatter
// (json/yaml/toon) sees a generic value with json-tag-derived keys. TOON only
// honors `toon:` tags by default, so passing structs directly leaks Go field
// names into the output.
func emit(v any) error {
	data, err := json.Marshal(v)
	if err != nil {
		return err
	}
	var out any
	if err := json.Unmarshal(data, &out); err != nil {
		return err
	}
	return bartolocli.Formatter.Format(out)
}
