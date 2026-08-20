// Package skillstore manages the shared agent skills store (EXPERIMENTAL
// upstream, added in sbx v0.37.0).
//
// The store is a daemon-managed directory of agent skill folders, seeded from
// the host and mounted into sandboxes that have not opted out. Imported skills
// survive sandbox deletion, but `sbx reset` clears the store.
//
// Host sources are scanned in this order, first match winning on a name
// conflict: ~/.agents/skills, ~/.claude/skills, ~/.copilot/skills,
// ~/.cursor/skills, ~/.factory/skills. Supported by the Claude, Codex, Copilot,
// Cursor and Droid agents.
package skillstore

import (
	"context"
	"fmt"
	"strings"

	"github.com/squall-chua/sbx-go-sdk/client"
)

type importConfig struct{ dryRun bool }

// ImportOption configures Import.
type ImportOption func(*importConfig)

// WithDryRun previews which skills would be imported without copying anything
// (`--dry-run`).
func WithDryRun() ImportOption { return func(c *importConfig) { c.dryRun = true } }

// Import copies skill folders from the host's agent skill directories into the
// shared store (`sbx skills import --force`).
//
// --force is always passed: the CLI otherwise prompts before replacing an
// existing skill, which would hang a non-interactive caller. Replacement is
// recoverable — the CLI backs up the existing folder before installing the new
// copy. Symlinks and loose top-level files are skipped by the CLI.
func Import(ctx context.Context, c *client.Client, opts ...ImportOption) error {
	var cfg importConfig
	for _, o := range opts {
		o(&cfg)
	}

	args := []string{"skills", "import", "--force"}
	if cfg.dryRun {
		args = append(args, "--dry-run")
	}

	r, err := c.Runner()
	if err != nil {
		return err
	}
	_, err = r.Capture(ctx, nil, args...)
	return err
}

// Store is what `sbx skills ls` reports: where the shared store lives on the
// host, and the skill folders currently in it.
type Store struct {
	// Path is the store directory the daemon manages and mounts into
	// sandboxes.
	Path string
	// Skills are the folder names in it, in the order the CLI lists them.
	Skills []string
}

const storePathPrefix = "Skills store: "

// List reports the shared store's path and contents (`sbx skills ls`, added in
// sbx v0.39.0).
//
// An empty store is not an error: Skills comes back empty and Path is still
// reported, since the directory exists whether or not anything has been
// imported into it.
func List(ctx context.Context, c *client.Client) (Store, error) {
	r, err := c.Runner()
	if err != nil {
		return Store{}, err
	}
	out, err := r.Capture(ctx, nil, "skills", "ls")
	if err != nil {
		return Store{}, err
	}

	var st Store
	for _, ln := range strings.Split(out, "\n") {
		ln = strings.TrimSpace(ln)
		switch {
		case ln == "":
		case strings.HasPrefix(ln, storePathPrefix):
			st.Path = strings.TrimSpace(strings.TrimPrefix(ln, storePathPrefix))
		default:
			st.Skills = append(st.Skills, ln)
		}
	}
	if st.Path == "" {
		return Store{}, fmt.Errorf("skills ls: %w: no %q line in output",
			client.ErrUnexpectedFormat, strings.TrimSpace(storePathPrefix))
	}
	return st, nil
}
