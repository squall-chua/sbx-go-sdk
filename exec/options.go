// Package exec runs commands inside sandboxes: capture, interactive attach, and
// detached, over the daemon's hijacking /exec/attach endpoint.
package exec

import "io"

// processConfig accumulates exec options.
type processConfig struct {
	env        map[string]string
	workdir    string
	user       string
	privileged bool
	tty        bool
	autoStart  bool
	muxOut     io.Writer
	muxErr     io.Writer
}

// ProcessOption configures an exec invocation.
type ProcessOption func(*processConfig)

// WithEnv sets environment variables (cumulative).
func WithEnv(env map[string]string) ProcessOption {
	return func(c *processConfig) {
		if c.env == nil {
			c.env = map[string]string{}
		}
		for k, v := range env {
			c.env[k] = v
		}
	}
}

// WithWorkdir sets the working directory.
func WithWorkdir(d string) ProcessOption { return func(c *processConfig) { c.workdir = d } }

// WithUser sets the user (login name).
func WithUser(u string) ProcessOption { return func(c *processConfig) { c.user = u } }

// WithPrivileged grants extended privileges.
func WithPrivileged() ProcessOption { return func(c *processConfig) { c.privileged = true } }

// WithTTY allocates a pseudo-TTY (raw stream, no stdcopy framing).
func WithTTY() ProcessOption { return func(c *processConfig) { c.tty = true } }

// WithAutoStart is reserved to transparently start a stopped sandbox before exec.
// It is accepted but not yet wired in this release; start the sandbox explicitly
// via sandbox.Start until then.
func WithAutoStart() ProcessOption { return func(c *processConfig) { c.autoStart = true } }

// WithMultiplexed routes the demultiplexed stdout and stderr streams to the given
// writers instead of returning stdout via the reader. When set, the reader Exec
// returns is empty.
func WithMultiplexed(stdout, stderr io.Writer) ProcessOption {
	return func(c *processConfig) { c.muxOut = stdout; c.muxErr = stderr }
}

// execBody is the JSON sent to POST /sandbox/{name}/exec/attach.
type execBody struct {
	Cmd        []string          `json:"cmd"`
	Env        map[string]string `json:"env,omitempty"`
	Workdir    string            `json:"workdir,omitempty"`
	User       string            `json:"user,omitempty"`
	Privileged bool              `json:"privileged,omitempty"`
	TTY        bool              `json:"tty,omitempty"`
}

func buildBody(cmd []string, opts ...ProcessOption) execBody {
	var c processConfig
	for _, o := range opts {
		o(&c)
	}
	return execBody{
		Cmd: cmd, Env: c.env, Workdir: c.workdir,
		User: c.user, Privileged: c.privileged, TTY: c.tty,
	}
}
