package secret

import (
	"context"
	"errors"

	"github.com/squall-chua/sbx-go-sdk/client"
)

type importConfig struct {
	dryRun    bool
	overwrite bool
}

// ImportOption configures Import and ImportAll.
type ImportOption func(*importConfig)

// WithDryRun previews what would be imported without writing anything
// (`--dry-run`). Strongly recommended before ImportAll.
func WithDryRun() ImportOption { return func(c *importConfig) { c.dryRun = true } }

// WithOverwriteExisting replaces an already-stored entry without confirmation
// (`--force`).
func WithOverwriteExisting() ImportOption { return func(c *importConfig) { c.overwrite = true } }

// Import stores the secret detected in the host environment for one named
// service, e.g. "openai" reads OPENAI_API_KEY (`sbx secret import SERVICE`).
// Per `sbx secret import --help`, this always writes to the global keychain
// (there is no scope flag or positional).
//
// An entry already stored for service (global scope) is an error unless
// WithOverwriteExisting is passed — checked before the CLI is invoked, so a
// pending import is never consumed as the answer to the CLI's own overwrite
// prompt. Use ImportAll to import every detected variable.
func Import(ctx context.Context, c *client.Client, service string, opts ...ImportOption) error {
	if service == "" {
		return errors.New("secret import: service must not be empty (use ImportAll to import everything detected)")
	}
	var cfg importConfig
	for _, o := range opts {
		o(&cfg)
	}
	if err := checkNotStored(ctx, c, "", "service", service, cfg.overwrite, "secret import", "WithOverwriteExisting"); err != nil {
		return err
	}
	return runImport(ctx, c, []string{service}, false, opts...)
}

// ImportAll stores EVERY credential environment variable detected on this host
// into the secret store (`sbx secret import --all`).
//
// The blast radius is deliberately in the name: this is not scoped to one
// service and it writes to the same store as `sbx secret set -g`. Preview with
// WithDryRun first.
func ImportAll(ctx context.Context, c *client.Client, opts ...ImportOption) error {
	return runImport(ctx, c, nil, true, opts...)
}

func runImport(ctx context.Context, c *client.Client, services []string, all bool, opts ...ImportOption) error {
	var cfg importConfig
	for _, o := range opts {
		o(&cfg)
	}

	args := []string{"secret", "import"}
	args = append(args, services...)
	if all {
		args = append(args, "--all")
	}
	if cfg.dryRun {
		args = append(args, "--dry-run")
	}
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
