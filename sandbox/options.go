package sandbox

import "io"

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

// WithAgentArgs passes arguments to the agent process (placed after "--" in
// `sbx run`). Repeatable; cumulative.
func WithAgentArgs(args ...string) Option {
	return func(d *Definition) { d.agentArgs = append(d.agentArgs, args...) }
}

// WithStdio overrides the terminal stdio used by Run (zero values inherit the
// caller's os.Stdin/out/err).
func WithStdio(in io.Reader, out, err io.Writer) Option {
	return func(d *Definition) { d.stdin = in; d.stdout = out; d.stderr = err }
}

// WithPublish publishes sandbox ports at creation time (`-p`, added in sbx
// v0.37.0). Each spec is [[HOST_IP:]HOST_PORT:]SANDBOX_PORT[/PROTOCOL]; omit
// HOST_PORT for an ephemeral host port. May be called once with several specs
// or repeatedly.
//
// Specs are passed straight through without validation; the CLI is the
// authority on the grammar and rejects a malformed spec itself.
//
// Publishing at creation is atomic with create; Sandbox.PublishPort adds ports
// to an existing sandbox afterwards.
func WithPublish(specs ...string) Option {
	return func(d *Definition) { d.publish = append(d.publish, specs...) }
}

// WithKit attaches kit artifacts at creation time (`--kit`, EXPERIMENTAL
// upstream). Each ref may be a local directory, a ZIP file, or an OCI
// reference. May be called once with several refs or repeatedly.
//
// A local path is made absolute when the argument vector is built; the daemon
// records the kit list verbatim and resolves a relative path against its own
// working directory, which would record one that does not exist.
//
// Prefer WithKit over AddKit for anything beyond a trivial kit: AddKit
// refuses any kit declaring credentials, publishedPorts, volumes,
// commands.startup or commands.initFiles; the CLI's own remedy is to
// recreate the sandbox with create --kit.
//
// Refs are otherwise passed straight through without validation; the CLI is
// the authority on the grammar and rejects a malformed ref itself.
func WithKit(refs ...string) Option {
	return func(d *Definition) { d.kits = append(d.kits, refs...) }
}
