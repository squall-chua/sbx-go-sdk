package sandbox

import "context"

type saveConfig struct{ export string }

// SaveOption configures SaveTemplate.
type SaveOption func(*saveConfig)

// WithExport also writes the saved image to a tar file on the host
// (`-o`, added in sbx v0.39.0). The template is registered either way; this
// only adds the export, for moving the image to a machine that cannot reach
// this daemon. Load it there with template.Load.
func WithExport(path string) SaveOption { return func(c *saveConfig) { c.export = path } }

// SaveTemplate snapshots the sandbox as a reusable template image
// (`sbx template save NAME TAG`). Shell-out (no daemon REST builder).
//
// The daemon refuses to snapshot a running sandbox, so call Stop first;
// otherwise the CLI prompts to stop and fails on a non-interactive stdin.
func (s *Sandbox) SaveTemplate(ctx context.Context, tag string, opts ...SaveOption) error {
	var cfg saveConfig
	for _, o := range opts {
		o(&cfg)
	}
	r, err := s.cli.Runner()
	if err != nil {
		return err
	}
	args := []string{"template", "save", s.info.Name, tag}
	if cfg.export != "" {
		args = append(args, "--output", cfg.export)
	}
	_, err = r.Capture(ctx, nil, args...)
	return err
}
