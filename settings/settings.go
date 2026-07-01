// Package settings reads and writes persistent sandboxd settings by shelling out
// to `sbx settings` (which edits settings.json; the daemon hot-reloads it within
// ~5s). Read paths use `--json` for a stable structured contract; mutations are
// fire-and-forget — Set/Unset return once the file is written, before the daemon
// reload. Shell-out failures return the raw *client.CLIError, as in policy/secret.
package settings

import (
	"encoding/json"
	"fmt"
	"strings"
)

// Setting is one entry from `sbx settings {get,list} --json`.
type Setting struct {
	Key         string          `json:"key"`
	Value       json.RawMessage `json:"value"` // typed JSON: bool/int/float/string/array/object
	Type        string          `json:"type"`  // "bool"|"int"|"float"|"string"|"json"
	Source      string          `json:"source"`
	Description string          `json:"description"`
}

// Bool decodes the value as a JSON boolean.
func (s Setting) Bool() (bool, error) {
	var b bool
	if err := json.Unmarshal(s.Value, &b); err != nil {
		return false, fmt.Errorf("setting %q value %s is not a bool: %w", s.Key, s.Value, err)
	}
	return b, nil
}

// Text renders the value as text: an unquoted scalar (string/number/bool) or the
// raw JSON for arrays/objects.
func (s Setting) Text() string {
	var str string
	if json.Unmarshal(s.Value, &str) == nil {
		return str
	}
	return strings.TrimSpace(string(s.Value))
}
