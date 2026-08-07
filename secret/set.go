package secret

import (
	"context"
	"errors"
	"strings"

	"github.com/squall-chua/sbx-go-sdk/client"
	"github.com/squall-chua/sbx-go-sdk/internal/oauthflow"
)

type setConfig struct{ overwrite, hostOnly bool }

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
	if token == "" {
		return errors.New("secret set: token must not be empty")
	}
	var cfg setConfig
	for _, o := range opts {
		o(&cfg)
	}
	if err := checkNotStored(ctx, c, scope, "service", service, cfg.overwrite, "secret set", "WithOverwrite"); err != nil {
		return err
	}

	args := []string{"secret", "set", service}
	args = append(args, scopeArgs(scope)...)
	if cfg.overwrite {
		args = append(args, "--force")
	}

	r, err := c.Runner()
	if err != nil {
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
