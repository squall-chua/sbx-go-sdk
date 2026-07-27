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
	Host        string   // target host whose outbound requests get the real secret (exact, IP, or wildcard e.g. "*.example.com")
	Hosts       []string // additional target hosts covered by the same secret (repeatable --host, sbx v0.33.0)
	Env         string   // env var set (to the placeholder) inside the sandbox
	Value       string   // the real secret
	Placeholder string   // optional; supports a {rand} suffix
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
//
// Unlike SetToken, SetRegistry and Import, SetCustom has no pre-flight
// existing-entry check and pipes nothing to stdin. If `set-custom` prompts to
// overwrite an existing entry, it may hit the same silent-cancel-exit-0 shape
// those three were fixed for (see README's "Known deviations & limitations")
// — not verified either way; fixing it is out of scope here.
func SetCustom(ctx context.Context, c *client.Client, scope string, s CustomSecret) error {
	args := []string{"secret", "set-custom", scopeArg(scope)}
	if s.Host != "" {
		args = append(args, "--host", s.Host)
	}
	for _, h := range s.Hosts {
		args = append(args, "--host", h)
	}
	args = append(args, "--env", s.Env, "--value", s.Value)
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
	secretCustomHeader = []string{"SCOPE", "TARGETS", "ENV", "PLACEHOLDER", "SECRET"}
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
	Targets     string // target host(s); comma-joined when one secret covers several (sbx v0.33.0)
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
//
// An empty scope lists every scope, not just global — the CLI's `-g` flag is
// the only way to ask for global-only, and List has no way to pass it.
func List(ctx context.Context, c *client.Client, scope string) (*Secrets, error) {
	raw, err := ListRaw(ctx, c, scope)
	if err != nil {
		return nil, err
	}
	return parseSecretList(raw)
}

// ListRaw returns the raw `sbx secret ls [SCOPE]` text. An empty scope lists
// every scope, not just global — only `-g` means global-only, and ListRaw has
// no way to request that.
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
			Targets:     r["TARGETS"],
			Env:         r["ENV"],
			Placeholder: r["PLACEHOLDER"],
			ValueMasked: r["SECRET"],
		})
	}
	return out, nil
}

// checkNotStored returns an error if scope already has a Stored row of type
// typ named name and overwrite is false — used before SetToken, SetRegistry
// and Import invoke the CLI, so a pre-existing entry is rejected before ever
// reaching the CLI's own y/N overwrite prompt. Without --force, that prompt
// blocks on non-interactive stdin and sbx cancels, exiting 0 — a silent
// no-op the caller would otherwise mistake for success. For SetToken and
// SetRegistry there's a second reason: they pipe the secret value to stdin
// (via CaptureStdin), so the prompt would otherwise read that piped value as
// its own answer; Import pipes nothing (it uses Capture), so only the
// silent-cancel-exit-0 risk applies there. verb and optionName customize the
// error message per caller.
//
// This check depends on `sbx secret ls` succeeding: if List fails (e.g.
// client.ErrUnexpectedFormat from a CLI table format change), the write is
// blocked rather than risking the silent no-op above — see the wrapped error
// below.
//
// List(ctx, c, "") maps to bare `sbx secret ls` — the CLI lists every scope,
// not global-only (`-g` is the only way to ask the CLI for global-only) — so
// this filters rows by s.Scope == scope itself rather than trusting the
// listing to already be scope-restricted; both use the same "" == global
// convention, so the comparison is exact.
func checkNotStored(ctx context.Context, c *client.Client, scope, typ, name string, overwrite bool, verb, optionName string) error {
	if overwrite {
		return nil
	}
	existing, err := List(ctx, c, scope)
	if err != nil {
		return fmt.Errorf("%s: cannot check existing secrets: %w", verb, err)
	}
	for _, s := range existing.Stored {
		if s.Scope == scope && s.Type == typ && s.Name == name {
			return fmt.Errorf("%s: %q already exists in this scope; pass %s() to replace it", verb, name, optionName)
		}
	}
	return nil
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
// ("" = global). This uses `secret rm --host` — not the positional service name
// Remove takes. Idempotent: the CLI exits 0 and reports "Deleted 0" when nothing
// matches. (The --host flag is absent from `sbx secret rm --help` but is supported.)
//
// Limitation (verified sbx v0.34.0): rm --host only matches single-host entries. A
// custom secret created with multiple Hosts (one secret covering several targets)
// cannot be removed by any one of its hosts — rm reports "Deleted 0". To delete such
// an entry, remove it by placeholder instead — `sbx secret rm --placeholder <ph>`
// deletes the whole entry regardless of host count (custom secrets are keyed by
// placeholder). The SDK exposes only the host-keyed path here; use the CLI for the
// placeholder path.
func RemoveCustom(ctx context.Context, c *client.Client, scope, host string) error {
	r, err := c.Runner()
	if err != nil {
		return err
	}
	_, err = r.Capture(ctx, nil, "secret", "rm", scopeArg(scope), "--host", host, "-f")
	return err
}
