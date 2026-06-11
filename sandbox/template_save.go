package sandbox

import "context"

// SaveTemplate snapshots the sandbox as a reusable template image
// (`sbx template save NAME TAG`). Shell-out (no daemon REST builder).
//
// The daemon refuses to snapshot a running sandbox, so call Stop first;
// otherwise the CLI prompts to stop and fails on a non-interactive stdin.
func (s *Sandbox) SaveTemplate(ctx context.Context, tag string) error {
	r, err := s.cli.Runner()
	if err != nil {
		return err
	}
	_, err = r.Capture(ctx, nil, "template", "save", s.info.Name, tag)
	return err
}
