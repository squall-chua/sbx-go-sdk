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
// (`--force`).
func WithOverwrite() SetOption { return func(c *setConfig) { c.overwrite = true } }

// SetToken stores a service secret, e.g. service "anthropic" or "github"
// (`sbx secret set [-g|SANDBOX] SERVICE --token VALUE`).
//
// SECURITY: the token is passed as a command-line argument, so it is visible in
// the host process list for the lifetime of the child process. This is not an
// oversight — `--password-stdin` is registry-only upstream, and the only other
// non-argv path is an interactive terminal prompt an SDK cannot drive. Use
// SetRegistry for registry credentials, which does read the value from stdin.
// The name says "Token" precisely so this exposure is visible at the call site.
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
	args = append(args, service, "--token", token)
	if cfg.overwrite {
		args = append(args, "--force")
	}

	r, err := c.Runner()
	if err != nil {
		return err
	}
	_, err = r.Capture(ctx, nil, args...)
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
// The password is fed through stdin, so unlike SetToken it is never exposed in
// the host process list.
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
	_, err = r.CaptureStdin(ctx, strings.NewReader(cred.Password), nil, args...)
	return err
}
