package sandbox

import "context"

// copyConfig accumulates cp options.
type copyConfig struct{ followSymlinks bool }

// CopyOption configures a copy.
type CopyOption func(*copyConfig)

// WithFollowSymlinks follows symlinks in the source path (`sbx cp -L`), in either
// direction (sandbox -> host support added in sbx v0.33.0).
func WithFollowSymlinks() CopyOption { return func(c *copyConfig) { c.followSymlinks = true } }

// CopyTo copies a host path into the sandbox (`sbx cp [-L] localPath name:sandboxPath`).
func (s *Sandbox) CopyTo(ctx context.Context, localPath, sandboxPath string, opts ...CopyOption) error {
	var cfg copyConfig
	for _, o := range opts {
		o(&cfg)
	}
	args := []string{"cp"}
	if cfg.followSymlinks {
		args = append(args, "-L")
	}
	args = append(args, localPath, s.info.Name+":"+sandboxPath)
	r, err := s.cli.Runner()
	if err != nil {
		return err
	}
	_, err = r.Capture(ctx, nil, args...)
	return err
}

// CopyFrom copies a sandbox path to the host (`sbx cp [-L] name:sandboxPath localPath`).
func (s *Sandbox) CopyFrom(ctx context.Context, sandboxPath, localPath string, opts ...CopyOption) error {
	var cfg copyConfig
	for _, o := range opts {
		o(&cfg)
	}
	args := []string{"cp"}
	if cfg.followSymlinks {
		args = append(args, "-L")
	}
	args = append(args, s.info.Name+":"+sandboxPath, localPath)
	r, err := s.cli.Runner()
	if err != nil {
		return err
	}
	_, err = r.Capture(ctx, nil, args...)
	return err
}
