package ssh

import (
	"context"

	"github.com/squall-chua/sbx-go-sdk/client"
)

type setupConfig struct {
	alias string
}

// Option configures Setup.
type Option func(*setupConfig)

// WithAlias sets the ssh_config Host pattern to write (default "*.sbx" upstream).
func WithAlias(alias string) Option { return func(c *setupConfig) { c.alias = alias } }

// Setup provisions the local SSH client for the sandbox endpoint
// (`sbx setup ssh`): it writes an ~/.ssh/config "Host *.sbx" block plus
// known_hosts. No client key is needed — authentication is the daemon socket's
// OS-user boundary plus an active Docker login. Idempotent and non-interactive;
// safe to re-run.
//
// v0.37.0 made `sbx setup ssh` the documented path. `sbx ssh setup` still works
// as a hidden alias with identical flags, but is not documented and may be
// withdrawn, so this calls the documented form.
func Setup(ctx context.Context, c *client.Client, opts ...Option) error {
	var cfg setupConfig
	for _, o := range opts {
		o(&cfg)
	}
	args := []string{"setup", "ssh"}
	if cfg.alias != "" {
		args = append(args, "--alias", cfg.alias)
	}
	r, err := c.Runner()
	if err != nil {
		return err
	}
	_, err = r.Capture(ctx, nil, args...)
	return err
}

// RemoveSetup undoes what Setup wrote (`sbx setup ssh remove`, added in sbx
// v0.39.0): it drops this app instance's generated SSH config and the include
// line pointing at it from ~/.ssh/config.
//
// It removes the local app instance's own config only, so a differently-named
// instance's block survives. Idempotent, like Setup.
//
// This is client-side provisioning, not the endpoint itself: it does not
// disable the feature.ssh setting, which is what Disable does.
func RemoveSetup(ctx context.Context, c *client.Client) error {
	r, err := c.Runner()
	if err != nil {
		return err
	}
	_, err = r.Capture(ctx, nil, "setup", "ssh", "remove")
	return err
}
