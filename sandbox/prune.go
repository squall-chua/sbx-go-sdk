package sandbox

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/squall-chua/sbx-go-sdk/client"
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
// Since sbx v0.42.0, removing a sandbox also deletes its sandbox-scoped
// secrets.
//
// A running sandbox is never a candidate, whatever the options.
//
// A real prune does not read the CLI's confirmation lines: their wording
// changed at v0.51.0 ("Sandbox 'X' removed" disappeared), and --json on a
// real prune is unverified at v0.51.0 (v0.42.1 refused it). Instead it reads
// the candidates from a dry run, runs the prune, and returns the candidates
// the daemon no longer lists.
//
// WithStoppedLongerThan's since= filter is no longer in the v0.51.0 help,
// which documents only until=TIMESTAMP, but it still filters: verified live.
func Prune(ctx context.Context, c *client.Client, opts ...PruneOption) ([]string, error) {
	cfg := &pruneConfig{}
	for _, o := range opts {
		o(cfg)
	}
	r, err := c.Runner()
	if err != nil {
		return nil, err
	}
	filter := []string{}
	if cfg.since != "" {
		filter = append(filter, "--filter", "since="+cfg.since)
	}

	// --json is used only on the dry run; see the doc comment.
	out, err := r.Capture(ctx, nil, append([]string{"prune", "--dry-run", "--json"}, filter...)...)
	if err != nil {
		return nil, err
	}
	candidates, err := pruneCandidates(out)
	if err != nil || cfg.dryRun || len(candidates) == 0 {
		return candidates, err
	}

	if _, err := r.Capture(ctx, nil, append([]string{"prune", "--force"}, filter...)...); err != nil {
		return nil, err
	}
	left, err := List(ctx, c)
	if err != nil {
		return nil, fmt.Errorf("prune: listing sandboxes after removal: %w", err)
	}
	present := make(map[string]bool, len(left))
	for _, sb := range left {
		present[sb.Name()] = true
	}
	removed := []string{}
	for _, n := range candidates {
		if !present[n] {
			removed = append(removed, n)
		}
	}
	return removed, nil
}

// pruneCandidates reads the would_remove list of `prune --dry-run --json`.
// Names under skipped_unknown_stop are skipped by the CLI, so they are not
// candidates.
func pruneCandidates(out string) ([]string, error) {
	var v struct {
		WouldRemove *[]struct {
			Name string `json:"name"`
		} `json:"would_remove"`
	}
	if err := json.Unmarshal([]byte(out), &v); err != nil {
		return nil, fmt.Errorf("prune --dry-run: %w: %w", client.ErrUnexpectedFormat, err)
	}
	if v.WouldRemove == nil {
		return nil, fmt.Errorf("prune --dry-run: %w: no \"would_remove\" key in output", client.ErrUnexpectedFormat)
	}
	names := make([]string, 0, len(*v.WouldRemove))
	for _, s := range *v.WouldRemove {
		names = append(names, s.Name)
	}
	return names, nil
}
