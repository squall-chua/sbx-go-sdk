---
name: sbx-go-sdk
description: Guide for using the sbx-go-sdk Go library to automate Docker Sandboxes (sbx) — isolated micro-VMs for AI coding agents. Use when writing Go code that imports github.com/squall-chua/sbx-go-sdk, or when the task involves creating/running sandboxes, exec-ing commands inside them, attaching agents, copying files, publishing ports, managing templates, network policy, secrets, or the sandboxd daemon from Go.
---

# Using sbx-go-sdk

A Go SDK that drives the local `sandboxd` daemon (REST over a unix socket) and shells out to
the `sbx` binary. Full guide: [README](../../README.md). Runnable demos:
[examples/](../../examples/).

## Pick the right entry point first

This is the #1 source of mistakes. `sbx` vocabulary is precise:

- **Create** (`sandbox.Create`) — provision a sandbox, return a handle, **no agent attached**.
  May leave the VM stopped.
- **Run** (`sandbox.Run` / `sb.Run`) — the **interactive agent session**: attaches the agent
  to a terminal and blocks. NOT "create + start". Use for a human-facing agent, not automation.
- **Exec** (`exec.Exec`) — run an **arbitrary command** (not the agent) and get its output.
  This is the workhorse for automation.
- **Start/Stop** (`sb.Start`/`sb.Stop`) — bring the micro-VM up/down without removing it.

## Quick start (automation flow)

```go
ctx := context.Background()
c, err := client.New(ctx, client.WithAutoStart())            // start sandboxd if down
sb, err := sandbox.Create(ctx, c,
    sandbox.WithAgent("shell"),                              // required
    sandbox.WithWorkspace("."))                              // required; ":ro" for read-only
defer sb.Remove(ctx)                                         // sandboxes are disposable
code, out, err := exec.Exec(ctx, sb, []string{"go", "test", "./..."},
    exec.WithAutoStart())                                    // start the VM if Create left it stopped
body, _ := io.ReadAll(out)                                   // demuxed stdout (stderr discarded)
```

## API map

| Need | Call |
| --- | --- |
| Connect / start daemon | `client.New(ctx, client.WithAutoStart())` |
| Provision a sandbox | `sandbox.Create(ctx, c, sandbox.WithAgent(...), sandbox.WithWorkspace(...))` |
| List / get | `sandbox.List(ctx, c)`, `sandbox.Get(ctx, c, name)`, `sb.Inspect(ctx)` |
| Lifecycle | `sb.Start/Stop/Remove(ctx)` |
| Run command (capture) | `exec.Exec(ctx, sb, cmd, exec.WithAutoStart())` |
| Stream stdout/stderr live | `exec.Exec(..., exec.WithMultiplexed(stdout, stderr))` |
| Interactive shell / TTY | `exec.ExecInteractive(ctx, sb, cmd, exec.WithTTY())` → Stdin/Stdout/Resize/Wait |
| Background command | `exec.ExecDetached(...)` → poll `exec.InspectExec(ctx, sb, id)` |
| Resource stats (CPU/mem/disk) | `exec.Stats(ctx, sb)` → `exec.Usage{ Cores, MemTotalKB, MemAvailableKB, MemUsedKB, CPUPercent, UptimeSeconds, DiskTotalGB, DiskUsedGB }` |
| Interactive agent | `sandbox.Run(ctx, c, ...)` / `sb.Run(ctx, sandbox.WithAgentArgs(...))` |
| Copy files | `sb.CopyTo(ctx, local, sandboxPath)`, `sb.CopyFrom(ctx, sandboxPath, local)` |
| Ports | `sb.PublishPort(ctx, sandbox.Port{...})`, `sb.Ports(ctx)`, `sb.UnpublishPort(ctx, spec)` |
| Templates | `sb.SaveTemplate(ctx, tag)`, `template.List/Inspect/Remove/Load` |
| Network policy | `policy.SetDefault/Allow/Deny/RemoveRule/Reset`, `policy.Log`, `policy.Check` (`*policy.Authorization`), `policy.InspectRaw`, `policy.ProfileNames` (`[]string`, empty without a governed org) |
| Secrets | `secret.SetCustom/List/Remove`, `secret.SetToken/SetRegistry` (stdin, no argv exposure), `secret.Import/ImportAll` |
| Settings | `settings.Get(ctx, c, key)`, `settings.List(ctx, c)`, `settings.ListAll(ctx, c)` (feature flags too, v0.38.0), `settings.Set(ctx, c, key, value)`, `settings.Unset(ctx, c, key)` |
| SSH endpoint | `ssh.Enable/Disable/Enabled`, `ssh.Setup(ctx, c, ssh.WithAlias(...))`, `ssh.TargetFor(name)` |
| Skills | `skillstore.Import(ctx, c, skillstore.WithDryRun())` — shared agent skills store (v0.37.0) |
| Kit artifacts | `kit.Inspect/Validate/Pack/Push/Pull(ctx, c, ...)` (v0.34.0) — attach with `sandbox.WithKit` (v0.34.0) or `sb.AddKit`/`sb.Kits` (v0.35.0) |
| MCP servers | `mcp.AddRemote/AddLocal/List/Inspect/Remove/Load(ctx, c, ...)` (v0.38.0) — fix a sandbox's set with `sandbox.WithStaticMCP`, or `mcp.Load` into a running one; `mcp.AuthStatus/AuthRemove` for hosted OAuth state; `c.MCPGatewayMode(ctx)` reports local vs hosted gateway |
| Sandbox summary | `sb.Summary(ctx)` → auth mode, injected secrets, session count, MCP-gateway state (v0.38.0) — none of these are on `sb.Inspect`'s REST record |
| Install check / sign-in | `c.Diagnose(ctx)` (`*client.Diagnosis`, `.OK()`), `c.Login(ctx, user, token)` (stdin, no argv), `c.Logout(ctx)` (stops every running sandbox), `c.RestartDaemon(ctx)` |
| Detect host config | `c.DetectSetup(ctx)` → `*client.SetupReport`, `.Section("SKILLS")` — read-only, never runs the wizard |
| OAuth handshakes | `secret.SetOAuth(ctx, c, "openai", onURL)`, `mcp.Authorize(ctx, c, name, onURL)` — both hand you the consent URL and block; always pass a cancellable ctx |

Exec options: `WithEnv`, `WithWorkdir`, `WithUser`, `WithPrivileged`, `WithTTY`, `WithAutoStart`,
`WithMultiplexed`. Create options: `WithAgent`, `WithWorkspace`, `WithName`, `WithCPUs`,
`WithMemory`, `WithTemplate`, `WithProfile`, `WithClone`, `WithAgentArgs`, `WithStdio`,
`WithPublish` (`-p`, v0.37.0), `WithKit` (`--kit`, v0.34.0), `WithDenyNetwork`, `WithStaticMCP`
(both v0.38.0), `WithoutSharedSkills`. Remove option: `WithForce` (removes an active session).

## Gotchas (verified against sandboxd v0.38.0)

- **Exec needs a running VM.** Pass `exec.WithAutoStart()`, or you get
  `client.ErrSandboxNotRunning`. `Create` does not guarantee the VM is up.
- **No daemon metrics endpoint.** `exec.Stats` (like the `sbx` TUI) just execs a `/proc` + `df`
  probe — so it needs a running VM and coreutils, and blocks ~200ms to sample CPU. It returns the
  same metrics the TUI shows (CPU/mem/disk/uptime). `CPUPercent` is the mean across cores, clamped
  0–100; `UptimeSeconds`/`Disk*` are best-effort (0 if df/uptime unavailable, e.g. busybox) and
  never fail the core CPU/mem snapshot.
- **`SaveTemplate` requires a stopped sandbox** — call `sb.Stop(ctx)` first, or it fails on a
  non-interactive stop prompt.
- **`secret.List` → `*Secrets`** still parses the CLI table (no `--json` upstream); a format
  change returns `client.ErrUnexpectedFormat`. Use `secret.ListRaw` for raw text. `policy.Profiles`
  keeps its text signature (deprecated); `policy.ProfileNames` is the typed REST call.
- **The OAuth calls block on a human.** `secret.SetOAuth` and `mcp.Authorize` print nothing —
  they invoke your `onURL` callback with the consent URL, then wait on a loopback callback until
  someone approves. Always give them a timeout/cancellable ctx.
- **Secret scope changed spelling in v0.38.0.** The SDK's scope argument is unchanged (`""` =
  global, otherwise a sandbox name), but a *registry* credential's default scope is now the new
  `secret.HostOnlyScope` — host-side pulls only, injected into no sandbox. `SetRegistry(…, "", …)`
  still means "every sandbox"; pass `secret.WithHostOnly()` for the new one.
- **`mcp.List`/`mcp.Inspect` parse CLI output** (no `--json` upstream); `mcp.AuthStatus`/`AuthRemove`
  do get `--format json`. `sbx mcp auth <name>` (interactive OAuth) is not wrapped — register with
  `mcp.WithSkipAuth()`, authorize out of band, confirm with `mcp.AuthStatus`. `mcp.Remove` on an
  unregistered name exits 0, so it cannot report whether anything was removed.
- **`sb.Inspect` vs `sb.Summary`.** `Inspect` is the daemon's REST record. `Summary`
  (`sbx inspect --json`, shell-out) is the only source of `AuthMode`, `Secrets`, `Sessions` and
  `MCPGateway`. Check `Summary.Sessions` before reaching for `Remove(WithForce())`.
- **`secret.SetCustom` is experimental** and exposes the value in host process listings — for
  headless agent credentials prefer `exec.WithEnv`. `secret.SetToken`/`SetRegistry` do not have
  this problem: both write the secret to the child process's stdin instead of an argument, so it
  never appears in the process list.
- **`SetToken`/`SetRegistry`/`Import` error on an existing entry** instead of silently no-op'ing —
  without `--force`, sbx's own overwrite prompt reads EOF from non-interactive stdin and exits 0
  having stored nothing. Pass `WithOverwrite()`/`WithOverwriteExisting()` to replace. `SetCustom`
  has no such check.
- **`settings`/`ssh` mutations are fire-and-forget shell-outs** — `settings.Set`,
  `ssh.Enable/Disable/Setup` write host state (`settings.json`, `~/.ssh/config`) and
  return before the daemon's ~5s hot-reload. `settings.Get/List` and `ssh.Enabled`
  read via `--json`. `ssh.Enable` sets only `feature.ssh` (also needs
  `platform.allowExperimentalFeatures`, default true). SSH connects by hostname
  (`ssh <name>.sbx`); v0.35.0 dropped the `ssh.port` loopback model.
- **`CopyFrom` auto-starts a stopped sandbox** — matching `sbx cp`'s own behaviour, unlike `exec`
  which needs an explicit `WithAutoStart()`. `CopyTo` still shells out to the `sbx` binary.
- **`policy.List` is REST and always requests `type=all`** — `GET /policy/network/rules`;
  omitting `type=all` would silently drop filesystem rules with no error.
- **A non-zero agent/command exit is `(code, nil)`** — only spawn/transport failures are errors.
  Check the returned code.
- **Don't call `client.Reset`** unless intended: it wipes all sandboxes and daemon state.
- **`WithStrictVersion()` and `CheckVersion` are deprecated** (see ADR 0004) — `api_version` bumps
  on every `sbx` release, so a strict compare fires on every upgrade, and upstream removed
  `POST /version` in v0.37.0, so `CheckVersion` can no longer succeed. For a runtime check, compare
  `DaemonHealth.Version`/`APIVersion` to `client.ClientVersion`/`client.TestedAPIVersion` yourself.
  SDK is pinned to sbx v0.37.0 / api 0.24.0.

## Errors

Branch with `errors.Is` on `client.ErrSandboxNotFound`, `ErrSandboxExists`,
`ErrSandboxNotRunning`, `ErrExecNotFound`, `ErrIncompatibleVersion`, `ErrDaemonNotRunning`,
`ErrBinaryNotFound`. Use `errors.As` for `*client.APIError` (REST: `.Op/.Status/.Message`) or
`client.CLIError` (shell-out).
