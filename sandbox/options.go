package sandbox

// Option configures a sandbox Definition.
type Option func(*Definition)

// WithAgent sets the agent (claude, codex, copilot, cursor, docker-agent, droid,
// gemini, kiro, opencode, shell). Required.
func WithAgent(a string) Option { return func(d *Definition) { d.agent = a } }

// WithWorkspace adds a host workspace (repeatable). Append ":ro" for read-only.
func WithWorkspace(path string) Option {
	return func(d *Definition) { d.workspaces = append(d.workspaces, path) }
}

// WithName sets an explicit sandbox name (else the SDK generates one).
func WithName(n string) Option { return func(d *Definition) { d.name = n } }

// WithCPUs sets the CPU allocation (0 = auto).
func WithCPUs(n int) Option { return func(d *Definition) { d.cpus = n } }

// WithMemory sets the memory limit (e.g. "8g").
func WithMemory(m string) Option { return func(d *Definition) { d.memory = m } }

// WithProfile assigns a governance profile.
func WithProfile(p string) Option { return func(d *Definition) { d.profile = p } }

// WithTemplate sets the base container image.
func WithTemplate(t string) Option { return func(d *Definition) { d.template = t } }

// WithClone runs the agent on an in-container git clone instead of a bind mount.
func WithClone() Option { return func(d *Definition) { d.clone = true } }
