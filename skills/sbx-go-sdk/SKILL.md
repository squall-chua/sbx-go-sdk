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
| Connect / start daemon | `client.New(ctx, client.WithAutoStart(), client.WithStrictVersion())` |
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
| Ports | `sb.PublishPort(ctx, sandbox.Port{...})`, `sb.Ports(ctx)` |
| Templates | `sb.SaveTemplate(ctx, tag)`, `template.List/Inspect/Remove/Load` |
| Network policy | `policy.SetDefault/Allow/Deny/RemoveRule/Reset`, `policy.Log` |
| Secrets | `secret.SetCustom/List/Remove` |

Exec options: `WithEnv`, `WithWorkdir`, `WithUser`, `WithPrivileged`, `WithTTY`, `WithAutoStart`,
`WithMultiplexed`. Create options: `WithAgent`, `WithWorkspace`, `WithName`, `WithCPUs`,
`WithMemory`, `WithTemplate`, `WithProfile`, `WithClone`, `WithAgentArgs`, `WithStdio`.

## Gotchas (verified against sandboxd v0.32.0)

- **Exec needs a running VM.** Pass `exec.WithAutoStart()`, or you get
  `client.ErrSandboxNotRunning`. `Create` does not guarantee the VM is up.
- **No daemon metrics endpoint.** `exec.Stats` (like the `sbx` TUI) just execs a `/proc` + `df`
  probe — so it needs a running VM and coreutils, and blocks ~200ms to sample CPU. It returns the
  same metrics the TUI shows (CPU/mem/disk/uptime). `CPUPercent` is the mean across cores, clamped
  0–100; `UptimeSeconds`/`Disk*` are best-effort (0 if df/uptime unavailable, e.g. busybox) and
  never fail the core CPU/mem snapshot.
- **`SaveTemplate` requires a stopped sandbox** — call `sb.Stop(ctx)` first, or it fails on a
  non-interactive stop prompt.
- **`policy.List` → `[]PolicyRule`, `secret.List` → `*Secrets`** by parsing the CLI table (no
  `--json` upstream); drift returns `client.ErrUnexpectedFormat`. Use `policy.ListRaw` /
  `secret.ListRaw` for raw text; `policy.Profiles` stays raw text.
- **`secret.SetCustom` is experimental** and exposes the value in host process listings — for
  headless agent credentials prefer `exec.WithEnv`.
- **`cp` and `UnpublishPort` shell out** (no daemon REST path) — they need the `sbx` binary.
- **A non-zero agent/command exit is `(code, nil)`** — only spawn/transport failures are errors.
  Check the returned code.
- **Don't call `client.Reset`** unless intended: it wipes all sandboxes and daemon state.
- **`WithStrictVersion()` is unreliable on non-release daemons** — `POST /version` can report
  `"incompatible"` even for a version-matched daemon (`DaemonHealth.Release == false`). For a
  dependable check, compare `DaemonHealth.Version`/`APIVersion` to `client.ClientVersion` /
  `client.TestedAPIVersion`. SDK is pinned to sbx v0.32.0 / api 0.10.0.

## Errors

Branch with `errors.Is` on `client.ErrSandboxNotFound`, `ErrSandboxExists`,
`ErrSandboxNotRunning`, `ErrExecNotFound`, `ErrIncompatibleVersion`, `ErrDaemonNotRunning`,
`ErrBinaryNotFound`. Use `errors.As` for `*client.APIError` (REST: `.Op/.Status/.Message`) or
`client.CLIError` (shell-out).
