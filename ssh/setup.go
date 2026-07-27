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
