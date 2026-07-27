package secret

import (
	"context"
	"errors"
	"strings"

	"github.com/squall-chua/sbx-go-sdk/client"
)

type setConfig struct{ overwrite bool }

// SetOption configures SetToken and SetRegistry.
type SetOption func(*setConfig)

// WithOverwrite replaces an existing stored entry without confirmation
// (`--force`). Confirmed by a live `sbx secret set` run: `--force` works on
// the stdin path (piping a value in, no `--token`). Upstream's `--help` claims
// `--force` applies only "when --token is used" — that help text is wrong;
// don't let it override this observed behavior.
func WithOverwrite() SetOption { return func(c *setConfig) { c.overwrite = true } }

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

	args := []string{"secret", "set"}
	args = append(args, scopeArg(scope))
	args = append(args, service)
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
	if err := checkNotStored(ctx, c, scope, "registry", cred.Host, cfg.overwrite, "secret set", "WithOverwrite"); err != nil {
		return err
	}

	args := []string{"secret", "set"}
	args = append(args, scopeArg(scope))
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
