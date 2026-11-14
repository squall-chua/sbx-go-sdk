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
// upstream). Each ref may be a local directory, a ZIP file, an OCI reference,
// or a git reference — `create --help` lists only "(directory, ZIP, or OCI)",
// but the CLI accepts a git reference too, reaching the same allowlist stage
// as `kit add`. Whether a given reference resolves still depends on the
// kit.allowedSources setting; a refusal happens before the sandbox is
// created, so nothing is left behind. May be called once with several refs
// or repeatedly.
//
// A local path is made absolute when the argument vector is built; the daemon
// records the kit list verbatim and resolves a relative path against its own
// working directory, which would record one that does not exist.
//
// Prefer WithKit over AddKit for anything beyond a trivial kit: AddKit
// refuses any kit declaring credentials, publishedPorts, volumes,
// commands.startup or commands.initFiles; the CLI's own remedy is to
// recreate the sandbox from scratch via `sbx rm` + `sbx create --kit` to use
// this kit.
//
// Refs are otherwise passed straight through without validation; the CLI is
// the authority on the grammar and rejects a malformed ref itself.
func WithKit(refs ...string) Option {
	return func(d *Definition) { d.kits = append(d.kits, refs...) }
}

// WithDenyNetwork adds per-sandbox network deny rules at creation time
// (`--deny-network`, added in sbx v0.38.0). Each host applies only to the new
// sandbox and can be listed or removed afterwards through the policy package.
//
// A local deny can only narrow egress, never widen it, so this stays allowed
// under centralized governance. May be called once with several hosts or
// repeatedly; hosts pass through unvalidated, as the CLI owns the grammar.
func WithDenyNetwork(hosts ...string) Option {
	return func(d *Definition) { d.denyNetwork = append(d.denyNetwork, hosts...) }
}

// WithoutSharedSkills opts the sandbox out of the shared agent skills store.
// It is WithSkills("off"); the SDK emits `--skills off`, because sbx v0.43.0
// deprecated the old `--no-share-skills` spelling. Applies at creation only.
//
// On Run, it makes a re-attach to an existing sandbox fail: the CLI refuses
// --skills, as it refused --no-share-skills, once the sandbox exists.
func WithoutSharedSkills() Option { return WithSkills("off") }

// WithSkills sets how the shared skills store reaches the agent's skills
// directory (`--skills`, added in sbx v0.43.0): "off", "readonly" (the store is
// linked in read-only, and the directory stays writable) or "readwrite" (the
// store is mounted over it, so writes are shared). Unset means the CLI default:
// readonly, or the skills.defaultMode setting.
//
// Applies at creation only. On Run, a re-attach to an existing sandbox fails
// with it set. The last of WithSkills and WithoutSharedSkills wins. The CLI
// rejects any other mode.
func WithSkills(mode string) Option { return func(d *Definition) { d.skills = mode } }

// WithMount binds host directories to paths a kit declares as host-mount@1
// (`--mount HOST:SANDBOX_PATH`, added in sbx v0.50.0). The daemon rejects a
// SANDBOX_PATH that no kit of the sandbox declares. The HOST half is made
// absolute when the argument vector is built.
//
// Applies at creation only. On Run, a re-attach to an existing sandbox fails
// with it set. May be called once with several specs or repeatedly.
func WithMount(specs ...string) Option {
	return func(d *Definition) { d.mounts = append(d.mounts, specs...) }
}

// WithPull sets the image pull policy at creation (`--pull`): "always",
// "missing" or "never". Unset means the CLI default: never for a saved local
// snapshot, missing for a v3 kit composition, and always otherwise. Use
// "never" with WithTemplate on a template saved by SaveTemplate.
func WithPull(policy string) Option { return func(d *Definition) { d.pull = policy } }

// WithRemoveOnExit removes the sandbox once the agent session ends (`run
// --rm`). It is honored only by the package-level Run, where it also removes
// an existing sandbox that Run re-attached to. Create and Sandbox.Run ignore
// it; a Sandbox handle would outlive its sandbox.
func WithRemoveOnExit() Option { return func(d *Definition) { d.rm = true } }

// WithWorkdir starts the agent session in dir (`run -w`), absolute or relative
// to the sandbox workspace. It applies to that session only, so both Run and
// Sandbox.Run honor it, on a re-attach too. Create ignores it.
func WithWorkdir(dir string) Option { return func(d *Definition) { d.workdir = dir } }

// WithStaticMCP fixes the sandbox's static MCP server set (`--static-mcp`,
// added in sbx v0.38.0). Each name must already be registered with the mcp
// package (or `sbx mcp add`).
//
// The set is chosen once, at creation: `sbx run` ignores it when re-attaching
// to an existing sandbox. To add a server to a running sandbox afterwards, use
// mcp.Load. May be called once with several names or repeatedly.
func WithStaticMCP(names ...string) Option {
	return func(d *Definition) { d.staticMCP = append(d.staticMCP, names...) }
}

// WithEnv sets environment variables inside the sandbox (`-e`, added in sbx
// v0.39.0). Each entry is either "KEY=VALUE", or a bare "KEY" to inherit the
// value from the calling process's own environment — the SDK passes its
// environment to the CLI child, so a bare key resolves against your process.
//
// This is the only route to the daemon's SandboxCreateRequest.Environment
// field, which had no CLI flag before v0.39.0.
//
// On Create the variables are baked into the sandbox. On Run they also apply
// to the agent session, so they take effect on a re-attach to an existing
// sandbox, not only when this call creates it.
//
// WithEnv wins over any file named by WithEnvFile. May be called once with
// several entries or repeatedly; entries pass through unvalidated, as the CLI
// owns the grammar.
func WithEnv(entries ...string) Option {
	return func(d *Definition) { d.env = append(d.env, entries...) }
}

// WithEnvFile reads environment variables from files inside the sandbox
// (`--env-file`, added in sbx v0.39.0). Paths are read by the CLI on the host.
//
// A later file wins over an earlier one, and anything set by WithEnv wins over
// every file — so the option order here does not matter, only the file order.
// May be called once with several paths or repeatedly.
func WithEnvFile(paths ...string) Option {
	return func(d *Definition) { d.envFile = append(d.envFile, paths...) }
}
