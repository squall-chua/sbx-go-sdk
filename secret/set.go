package secret

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/squall-chua/sbx-go-sdk/client"
	"github.com/squall-chua/sbx-go-sdk/internal/oauthflow"
)

type setConfig struct {
	overwrite, hostOnly bool
	ref, command        string
	refresh             string
	noVerify, showError bool
}

// resolverArgs returns the flags that make the daemon fetch the value on
// demand, and reports whether a resolver was configured at all.
func (c *setConfig) resolverArgs() ([]string, bool) {
	var args []string
	switch {
	case c.ref != "":
		args = append(args, "--ref", c.ref)
	case c.command != "":
		args = append(args, "--command", c.command)
	default:
		return nil, false
	}
	if c.refresh != "" {
		args = append(args, "--refresh", c.refresh)
	}
	if c.noVerify {
		args = append(args, "--no-verify")
	}
	if c.showError {
		args = append(args, "--show-error")
	}
	return args, true
}

// validateSource rejects a call that names no value source, or two.
func (c *setConfig) validateSource(op, literal string) error {
	if c.ref != "" && c.command != "" {
		return fmt.Errorf("%s: FromRef and FromCommand are mutually exclusive", op)
	}
	if literal != "" && (c.ref != "" || c.command != "") {
		return fmt.Errorf("%s: a literal value and a resolver are mutually exclusive", op)
	}
	if literal == "" && c.ref == "" && c.command == "" {
		return fmt.Errorf("%s: needs a value, or FromRef/FromCommand to resolve one", op)
	}
	return nil
}

// SetOption configures SetToken and SetRegistry.
type SetOption func(*setConfig)

// WithOverwrite replaces an existing stored entry without confirmation
// (`--force`). Confirmed by a live `sbx secret set` run: `--force` works on
// the stdin path (piping a value in, no `--token`). Upstream's `--help` claims
// `--force` applies only "when --token is used" — that help text is wrong;
// don't let it override this observed behavior.
func WithOverwrite() SetOption { return func(c *setConfig) { c.overwrite = true } }

// WithHostOnly stores a registry credential in the host-only scope: it is used
// for template and kit pulls on the host and injected into no sandbox at all.
// This is the CLI's own default as of sbx v0.38.0, and it lists as scope
// "(host only)". SetRegistry without it keeps the SDK's original behaviour and
// stores a "(global)" entry injected into every sandbox (`--all-sandboxes`).
//
// Ignored by SetToken: service secrets have no host-only scope.
func WithHostOnly() SetOption { return func(c *setConfig) { c.hostOnly = true } }

// SetToken stores a service secret, e.g. service "anthropic" or "github"
// (`sbx secret set [-g|SANDBOX] SERVICE`, token via stdin).
//
// The token is written to the child's stdin and never appears in the argument
// vector, so it is not visible in the host process list. Use SetRegistry for
// registry credentials, which has the same property.
//
// An entry already stored for service in scope is an error unless
// WithOverwrite is passed — checked before the CLI is invoked, so a pending
// secret is never consumed as the answer to the CLI's own overwrite prompt.
// That check itself depends on `sbx secret ls` succeeding; if it fails (e.g.
// a CLI table format change), the write is blocked rather than risking a
// silent no-op.
func SetToken(ctx context.Context, c *client.Client, scope, service, token string, opts ...SetOption) error {
	if service == "" {
		return errors.New("secret set: service must not be empty")
	}
	var cfg setConfig
	for _, o := range opts {
		o(&cfg)
	}
	if err := cfg.validateSource("secret set", token); err != nil {
		return err
	}
	if err := checkNotStored(ctx, c, scope, "service", service, cfg.overwrite, "secret set", "WithOverwrite"); err != nil {
		return err
	}

	args := []string{"secret", "set", service}
	args = append(args, scopeArgs(scope)...)
	if cfg.overwrite {
		args = append(args, "--force")
	}
	resolver, resolved := cfg.resolverArgs()
	args = append(args, resolver...)

	r, err := c.Runner()
	if err != nil {
		return err
	}
	// A resolver carries no value to hide, so there is nothing to pipe.
	if resolved {
		_, err = r.Capture(ctx, nil, args...)
		return err
	}
	_, err = r.CaptureStdin(ctx, strings.NewReader(token+"\n"), nil, args...)
	return err
}

// SetOAuth stores a service credential obtained through an OAuth handshake
// rather than a token (`sbx secret set SERVICE --oauth`). Global scope only —
// the CLI supports no other, and upstream documents the flow for "openai".
//
// With no terminal the CLI prints an authorization URL and then waits on a
// loopback callback until the user completes consent. SetOAuth hands that URL to
// onURL as soon as it appears and blocks until the flow finishes, so the caller
// decides how to present it — print it, open a browser, post it to a chat.
// Cancel ctx to abandon a flow nobody completes.
//
// onURL is called once, from another goroutine, while the child still runs; keep
// it quick and make it safe to call concurrently.
//
// Unlike SetToken this runs no pre-flight existing-entry check: the CLI's own
// overwrite prompt for an OAuth-configured service is part of the interactive
// flow, not something the SDK can answer ahead of it.
//
// The URL emission, the blocking, and cancellation are verified against sbx
// v0.38.0; the success path is not — completing it needs a human at a browser.
func SetOAuth(ctx context.Context, c *client.Client, service string, onURL func(string)) error {
	if service == "" {
		return errors.New("secret set: service must not be empty")
	}
	r, err := c.Runner()
	if err != nil {
		return err
	}
	return oauthflow.Run(ctx, r, onURL, "secret", "set", service, "--oauth")
}

// RegistryCredential is a container-registry pull credential.
type RegistryCredential struct {
	// Host is the registry hostname, e.g. "ghcr.io" or "myregistry.azurecr.io".
	Host string
	// Username is optional; omit it for token-only authentication.
	Username string
	// Password is the password or token. It is written to the child's stdin and
	// never appears in the argument vector.
	Password string
}

// SetRegistry stores a registry pull credential
// (`sbx secret set [-g|SANDBOX] --registry HOST --password-stdin`).
//
// The password is written to the child's stdin and never appears in the
// argument vector, so it is not visible in the host process list.
//
// An entry already stored for cred.Host in scope is an error unless
// WithOverwrite is passed — checked before the CLI is invoked, so a pending
// password is never consumed as the answer to the CLI's own overwrite prompt.
// That check itself depends on `sbx secret ls` succeeding; if it fails (e.g.
// a CLI table format change), the write is blocked rather than risking a
// silent no-op.
func SetRegistry(ctx context.Context, c *client.Client, scope string, cred RegistryCredential, opts ...SetOption) error {
	if cred.Host == "" {
		return errors.New("secret set: registry host must not be empty")
	}
	if cred.Password == "" {
		return errors.New("secret set: registry password must not be empty")
	}
	var cfg setConfig
	for _, o := range opts {
		o(&cfg)
	}
	// A host-only entry lists under its own scope, so the pre-flight check has to
	// look there rather than at the global rows.
	checkScope := scope
	if scope == "" && cfg.hostOnly {
		checkScope = HostOnlyScope
	}
	if err := checkNotStored(ctx, c, checkScope, "registry", cred.Host, cfg.overwrite, "secret set", "WithOverwrite"); err != nil {
		return err
	}

	args := []string{"secret", "set"}
	switch {
	case scope != "":
		args = append(args, scopeArgs(scope)...)
	case !cfg.hostOnly:
		// Global for a registry credential means "injected into every sandbox",
		// which sbx v0.38.0 spells --all-sandboxes; a bare `secret set --registry`
		// now stores a host-only entry instead. See WithHostOnly.
		args = append(args, "--all-sandboxes")
	}
	args = append(args, "--registry", cred.Host, "--password-stdin")
	if cred.Username != "" {
		args = append(args, "--username", cred.Username)
	}
	if cfg.overwrite {
		args = append(args, "--force")
	}

	r, err := c.Runner()
	if err != nil {
		return err
	}
	_, err = r.CaptureStdin(ctx, strings.NewReader(cred.Password+"\n"), nil, args...)
	return err
}

// FromRef resolves the secret from an external store instead of storing a
// literal (`--ref`, added in sbx v0.39.0). Two forms are accepted upstream: a
// 1Password reference, "op://vault/item/field", and an AWS Secrets Manager ARN.
//
// The daemon resolves it on demand, so the value never enters the secret store
// — only the reference does. Resolution runs on the *host*: a 1Password ref
// needs the `op` binary on PATH and an unlocked session, an ARN needs AWS
// credentials the daemon can see. Neither is checked until the value is
// actually needed, except once at store time, which WithoutVerify skips.
//
// Mutually exclusive with FromCommand, and with passing a literal value.
func FromRef(ref string) SetOption { return func(c *setConfig) { c.ref = ref } }

// FromCommand resolves the secret from a command's standard output instead of
// storing a literal (`--command`, added in sbx v0.39.0).
//
// The command text is stored by the daemon and replayed on demand, so put any
// environment it needs in the command itself or in a wrapper script. Never
// embed a secret in the command: the text appears in the argument vector when
// this call runs, in shell history, and in `sbx secret ls` output — upstream
// prints that same warning. FromRef is the safer choice where it fits.
//
// Mutually exclusive with FromRef, and with passing a literal value.
func FromCommand(cmd string) SetOption { return func(c *setConfig) { c.command = cmd } }

// WithRefresh sets how often a resolved secret is re-fetched (`--refresh`):
// "on-demand", or a duration such as "30m". Upstream defaults service secrets
// to "55m" and custom secrets to "on-demand".
//
// Ignored without FromRef or FromCommand — a literal value has nothing to
// refresh from.
func WithRefresh(policy string) SetOption { return func(c *setConfig) { c.refresh = policy } }

// WithoutVerify skips the one-off check that the resolver actually works when
// the secret is stored (`--no-verify`).
//
// Without it a broken resolver fails loudly and immediately — a missing `op`
// binary reports "verify ref failed (not_found)", a command exiting non-zero
// reports "verify command failed (exit_status)" — and nothing is stored. Skip
// the check only when the resolver cannot succeed yet at store time, such as a
// vault that is still locked.
func WithoutVerify() SetOption { return func(c *setConfig) { c.noVerify = true } }

// WithResolverErrors includes the resolver's standard error in the failure
// message when the store-time check fails (`--show-error`).
//
// Off by default for a reason upstream states plainly: that output may contain
// secrets. Turn it on to debug a resolver, not in production logging.
func WithResolverErrors() SetOption { return func(c *setConfig) { c.showError = true } }
