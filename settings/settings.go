// Package settings reads and writes persistent sandboxd settings by shelling out
// to `sbx settings` (which edits settings.json; the daemon hot-reloads it within
// ~5s). Read paths use `--json` for a stable structured contract; mutations are
// fire-and-forget — Set/Unset return once the file is written, before the daemon
// reload. Shell-out failures return the raw *client.CLIError, as in policy/secret.
package settings

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/squall-chua/sbx-go-sdk/client"
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

func capture(ctx context.Context, c *client.Client, args ...string) (string, error) {
	r, err := c.Runner()
	if err != nil {
		return "", err
	}
	return r.Capture(ctx, nil, args...)
}

// List returns all settings (`sbx settings list --json`).
func List(ctx context.Context, c *client.Client) ([]Setting, error) {
	out, err := capture(ctx, c, "settings", "list", "--json")
	if err != nil {
		return nil, err
	}
	var ss []Setting
	if err := json.Unmarshal([]byte(out), &ss); err != nil {
		return nil, fmt.Errorf("settings list: %w: %w", client.ErrUnexpectedFormat, err)
	}
	return ss, nil
}

// Get returns one setting (`sbx settings get --json <key>`). An undefined key makes
// the CLI exit non-zero, which surfaces as the raw *client.CLIError.
func Get(ctx context.Context, c *client.Client, key string) (*Setting, error) {
	out, err := capture(ctx, c, "settings", "get", "--json", key)
	if err != nil {
		return nil, err
	}
	var s Setting
	if err := json.Unmarshal([]byte(out), &s); err != nil {
		return nil, fmt.Errorf("settings get %q: %w: %w", key, client.ErrUnexpectedFormat, err)
	}
	return &s, nil
}

func run(ctx context.Context, c *client.Client, args ...string) error {
	_, err := capture(ctx, c, args...)
	return err
}

// Set writes an override (`sbx settings set <key> <value>`). The value is parsed by
// the setting's declared type; JSON values (e.g. `["docker.io/"]`) pass through as a
// single argument. Fire-and-forget: returns once the file is written; the daemon
// hot-reloads within ~5s.
func Set(ctx context.Context, c *client.Client, key, value string) error {
	return run(ctx, c, "settings", "set", key, value)
}

// Unset removes an override (`sbx settings unset <key>`), reverting to the env or
// default value. Fire-and-forget (see Set).
func Unset(ctx context.Context, c *client.Client, key string) error {
	return run(ctx, c, "settings", "unset", key)
}
