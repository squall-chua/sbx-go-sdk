package sandbox

import (
	"context"
	"path/filepath"
	"strings"

	"github.com/squall-chua/sbx-go-sdk/client"
	"github.com/squall-chua/sbx-go-sdk/internal/cli"
)

// Run provisions-if-missing and interactively attaches an agent (`sbx run AGENT
// …`), inheriting the caller's terminal stdio (override with WithStdio). It blocks
// until the agent exits and returns its exit code. A non-zero agent exit is
// (code, nil); only spawn/wait failures return a non-nil error. It does not return
// a handle (use Create/Get for that).
func Run(ctx context.Context, c *client.Client, opts ...Option) (int, error) {
	d := newDefinition(opts...)
	for i, ws := range d.workspaces {
		path, ro, _ := strings.Cut(ws, ":")
		abs, err := filepath.Abs(path)
		if err != nil {
			return -1, err
		}
		if ro == "ro" {
			d.workspaces[i] = abs + ":ro"
		} else {
			d.workspaces[i] = abs
		}
	}
	args, err := d.toRunArgs()
	if err != nil {
		return -1, err
	}
	r, err := c.Runner()
	if err != nil {
		return -1, err
	}
	return r.Inherit(ctx, d.stdio(), nil, args...)
}

// Run re-attaches an existing sandbox (`sbx run SANDBOX [-- AGENT_ARGS]`),
// inheriting terminal stdio. Returns the agent's exit code. Only WithAgentArgs and
// WithStdio are honored; create-time options (WithWorkspace, WithCPUs, …) are ignored.
func (s *Sandbox) Run(ctx context.Context, opts ...Option) (int, error) {
	d := newDefinition(opts...)
	args := []string{"run", s.info.Name}
	if len(d.agentArgs) > 0 {
		args = append(args, "--")
		args = append(args, d.agentArgs...)
	}
	r, err := s.cli.Runner()
	if err != nil {
		return -1, err
	}
	return r.Inherit(ctx, d.stdio(), nil, args...)
}

// stdio maps the Definition's stdio overrides into a cli.Stdio (zero values
// inherit os.Stdin/out/err inside cli.Inherit).
func (d *Definition) stdio() cli.Stdio {
	return cli.Stdio{In: d.stdin, Out: d.stdout, Err: d.stderr}
}
