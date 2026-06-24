// Package secret manages stored sandbox secrets via shell-out to `sbx secret`.
// For headless agent credentials, prefer exec.WithEnv; SetCustom is EXPERIMENTAL
// upstream.
package secret

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/squall-chua/sbx-go-sdk/client"
	"github.com/squall-chua/sbx-go-sdk/internal/coltable"
)

// CustomSecret describes a custom proxy-injected secret.
type CustomSecret struct {
	Host        string // target host whose outbound requests get the real secret
	Env         string // env var set (to the placeholder) inside the sandbox
	Value       string // the real secret
	Placeholder string // optional; supports a {rand} suffix
}

// scopeArg returns "-g" for global ("") or the sandbox name as a positional arg.
// NOTE: `sbx secret` uses "-g"/bare positional; `sbx policy` uses "--sandbox NAME"
// (see policy.scopeArgs). The encodings differ per CLI on purpose — do not unify.
func scopeArg(scope string) string {
	if scope == "" {
		return "-g"
	}
	return scope
}

// SetCustom creates/updates a custom secret in scope ("" = global). EXPERIMENTAL.
// The Value is passed as a `sbx secret set-custom --value` CLI argument, so it is
// briefly visible in host process listings.
func SetCustom(ctx context.Context, c *client.Client, scope string, s CustomSecret) error {
	args := []string{"secret", "set-custom", scopeArg(scope), "--host", s.Host, "--env", s.Env, "--value", s.Value}
	if s.Placeholder != "" {
		args = append(args, "--placeholder", s.Placeholder)
	}
	r, err := c.Runner()
	if err != nil {
		return err
	}
	_, err = r.Capture(ctx, nil, args...)
	return err
}

// Header columns of the two `sbx secret ls` tables, in order. Drift yields
// client.ErrUnexpectedFormat.
var (
	secretStdHeader    = []string{"SCOPE", "TYPE", "NAME", "SECRET"}
	secretCustomHeader = []string{"SCOPE", "TARGET", "ENV", "PLACEHOLDER", "SECRET"}
)

// Stored is a service or registry secret row (`sbx secret set`). Type is
// "service" or "registry".
type Stored struct {
	Scope       string // "" = global, else sandbox name
	Type        string // "service" | "registry"
	Name        string // service name or registry host
	ValueMasked string // masked display value — never the real secret
}

// Custom is a custom secret row (`sbx secret set-custom`).
type Custom struct {
	Scope       string // "" = global, else sandbox name
	Target      string // target host
	Env         string // env var injected into the sandbox
	Placeholder string
	ValueMasked string // masked display value
}

// Secrets is the parsed `sbx secret ls` output: the standard table (service +
// registry) and the custom-secrets table.
type Secrets struct {
	Stored []Stored
	Custom []Custom
}

// List returns the parsed `sbx secret ls [SCOPE]` output. A format change in the
// CLI's tables yields client.ErrUnexpectedFormat — use ListRaw to fall back.
func List(ctx context.Context, c *client.Client, scope string) (*Secrets, error) {
	raw, err := ListRaw(ctx, c, scope)
	if err != nil {
		return nil, err
	}
	return parseSecretList(raw)
}

// ListRaw returns the raw `sbx secret ls [SCOPE]` text.
func ListRaw(ctx context.Context, c *client.Client, scope string) (string, error) {
	args := []string{"secret", "ls"}
	if scope != "" {
		args = append(args, scope)
	}
	r, err := c.Runner()
	if err != nil {
		return "", err
	}
	return r.Capture(ctx, nil, args...)
}

// parseSecretList splits the output into the standard and custom sections and
// parses each. A missing header in a section means that section is empty.
func parseSecretList(raw string) (*Secrets, error) {
	std, custom := splitCustomSection(raw)
	out := &Secrets{}

	rows, err := coltable.Parse(std, secretStdHeader)
	if err != nil && !errors.Is(err, coltable.ErrNoHeader) {
		return nil, fmt.Errorf("secret list: %w: %w", client.ErrUnexpectedFormat, err)
	}
	for _, r := range rows {
		out.Stored = append(out.Stored, Stored{
			Scope:       normScope(r["SCOPE"]),
			Type:        r["TYPE"],
			Name:        r["NAME"],
			ValueMasked: r["SECRET"],
		})
	}

	crows, err := coltable.Parse(custom, secretCustomHeader)
	if err != nil && !errors.Is(err, coltable.ErrNoHeader) {
		return nil, fmt.Errorf("secret list: %w: %w", client.ErrUnexpectedFormat, err)
	}
	for _, r := range crows {
		out.Custom = append(out.Custom, Custom{
			Scope:       normScope(r["SCOPE"]),
			Target:      r["TARGET"],
			Env:         r["ENV"],
			Placeholder: r["PLACEHOLDER"],
			ValueMasked: r["SECRET"],
		})
	}
	return out, nil
}

// splitCustomSection splits raw at the "CUSTOM SECRETS" label line into the
// standard-table text and the custom-table text. With no label, everything is the
// standard section.
func splitCustomSection(raw string) (standard, custom string) {
	lines := strings.Split(strings.ReplaceAll(raw, "\r\n", "\n"), "\n")
	for i, ln := range lines {
		if strings.TrimSpace(ln) == "CUSTOM SECRETS" {
			return strings.Join(lines[:i], "\n"), strings.Join(lines[i+1:], "\n")
		}
	}
	return raw, ""
}

// normScope maps sbx's "(global)" to the SDK's "" global convention.
func normScope(s string) string {
	if s == "(global)" {
		return ""
	}
	return s
}

// Remove deletes a secret (service) in scope ("" = global). Uses -f to skip the
// confirmation prompt (the CLI would otherwise block on non-TTY stdin).
func Remove(ctx context.Context, c *client.Client, scope, service string) error {
	args := []string{"secret", "rm", scopeArg(scope)}
	if service != "" {
		args = append(args, service)
	}
	args = append(args, "-f")
	r, err := c.Runner()
	if err != nil {
		return err
	}
	_, err = r.Capture(ctx, nil, args...)
	return err
}

// RemoveCustom deletes the custom (set-custom) secret for a target host in scope
// ("" = global). Custom secrets are keyed by host, so this uses `secret rm --host`
// — not the positional service name Remove takes. Idempotent: the CLI exits 0 and
// reports "Deleted 0" when nothing matches. (The --host flag is absent from
// `sbx secret rm --help` but is supported.)
func RemoveCustom(ctx context.Context, c *client.Client, scope, host string) error {
	r, err := c.Runner()
	if err != nil {
		return err
	}
	_, err = r.Capture(ctx, nil, "secret", "rm", scopeArg(scope), "--host", host, "-f")
	return err
}
