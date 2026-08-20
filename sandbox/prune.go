package sandbox

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"github.com/squall-chua/sbx-go-sdk/client"
	"github.com/squall-chua/sbx-go-sdk/internal/coltable"
)

type pruneConfig struct {
	dryRun bool
	since  string
}

// PruneOption configures Prune.
type PruneOption func(*pruneConfig)

// WithDryRun reports what Prune would remove without removing anything
// (`--dry-run`).
func WithDryRun() PruneOption { return func(c *pruneConfig) { c.dryRun = true } }

// WithStoppedLongerThan narrows Prune to sandboxes stopped for longer than d
// (`--filter since=DURATION`, e.g. "24h"). The duration is passed through
// unvalidated; the CLI owns the grammar.
func WithStoppedLongerThan(d string) PruneOption {
	return func(c *pruneConfig) { c.since = d }
}

var (
	pruneHeader  = []string{"SANDBOX", "AGENT", "STOPPED", "WORKSPACE"}
	prunedLine   = regexp.MustCompile(`^Sandbox '(.+)' removed$`)
	pruneNothing = "No stopped sandboxes to prune."
)

// Prune removes every stopped sandbox (`sbx prune`, added in sbx v0.39.0) and
// returns the names it removed, or — with WithDryRun — the names it would have.
// Nothing to prune is not an error: the result is an empty slice and a nil
// error.
//
// Prune always passes --force, because without it the CLI refuses outright on a
// non-interactive stdin ("stdin is not a terminal; use --force to skip
// confirmation") rather than prompting. That flag carries a second meaning
// worth knowing: it also removes a stopped sandbox that is still in use, such
// as one holding an open SSH connection. Pair it with WithDryRun first if that
// matters.
//
// A running sandbox is never a candidate, whatever the options.
func Prune(ctx context.Context, c *client.Client, opts ...PruneOption) ([]string, error) {
	cfg := &pruneConfig{}
	for _, o := range opts {
		o(cfg)
	}
	args := []string{"prune"}
	if cfg.dryRun {
		args = append(args, "--dry-run")
	} else {
		args = append(args, "--force")
	}
	if cfg.since != "" {
		args = append(args, "--filter", "since="+cfg.since)
	}

	r, err := c.Runner()
	if err != nil {
		return nil, err
	}
	out, err := r.Capture(ctx, nil, args...)
	if err != nil {
		return nil, err
	}
	if strings.Contains(out, pruneNothing) {
		return []string{}, nil
	}
	if cfg.dryRun {
		return pruneCandidates(out)
	}
	return pruneRemoved(out), nil
}

// pruneCandidates reads the SANDBOX column of the table --dry-run prints under
// its "Would remove N stopped sandbox(es):" line.
func pruneCandidates(out string) ([]string, error) {
	rows, err := coltable.Parse(out, pruneHeader)
	if err != nil {
		return nil, fmt.Errorf("prune --dry-run: %w: %w", client.ErrUnexpectedFormat, err)
	}
	names := make([]string, 0, len(rows))
	for _, row := range rows {
		names = append(names, row["SANDBOX"])
	}
	return names, nil
}

// pruneRemoved reads the per-sandbox confirmation lines a real prune prints.
// A removal the CLI reported no line for is simply not returned — the caller
// asked what went, not what was attempted.
func pruneRemoved(out string) []string {
	names := []string{}
	for _, ln := range strings.Split(out, "\n") {
		if m := prunedLine.FindStringSubmatch(strings.TrimSpace(ln)); m != nil {
			names = append(names, m[1])
		}
	}
	return names
}
