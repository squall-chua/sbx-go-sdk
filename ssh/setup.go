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

// Setup provisions the local SSH client for the sandbox endpoint (`sbx ssh setup`):
// it writes an ~/.ssh/config "Host *.sbx" block + known_hosts. No client key is
// needed (auth is the daemon socket + Docker login). Idempotent and
// non-interactive; safe to re-run.
func Setup(ctx context.Context, c *client.Client, opts ...Option) error {
	var cfg setupConfig
	for _, o := range opts {
		o(&cfg)
	}
	args := []string{"ssh", "setup"}
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
