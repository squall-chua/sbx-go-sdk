// Package secret manages stored sandbox secrets via shell-out to `sbx secret`.
// For headless agent credentials, prefer exec.WithEnv; SetCustom is EXPERIMENTAL
// upstream.
package secret

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/squall-chua/sbx-go-sdk/client"
)

// CustomSecret describes a custom proxy-injected secret.
type CustomSecret struct {
	Host        string   // target host whose outbound requests get the real secret (exact, IP, or wildcard e.g. "*.example.com")
	Hosts       []string // additional target hosts covered by the same secret (repeatable --host, sbx v0.33.0)
	Env         string   // env var set (to the placeholder) inside the sandbox
	Value       string   // the real secret; leave empty when a resolver option supplies it
	Placeholder string   // optional; supports a {rand} suffix
}

// scopeArgs returns nothing for global ("") or `--sandbox NAME` for a sandbox.
//
// sbx v0.38.0 made global the default scope for service and custom secrets and
// deprecated both older spellings: `-g` prints "Flag --global has been
// deprecated…" and a bare positional sandbox name prints "positional sandbox
// scope is deprecated". Those warnings land in the captured output the SDK
// parses, so the SDK emits the new form. This now matches policy.scopeArgs.
func scopeArgs(scope string) []string {
	if scope == "" {
		return nil
	}
	return []string{"--sandbox", scope}
}

// SetCustom creates/updates a custom secret in scope ("" = global). EXPERIMENTAL.
// The Value is passed as a `sbx secret set-custom --value` CLI argument, so it is
// briefly visible in host process listings. Pass FromRef or FromCommand
// instead of a Value to have the daemon resolve it on demand, which keeps the
// secret itself out of both the argument vector and the store.
//
// Unlike SetToken, SetRegistry and Import, SetCustom has no pre-flight
// existing-entry check and pipes nothing to stdin. If `set-custom` prompts to
// overwrite an existing entry, it may hit the same silent-cancel-exit-0 shape
// those three were fixed for (see README's "Known deviations & limitations")
// — not verified either way; fixing it is out of scope here.
func SetCustom(ctx context.Context, c *client.Client, scope string, s CustomSecret, opts ...SetOption) error {
	var cfg setConfig
	for _, o := range opts {
		o(&cfg)
	}
	if err := cfg.validateSource("secret set-custom", s.Value); err != nil {
		return err
	}

	args := append([]string{"secret", "set-custom"}, scopeArgs(scope)...)
	if s.Host != "" {
		args = append(args, "--host", s.Host)
	}
	for _, h := range s.Hosts {
		args = append(args, "--host", h)
	}
	args = append(args, "--env", s.Env)
	if resolver, resolved := cfg.resolverArgs(); resolved {
		args = append(args, resolver...)
	} else {
		args = append(args, "--value", s.Value)
	}
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

// Stored is a service or registry secret row (`sbx secret set`). Type is
// "service" or "registry".
type Stored struct {
	Scope string // "" = global, HostOnlyScope, else sandbox name
	Type  string // "service" | "registry"
	Name  string // service name or registry host
	// ValueMasked is what the text table's SECRET column shows — a mask, with
	// "USERNAME/" in front for a registry credential that has one. Never the
	// real secret.
	ValueMasked string
}

// Custom is a custom secret row (`sbx secret set-custom`).
type Custom struct {
	Scope       string // "" = global, else sandbox name
	Targets     string // target host(s); ", "-joined when one secret covers several (sbx v0.33.0)
	Env         string // env var injected into the sandbox
	Placeholder string
	// ValueMasked is what the text table's SECRET column shows. For a stored
	// literal that is a mask ("*****"); for a resolver it is the source and its
	// refresh policy instead, e.g. "command:vault read -field=k s/ai (30m)".
	// Never the real secret either way.
	ValueMasked string
}

// Secrets is the parsed `sbx secret ls` output: the standard list (service +
// registry) and the custom-secrets list.
type Secrets struct {
	Stored []Stored
	Custom []Custom
}

// List returns the parsed `sbx secret ls [--sandbox SCOPE] --json` output (the
// flag arrived in sbx v0.42.0). Output that is not the expected JSON yields
// client.ErrUnexpectedFormat — use ListRaw to fall back to the text.
//
// An empty scope lists every scope, not just global — the CLI's `-g` flag is
// the only way to ask for global-only, and List has no way to pass it.
func List(ctx context.Context, c *client.Client, scope string) (*Secrets, error) {
	args := append([]string{"secret", "ls"}, scopeArgs(scope)...)
	r, err := c.Runner()
	if err != nil {
		return nil, err
	}
	raw, err := r.Capture(ctx, nil, append(args, "--json")...)
	if err != nil {
		return nil, err
	}
	return parseSecretList(raw)
}

// ListRaw returns the raw `sbx secret ls [SCOPE]` text. An empty scope lists
// every scope, not just global — only `-g` means global-only, and ListRaw has
// no way to request that.
func ListRaw(ctx context.Context, c *client.Client, scope string) (string, error) {
	args := append([]string{"secret", "ls"}, scopeArgs(scope)...)
	r, err := c.Runner()
	if err != nil {
		return "", err
	}
	return r.Capture(ctx, nil, args...)
}

// secretJSON is one entry of either list in `sbx secret ls --json`.
type secretJSON struct {
	Scope       string   `json:"scope"`
	Type        string   `json:"type"`
	Name        string   `json:"name"`
	Targets     []string `json:"targets"`
	Env         string   `json:"env"`
	Placeholder string   `json:"placeholder"`
	Secret      string   `json:"secret"`
	Username    string   `json:"username"`
	Kind        string   `json:"kind"`
	Source      string   `json:"source"`
	Refresh     string   `json:"refresh"`
}

// masked rebuilds the text table's SECRET column, so ValueMasked reads the same
// as it did before List moved to --json.
func (s secretJSON) masked() string {
	switch {
	case s.Kind != "":
		return s.Kind + ":" + s.Source + " (" + s.Refresh + ")"
	case s.Username != "":
		return s.Username + "/" + s.Secret
	}
	return s.Secret
}

// parseSecretList decodes `sbx secret ls --json`. A missing list key is
// client.ErrUnexpectedFormat, never an empty result: the CLI prints both keys
// even when a scope holds nothing.
func parseSecretList(raw string) (*Secrets, error) {
	var v struct {
		Secrets       *[]secretJSON `json:"secrets"`
		CustomSecrets *[]secretJSON `json:"custom_secrets"`
	}
	if err := json.Unmarshal([]byte(raw), &v); err != nil {
		return nil, fmt.Errorf("secret list: %w: %w", client.ErrUnexpectedFormat, err)
	}
	if v.Secrets == nil || v.CustomSecrets == nil {
		return nil, fmt.Errorf("secret list: %w: no \"secrets\" or \"custom_secrets\" key in output", client.ErrUnexpectedFormat)
	}
	out := &Secrets{}
	for _, s := range *v.Secrets {
		out.Stored = append(out.Stored, Stored{
			Scope:       normScope(s.Scope),
			Type:        s.Type,
			Name:        s.Name,
			ValueMasked: s.masked(),
		})
	}
	for _, s := range *v.CustomSecrets {
		out.Custom = append(out.Custom, Custom{
			Scope:       normScope(s.Scope),
			Targets:     strings.Join(s.Targets, ", "),
			Env:         s.Env,
			Placeholder: s.Placeholder,
			ValueMasked: s.masked(),
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
// client.ErrUnexpectedFormat from a CLI output format change), the write is
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
	// HostOnlyScope is a display label, not a sandbox name, so it cannot narrow
	// the listing — list every scope and let the row filter below do the work.
	listScope := scope
	if listScope == HostOnlyScope {
		listScope = ""
	}
	existing, err := List(ctx, c, listScope)
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

// normScope maps the scope words of `secret ls --json` to the SDK's
// conventions: "global" is "" and "host-only" is HostOnlyScope. Any other
// value is a sandbox name.
func normScope(s string) string {
	switch s {
	case "global":
		return ""
	case "host-only":
		return HostOnlyScope
	}
	return s
}

// HostOnlyScope is the Stored.Scope value of a registry credential that is used
// for host-side template and kit pulls but injected into no sandbox — the third
// scope sbx v0.38.0 added alongside global and per-sandbox. It is not a sandbox
// name and cannot be passed as a scope argument; write such an entry with
// SetRegistry + WithHostOnly.
const HostOnlyScope = "(host only)"

// Remove deletes a secret (service) in scope ("" = global). Uses -f to skip the
// confirmation prompt (the CLI would otherwise block on non-TTY stdin).
func Remove(ctx context.Context, c *client.Client, scope, service string) error {
	args := append([]string{"secret", "rm"}, scopeArgs(scope)...)
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
	args := append([]string{"secret", "rm"}, scopeArgs(scope)...)
	args = append(args, "--host", host, "-f")
	_, err = r.Capture(ctx, nil, args...)
	return err
}
