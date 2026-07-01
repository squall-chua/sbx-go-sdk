package ssh

import (
	"context"

	"github.com/squall-chua/sbx-go-sdk/client"
)

type setupConfig struct {
	alias      string
	regenerate bool
}

// Option configures Setup.
type Option func(*setupConfig)

// WithAlias sets the ssh_config Host alias to write (default "sbx" upstream).
func WithAlias(alias string) Option { return func(c *setupConfig) { c.alias = alias } }

// WithRegenerate rotates the managed SSH key.
func WithRegenerate() Option { return func(c *setupConfig) { c.regenerate = true } }

// Setup provisions the local SSH client for the sandbox endpoint (`sbx ssh setup`):
// it writes an ~/.ssh/config alias + a dedicated managed key. Idempotent and
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
	if cfg.regenerate {
		args = append(args, "--regenerate")
	}
	r, err := c.Runner()
	if err != nil {
		return err
	}
	_, err = r.Capture(ctx, nil, args...)
	return err
}
