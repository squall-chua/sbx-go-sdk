// Package exec runs commands inside sandboxes: capture, interactive attach, and
// detached, over the daemon's hijacking /exec/attach endpoint.
package exec

// processConfig accumulates exec options.
type processConfig struct {
	env        map[string]string
	workdir    string
	user       string
	privileged bool
	tty        bool
	autoStart  bool
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

// WithAutoStart transparently starts a stopped sandbox before exec.
func WithAutoStart() ProcessOption { return func(c *processConfig) { c.autoStart = true } }

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
