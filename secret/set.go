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
// (`--force`). Upstream's `--help` describes `--force` as applying "when
// --token is used"; SetToken no longer passes `--token` (the token goes
// through stdin instead), so whether `--force` still affects the stdin path is
// unconfirmed. It is passed regardless — harmless if upstream ignores it here.
func WithOverwrite() SetOption { return func(c *setConfig) { c.overwrite = true } }

// SetToken stores a service secret, e.g. service "anthropic" or "github"
// (`sbx secret set [-g|SANDBOX] SERVICE`, token via stdin).
//
// The token is written to the child's stdin and never appears in the argument
// vector, so it is not visible in the host process list. Use SetRegistry for
// registry credentials, which has the same property.
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
