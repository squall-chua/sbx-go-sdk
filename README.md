# sbx-go-sdk

[![Go 1.25](https://img.shields.io/badge/Go-1.25-00ADD8?logo=go)](https://go.dev/)
[![sbx v0.37.0](https://img.shields.io/badge/sbx-v0.37.0-2496ED?logo=docker)](https://docs.docker.com/)
[![Go Reference](https://pkg.go.dev/badge/github.com/squall-chua/sbx-go-sdk.svg)](https://pkg.go.dev/github.com/squall-chua/sbx-go-sdk)

A Go SDK for automating **Docker Sandboxes** (`sbx`) — isolated micro-VM environments
provisioned for AI coding agents. Drive sandbox creation, command execution, interactive
agent sessions, file transfer, ports, templates, network policy, and the daemon itself,
all from Go.

```go
ctx := context.Background()

c, _ := client.New(ctx, client.WithAutoStart())                 // talk to (or start) sandboxd
sb, _ := sandbox.Create(ctx, c,                                 // provision a sandbox
    sandbox.WithAgent("claude"),
    sandbox.WithWorkspace("."),
)
defer sb.Remove(ctx)

code, out, _ := exec.Exec(ctx, sb,                              // run a command inside it
    []string{"claude", "-p", "summarise the repo"},
    exec.WithAutoStart(),
)
body, _ := io.ReadAll(out)
fmt.Printf("exit %d:\n%s", code, body)
```

---

## Contents

- [What is `sbx`?](#what-is-sbx)
- [Domain model](#domain-model) — the words this SDK uses, and why
- [How it works](#how-it-works) — hybrid REST + CLI transport
- [Set up `sbx`](#set-up-sbx-prerequisite) — install the CLI & daemon first
- [Install the SDK](#install-the-sdk)
- [Quick start](#quick-start)
- [Package map](#package-map)
- [Usage guide](#usage-guide)
  - [1. Connect to the daemon](#1-connect-to-the-daemon)
  - [2. Create a sandbox](#2-create-a-sandbox)
  - [3. List & inspect](#3-list--inspect)
  - [4. Lifecycle: start / stop / remove](#4-lifecycle-start--stop--remove)
  - [5. Exec commands](#5-exec-commands)
  - [6. Run an agent interactively](#6-run-an-agent-interactively)
  - [7. Copy files](#7-copy-files)
  - [8. Publish ports](#8-publish-ports)
  - [9. Templates](#9-templates)
  - [10. Network policy](#10-network-policy)
  - [11. Secrets](#11-secrets)
  - [12. Settings & SSH](#12-settings--ssh)
  - [13. Skills](#13-skills)
- [Error handling](#error-handling)
- [Runnable examples](#runnable-examples)
- [Agent skill](#agent-skill)
- [Version alignment](#version-alignment) — how the SDK tracks `sbx` releases
- [Known deviations & limitations](#known-deviations--limitations)

---

## What is `sbx`?

`sbx` (Docker Sandboxes) provisions disposable, isolated micro-VMs for AI coding agents
(`claude`, `codex`, `copilot`, `cursor`, `gemini`, `opencode`, `shell`, …). Each sandbox
mounts one or more host directories as workspaces, runs behind a network-egress proxy, and
is managed by a local background daemon, `sandboxd`.

`sbx` ships as a single binary that is **both the CLI and the daemon** (the dockerd model).
This SDK talks to that daemon over its local unix socket and shells out to the binary where
the daemon has no REST path — giving you the full `sbx` surface from Go without parsing CLI
output.

## Domain model

The SDK deliberately mirrors `sbx`'s own vocabulary. Getting these distinctions right is the
key to using the API correctly (full glossary in [CONTEXT.md](CONTEXT.md)):

| Term | Meaning | Don't confuse with |
| --- | --- | --- |
| **Sandbox** | The isolated micro-VM. The central resource. | container, VM, box |
| **Agent** | The AI tool running *inside* a sandbox. A sandbox is created *for* an agent. | assistant, bot |
| **Workspace** | A host directory mounted into the sandbox (append `:ro` for read-only). | mount, volume |
| **Create** | Provision a sandbox **without** attaching. (`sbx create`) | run, new |
| **Run** | Launch and **interactively attach** to the agent (creating the sandbox if needed). (`sbx run`) | start, exec |
| **Exec** | Run an **arbitrary command** inside the sandbox — *not* the agent. (`sbx exec`) | run, shell |
| **Start / Stop** | Bring the micro-VM up/down without removing it. | pause, resume |
| **Template** | A saved sandbox image new sandboxes can be created from. | image, snapshot |
| **Daemon** (`sandboxd`) | The local process the SDK talks to over a unix socket. | server, engine |

> ⚠️ **Run ≠ "create + start".** In this SDK, `Run` means *the interactive agent session*.
> To bring a stopped VM up, use `Start`. To run a one-off command, use `exec.Exec`.

## How it works

The transport is **hybrid** (see [docs/adr/0001](docs/adr/0001-hybrid-cli-shellout-plus-rest.md)):

- **REST over a unix socket** for everything the daemon exposes — list, inspect, start, stop,
  remove, exec/attach (via a hijacked Docker stdcopy stream), ports (publish/unpublish), copying a
  file *from* a sandbox, policy list/check, templates, network log.
- **Shell-out to the `sbx` binary** for orchestration-heavy operations with no REST client
  path — `Create`, agent `Run`, `template save`, copying a file *to* a sandbox, skills import, and
  `policy`/`secret` mutations.

The socket path is resolved with this precedence: `client.WithSocketPath(...)` >
`$DOCKER_SANDBOXES_API` > the XDG default
(`~/.local/state/sandboxes/sandboxes/sandboxd/sandboxd.sock`). You normally don't set it.

## Set up `sbx` (prerequisite)

This SDK *automates an existing Docker Sandboxes install* — it does **not** bundle `sbx`. Install
the CLI, sign in, and bring the daemon up **before** running any SDK code.

> 📚 Official docs:
> [Get started with Docker Sandboxes](https://docs.docker.com/ai/sandboxes/get-started/) ·
> [`sbx` CLI reference](https://docs.docker.com/reference/cli/sbx/) ·
> [Architecture](https://docs.docker.com/ai/sandboxes/architecture/) ·
> [Releases](https://github.com/docker/sbx-releases)

**1. Install the CLI**

| Platform | Command(s) |
| --- | --- |
| macOS (Apple silicon, macOS 14+) | `brew install docker/tap/sbx` |
| Windows 11 (x86_64) | `winget install -h Docker.sbx` |
| Linux (Ubuntu 24.04+, x86_64) | `curl -fsSL https://get.docker.com \| sudo REPO_ONLY=1 sh` then `sudo apt-get install docker-sbx` |

**2. Enable hardware virtualization** (sandboxes are micro-VMs)

```bash
# Linux: verify KVM, then add yourself to the kvm group (log out/in afterwards)
lsmod | grep kvm
sudo usermod -aG kvm "$USER"
```

```powershell
# Windows: enable the Hypervisor Platform once, from an elevated PowerShell
Enable-WindowsOptionalFeature -Online -FeatureName HypervisorPlatform -All
```

**3. Sign in** — opens a browser for Docker OAuth and prompts for a default network policy
(Balanced is recommended). You also need an **API key for your agent's model provider** (e.g.
Anthropic for `claude`) to actually run an agent.

```bash
sbx login
```

**4. Verify** the CLI and daemon are healthy:

```bash
sbx version    # CLI + daemon version (target this SDK against v0.37.0)
sbx diagnose   # diagnose install / daemon issues
sbx ls         # list sandboxes (an empty list means the daemon is reachable)
```

The **`sandboxd` daemon starts automatically** once you're authenticated. The SDK's
`client.WithAutoStart()` also brings it up if it's down (it shells out to `sbx daemon start
--detach`).

## Install the SDK

```bash
go get github.com/squall-chua/sbx-go-sdk
```

Requires:

- **Go 1.25+**.
- The **`sbx` binary** on `PATH` (or set `client.WithBinaryPath`) — see
  [Set up `sbx`](#set-up-sbx-prerequisite) above. The SDK shells out to it for create/run/cp/etc.
- A reachable **`sandboxd`** — pass `client.WithAutoStart()` and the SDK will start it for you.

This SDK is built and live-verified against **`sbx` / `sandboxd` v0.37.0** (daemon API `0.24.0`);
see [Version alignment](#version-alignment) for how it tracks newer `sbx` releases.

## Quick start

```go
package main

import (
	"context"
	"fmt"
	"io"
	"log"

	"github.com/squall-chua/sbx-go-sdk/client"
	"github.com/squall-chua/sbx-go-sdk/exec"
	"github.com/squall-chua/sbx-go-sdk/sandbox"
)

func main() {
	ctx := context.Background()

	// 1. Connect; start the daemon if it isn't already running.
	c, err := client.New(ctx, client.WithAutoStart())
	if err != nil {
		log.Fatal(err)
	}

	// 2. Provision a sandbox for the "shell" agent over the current directory.
	sb, err := sandbox.Create(ctx, c,
		sandbox.WithAgent("shell"),
		sandbox.WithWorkspace("."),
	)
	if err != nil {
		log.Fatal(err)
	}
	defer sb.Remove(ctx) // disposable: clean it up when done

	// 3. Run a command. WithAutoStart brings the VM up if Create left it stopped.
	code, out, err := exec.Exec(ctx, sb,
		[]string{"sh", "-c", "echo hello from $(hostname)"},
		exec.WithAutoStart(),
	)
	if err != nil {
		log.Fatal(err)
	}
	body, _ := io.ReadAll(out)
	fmt.Printf("[exit %d] %s", code, body)
}
```

## Package map

| Package | Import | What it covers |
| --- | --- | --- |
| `client` | `…/client` | Daemon connection, lifecycle (start/stop/status/health/version), options. |
| `sandbox` | `…/sandbox` | Create, list, inspect, start/stop/remove, interactive `Run`, cp, ports, save-template. |
| `exec` | `…/exec` | Run commands: capture, streaming, interactive attach (TTY + resize), detached, resource stats. |
| `template` | `…/template` | List / inspect / remove / load template images. |
| `policy` | `…/policy` | Network egress policy: defaults, allow/deny rules, profiles, proxy log. |
| `secret` | `…/secret` | Stored proxy-injected secrets and credential import (experimental upstream). |
| `settings` | `…/settings` | Read/write persistent sandboxd settings via `sbx settings … --json`. |
| `ssh` | `…/ssh` | Experimental SSH endpoint: enable/disable, `setup`, connection targets — composed over `settings`. |
| `skillstore` | `…/skillstore` | Import host agent-skill folders into sandboxd's shared skills store (experimental upstream). |

Two return types are re-exported aliases so external callers never need `internal/*`:
`sandbox.Info` (daemon sandbox description) and `client.Runner` (the `sbx`-binary runner).

---

## Usage guide

### 1. Connect to the daemon

```go
// Default: resolve the socket, don't start anything.
c, err := client.New(ctx)

// Common production setup: ensure the daemon is up.
c, err := client.New(ctx,
	client.WithAutoStart(),                 // start sandboxd if down, wait up to ~30s
	client.WithHTTPTimeout(30*time.Second), // per-request REST timeout
)

// Overrides (rarely needed):
client.WithSocketPath("/custom/sandboxd.sock")
client.WithBinaryPath("/opt/sbx/bin/sbx")

// Deprecated — see "Version alignment" / ADR 0004:
client.WithStrictVersion() // hard-fails on any api_version difference, which fires on every sbx release
```

Daemon introspection and control:

```go
h, _ := c.Health(ctx)                  // liveness: status, version
st, _ := c.DaemonStatus(ctx)           // Running bool + Socket (down daemon => Running:false, nil err)
res, _ := c.CheckVersion(ctx)          // deprecated: POST /version was removed in sbx v0.37.0, always errors now
info, _ := c.Info(ctx)                 // socket paths
_ = c.EnsureRunning(ctx)               // start + wait-for-healthy if needed
_ = c.SetLogLevel(ctx, "proxy", "debug")
_ = c.StopDaemon(ctx)                  // shut sandboxd down
```

> ⚠️ Avoid `c.Reset(ctx)` unless you mean it: it wipes **all** sandboxes and daemon state.

### 2. Create a sandbox

`Create` provisions a sandbox and returns a hydrated handle. It does **not** attach an agent
and may leave the VM stopped — pair it with `exec.WithAutoStart()` or `sb.Start(ctx)` before
running commands.

```go
sb, err := sandbox.Create(ctx, c,
	sandbox.WithAgent("claude"),            // required: the agent this sandbox is for
	sandbox.WithWorkspace("."),             // required: at least one; repeatable
	sandbox.WithWorkspace("../shared:ro"),  // ":ro" => read-only mount
	sandbox.WithName("review-bot"),         // optional; otherwise auto-named "<agent>-<dir>"
	sandbox.WithCPUs(4),
	sandbox.WithMemory("8g"),
	sandbox.WithTemplate("myimg:v1"),       // base on a saved template
	sandbox.WithProfile("balanced"),        // governance profile
	sandbox.WithClone(),                    // run on an in-container git clone, not a bind mount
	sandbox.WithPublish("8080"),            // publish ports atomically with create ("-p", sbx v0.37.0+)
)
```

The SDK owns the sandbox's identity: when `WithName` is omitted it derives a sanitized,
collision-free `<agent>-<workspace-basename>` name, so it never has to parse `create` output.
Re-using an existing name returns `client.ErrSandboxExists`.

### 3. List & inspect

```go
all, _ := sandbox.List(ctx, c)            // []*sandbox.Sandbox
sb, _ := sandbox.Get(ctx, c, "review-bot")

// Cheap accessors off the last-known state:
sb.Name()        // string
sb.Agent()       // "" if unset
sb.State()       // status string
sb.IsRunning()   // bool

// Inspect refreshes from the daemon and returns the full record (sandbox.Info):
info, _ := sb.Inspect(ctx)
fmt.Println(info.Status, info.Workspace, info.Ports)
```

### 4. Lifecycle: start / stop / remove

```go
_ = sb.Start(ctx)   // bring the micro-VM up
_ = sb.Stop(ctx)    // bring it down, keep the sandbox
_ = sb.Remove(ctx)  // delete it (no confirmation prompt)

// Force-remove one with an active session (e.g. an open SSH connection):
_ = sb.Remove(ctx, sandbox.WithForce())
```

### 5. Exec commands

`exec.Exec` runs a command to completion. The sandbox must be running — pass
`exec.WithAutoStart()` to transparently start a stopped one (otherwise you get
`client.ErrSandboxNotRunning`).

**Capture output:**

```go
code, out, err := exec.Exec(ctx, sb, []string{"go", "test", "./..."},
	exec.WithWorkdir("/workspace"),
	exec.WithEnv(map[string]string{"CGO_ENABLED": "0"}),
	exec.WithAutoStart(),
)
body, _ := io.ReadAll(out) // demuxed stdout; stderr is discarded in capture mode
```

**Stream stdout and stderr live** to your own writers (the returned reader is then empty):

```go
code, _, err := exec.Exec(ctx, sb, []string{"npm", "run", "build"},
	exec.WithMultiplexed(os.Stdout, os.Stderr),
)
```

**Other exec options:** `WithUser("root")`, `WithPrivileged()`, `WithTTY()`.

**Interactive attach** — a live bidirectional session (stdin/stdout + TTY resize):

```go
sess, _ := exec.ExecInteractive(ctx, sb, []string{"bash"}, exec.WithTTY())
defer sess.Close()

io.WriteString(sess.Stdin(), "ls -la\n")
_ = sess.Resize(ctx, 120, 40)
go io.Copy(os.Stdout, sess.Stdout())
code, _ := sess.Wait(ctx)
```

**Follow a log file** — `exec.Logs` is a convenience wrapper that runs `tail -F
<path>` under an interactive attach and hands back the live session. Read
`Stdout()` to stream new lines until you `Close()`; `-F` keeps following across log
rotation and waits for a not-yet-created file. For a full replay or different
flags, call `ExecInteractive` with your own command.

```go
sess, _ := exec.Logs(ctx, sb, "/var/log/app.log")
defer sess.Close()
io.Copy(os.Stdout, sess.Stdout()) // streams continuously
```

**Detached** — fire-and-forget; poll for completion:

```go
id, _ := exec.ExecDetached(ctx, sb, []string{"sh", "-c", "long-job"})
state, _ := exec.InspectExec(ctx, sb, id) // State{ Running, ExitCode }
```

**Resource stats** — a point-in-time CPU/memory/disk/uptime snapshot, read from
`/proc` and `df` inside the sandbox (the same metrics the `sbx` TUI shows). There
is no daemon stats endpoint; `Stats` simply execs a tiny probe, so the sandbox
must be running (or pass `exec.WithAutoStart()`). It samples CPU over a ~200ms
window, so the call blocks briefly.

```go
u, err := exec.Stats(ctx, sb)
// exec.Usage{ Cores, MemTotalKB, MemAvailableKB, MemUsedKB, CPUPercent, UptimeSeconds, DiskTotalGB, DiskUsedGB }
fmt.Printf("cpu %.1f%% / %d cores, mem %d/%d MiB, disk %.0f/%.0f GiB\n",
	u.CPUPercent, u.Cores, u.MemUsedKB/1024, u.MemTotalKB/1024, u.DiskUsedGB, u.DiskTotalGB)
```

`CPUPercent` is the mean utilization across all cores, clamped to 0–100; memory
is in KiB, disk (root filesystem) in GB. `UptimeSeconds` and the `Disk*` fields
are best-effort — they read 0 if the sandbox can't supply them (e.g. a busybox
`df` without `-BG`), without failing the CPU/memory snapshot.

### 6. Run an agent interactively

`sandbox.Run` is the **agent session**: it creates the sandbox if missing, then attaches your
terminal to the agent and blocks until it exits. A non-zero agent exit is `(code, nil)` — only
spawn/wait failures return a non-nil error. It returns no handle (use `Create`/`Get` for that).

```go
// Provision-if-missing + attach the agent to your terminal:
code, _ := sandbox.Run(ctx, c,
	sandbox.WithAgent("claude"),
	sandbox.WithWorkspace("."),
	sandbox.WithAgentArgs("-p", "fix the failing test"), // args after "--"
)

// Re-attach to an existing sandbox's agent:
code, _ = sb.Run(ctx, sandbox.WithAgentArgs("--continue"))

// Redirect stdio (default is os.Stdin/out/err):
sandbox.Run(ctx, c, sandbox.WithAgent("shell"), sandbox.WithWorkspace("."),
	sandbox.WithStdio(myIn, myOut, myErr))
```

### 7. Copy files

`CopyTo` (host → sandbox) shells out to the `sbx` binary. `CopyFrom` (sandbox → host) reads
`GET /sandbox/{name}/files` over REST (this endpoint answered `404` through v0.35.0; it works as
of v0.37.0) and extracts the tar stream itself. `CopyFrom` auto-starts a stopped sandbox to match
`sbx cp`'s own behaviour — unlike `exec`, which requires an explicit `WithAutoStart()`.

```go
_ = sb.CopyTo(ctx, "./config.json", "/home/user/config.json")
_ = sb.CopyTo(ctx, "./link", "/home/user/target", sandbox.WithFollowSymlinks())
_ = sb.CopyFrom(ctx, "/home/user/out.log", "./out.log")
```

### 8. Publish ports

```go
// Publish one mapping (additive); returns the full published set.
ports, _ := sb.PublishPort(ctx, sandbox.Port{
	SandboxPort: 8080,
	HostIP:      "127.0.0.1",
	Protocol:    "tcp",
	// HostPort: 0 => the daemon assigns an ephemeral host port
})

ports, _ = sb.Ports(ctx)                          // list published ports
_ = sb.UnpublishPort(ctx, "127.0.0.1:18080:8080/tcp") // remove by CLI spec
```

### 9. Templates

Templates are images. Snapshot a sandbox, then create new sandboxes from it.

```go
// The daemon refuses to snapshot a running sandbox — stop it first.
_ = sb.Stop(ctx)
_ = sb.SaveTemplate(ctx, "myimg:v1")

imgs, _ := template.List(ctx, c)                  // []template.Image
img, _ := template.Inspect(ctx, c, "myimg:v1")
_ = template.Remove(ctx, c, "myimg:v1")

f, _ := os.Open("image.tar")
_ = template.Load(ctx, c, f)                      // import a tar into the image store
```

### 10. Network policy

Governs which hosts a sandbox may reach. Scope `""` is global; a sandbox name scopes a rule.

```go
_ = policy.SetDefault(ctx, c, "balanced")            // "allow-all" | "balanced" | "deny-all"
_ = policy.Allow(ctx, c, "", "api.github.com")       // global allow
_ = policy.Deny(ctx, c, "review-bot", "evil.test")   // per-sandbox deny
_ = policy.RemoveRule(ctx, c, "review-bot", "evil.test")  // resource selector required
_ = policy.Reset(ctx, c)

rules, _ := policy.List(ctx, c, "")                  // []policy.PolicyRule (REST, always type=all)
raw, _ := policy.ListRaw(ctx, c, "")                 // unparsed text escape hatch
prof, _ := policy.Profiles(ctx, c)                   // raw text (no --json upstream)
log, _ := policy.Log(ctx, c)                         // structured: allowed/blocked hosts
for _, e := range log.BlockedHosts {
	fmt.Println("blocked:", e.Host, e.VMName)
}

// Ask whether the current policy would authorize an access, without connecting:
auth, _ := policy.Check(ctx, c, "evil.test")                          // *policy.Authorization
fmt.Println(auth.Allowed, auth.Rule, auth.Reason)
_, _ = policy.Check(ctx, c, "api.github.com", policy.WithCheckSandbox("review-bot"))

detail, _ := policy.InspectRaw(ctx, c, "review-bot-net-allow") // human-rendered detail; selector from List
```

### 11. Secrets

Stored, proxy-injected credentials. **Experimental upstream** — for headless agent
credentials, prefer `exec.WithEnv` instead.

```go
_ = secret.SetCustom(ctx, c, "", secret.CustomSecret{
	Host:  "api.example.com", // requests to this host get the real value
	Env:   "API_KEY",         // env var (set to a placeholder) inside the sandbox
	Value: "sk-...",          // the real secret
})

// SetToken/SetRegistry write the secret to the child process's stdin — it never
// appears in the argument vector, unlike SetCustom above.
_ = secret.SetToken(ctx, c, "", "anthropic", os.Getenv("ANTHROPIC_API_KEY"))
_ = secret.SetRegistry(ctx, c, "", secret.RegistryCredential{
	Host:     "ghcr.io",
	Username: "me",
	Password: os.Getenv("GHCR_TOKEN"),
})

// Detect and store credentials already present in the host environment:
_ = secret.Import(ctx, c, "openai")                    // one named service (OPENAI_API_KEY)
_ = secret.ImportAll(ctx, c, secret.WithDryRun())      // preview everything detected; drop WithDryRun to write

secrets, _ := secret.List(ctx, c, "")      // *secret.Secrets{Stored, Custom}
raw, _ := secret.ListRaw(ctx, c, "")       // unparsed text escape hatch
_ = secret.Remove(ctx, c, "", "openai")              // a service/registry secret (positional name)
_ = secret.RemoveCustom(ctx, c, "", "api.example.com") // a custom secret (keyed by host)
```

> ⚠️ `SetCustom` passes the value as a CLI argument, so it is briefly visible in host process
> listings. Don't use it for high-sensitivity secrets in shared environments.

---

### 12. Settings & SSH

`settings` wraps `sbx settings … --json`; `ssh` manages the experimental SSH endpoint
(composed over `settings`). Both shell out to the `sbx` binary.

```go
// Read a setting (typed JSON value).
s, _ := settings.Get(ctx, c, "ssh.autoCreate")
auto, _ := s.Bool()                          // or s.Text() for string settings

// Write / clear an override (fire-and-forget; daemon hot-reloads within ~5s).
_ = settings.Set(ctx, c, "kit.allowedSources", `["docker.io/","ghcr.io/"]`)
_ = settings.Unset(ctx, c, "kit.allowedSources")

// Enable and connect to the SSH endpoint.
_ = ssh.Enable(ctx, c)             // settings set feature.ssh true
_ = ssh.Setup(ctx, c)              // writes ~/.ssh/config "Host *.sbx" + known_hosts (no key)
tgt := ssh.TargetFor("mybox")      // {Host:"mybox.sbx"}
fmt.Println(tgt.Command())         // ssh mybox.sbx
```

> `Set`/`Enable`/etc. are fire-and-forget — they write `settings.json` and return; the
> daemon reloads within ~5s. `ssh.Enable` toggles only `feature.ssh`; SSH also requires
> `platform.allowExperimentalFeatures` (default `true`).

### 13. Skills

`skillstore` imports the host's agent skill folders into sandboxd's shared skills store
(`sbx skills import`, **experimental upstream**, added in v0.37.0). Imported skills survive
sandbox deletion; `sbx reset` clears the store. Shells out to the `sbx` binary.

```go
_ = skillstore.Import(ctx, c)                          // --force: replaces existing skills (CLI backs them up first)
_ = skillstore.Import(ctx, c, skillstore.WithDryRun()) // preview only, writes nothing
```

---

## Error handling

Branch on sentinels with `errors.Is`; pull detail out of `*client.APIError` (REST) or
`client.CLIError` (shell-out) with `errors.As`.

```go
sb, err := sandbox.Get(ctx, c, "nope")
if errors.Is(err, client.ErrSandboxNotFound) {
	// handle missing sandbox
}

var apiErr *client.APIError
if errors.As(err, &apiErr) {
	fmt.Println(apiErr.Op, apiErr.Status, apiErr.Message)
}
```

| Sentinel | Raised when |
| --- | --- |
| `client.ErrSandboxNotFound` | REST `404` (get/inspect/lifecycle on a missing sandbox). |
| `client.ErrSandboxExists` | REST `409`, or `Create` with a name that's already taken. |
| `client.ErrSandboxNotRunning` | `exec.*` on a stopped sandbox without `WithAutoStart`. |
| `client.ErrExecNotFound` | `InspectExec` for an unknown exec id. |
| `client.ErrIncompatibleVersion` | `WithStrictVersion` and the daemon is incompatible. |
| `client.ErrDaemonNotRunning` | `EnsureRunning` couldn't make the socket healthy in time. |
| `client.ErrBinaryNotFound` | the `sbx` binary isn't on `PATH` (any shell-out op). |

## Runnable examples

Self-contained programs live in [`examples/`](examples/) — each is `go run`-able against a
live daemon:

| Example | Shows |
| --- | --- |
| [`examples/quickstart`](examples/quickstart) | Connect → create → exec → remove. |
| [`examples/exec`](examples/exec) | Capture, env/workdir, streaming, detached + poll, resource stats. |
| [`examples/run-agent`](examples/run-agent) | Interactive agent session over your terminal. |
| [`examples/resources`](examples/resources) | Ports, file copy, templates, policy, secrets. |

```bash
go run ./examples/quickstart
```

## Agent skill

A Claude Code skill ships with this repo at
[`skills/sbx-go-sdk/SKILL.md`](skills/sbx-go-sdk/SKILL.md). When an agent
works in (or imports) this SDK, the skill teaches it the API surface, the Create-vs-Run-vs-Exec
distinction, and the gotchas below — so it reaches for the right call without re-reading the
source. Invoke it with `/sbx-go-sdk`.

## Version alignment

This SDK is **pinned to a tested `sbx` / `sandboxd` range**. It is currently built and
live-verified against **`sbx` v0.37.0** with daemon REST **`api_version 0.24.0`**. Both values
are exported constants you can read at runtime:

```go
client.ClientVersion    // "v0.37.0" — the sbx/daemon version the SDK was built against
client.TestedAPIVersion // "0.24.0"  — the daemon REST api_version its wire types were generated from
```

**Why a pin exists.** The [transport is hybrid](#how-it-works): REST wire structs are generated
from the `sbx` binary's DWARF, and orchestration ops shell out to versioned CLI flags. A daemon
that changes either surface can drift from what the SDK expects, so the SDK targets a known-good
range rather than promising forward-compatibility with every release.

**Version checking is a development-time gate, not a runtime one** — see
[ADR 0004](docs/adr/0004-version-alignment-is-a-development-time-gate.md). `client.WithStrictVersion()`
and `Client.CheckVersion` are **deprecated**: `api_version` bumps on every `sbx` release with no
documented compatibility contract, so a strict compare fires on every upgrade — including ones
where nothing the SDK uses changed — and upstream removed `POST /version` entirely in v0.37.0, so
`CheckVersion` can no longer succeed against a current daemon. Drift detection instead lives in
`TestContract_VersionAlignment`, an integration test a maintainer runs deliberately, not something
`client.New` enforces. For a runtime check, compare `DaemonHealth().Version` / `.APIVersion`
against the constants yourself:

```go
h, _ := c.DaemonHealth(ctx)
if h.Version != client.ClientVersion || h.APIVersion != client.TestedAPIVersion {
    log.Printf("daemon %s/api %s differs from SDK-tested %s/api %s — behaviour may have drifted",
        h.Version, h.APIVersion, client.ClientVersion, client.TestedAPIVersion)
}
```

**How the SDK re-aligns when a newer `sbx` ships.** Maintainers run a re-sync loop, guarded by a
contract test:

1. Install the new `sbx`, then run the drift gate:
   `go test -tags integration -run TestContract_VersionAlignment ./internal/integration`.
   It compares the live daemon's `version` / `api_version` to the pinned constants and **fails on
   drift** with a remediation hint (set `SBX_ALLOW_VERSION_DRIFT=1` to downgrade to a warning
   while upgrading).
2. Regenerate the wire types: `go run ./internal/tools/dwarfgen -bin $(which sbx)`, then review
   the [`internal/api/types_gen.go`](internal/api/types_gen.go) diff.
3. Reconcile [`docs/sbx-version-coverage.md`](docs/sbx-version-coverage.md) against the upstream
   release notes for the new version — the drift gate's failure message points here, so a sync
   that quietly misses features (as v0.35.0's did) gets caught.
4. Run the full `integration` suite against the new daemon, migrate any
   [stubbed endpoints](#known-deviations--limitations) that are now implemented, and bump
   `client.ClientVersion` / `client.TestedAPIVersion`.

So: **pin to a tested range, negotiate leniently at runtime, and re-sync deliberately** — the
contract test is what tells maintainers a re-sync is due.

## Known deviations & limitations

Verified live against `sandboxd` v0.37.0:

- **`CopyFrom` is REST and auto-starts the sandbox** — `GET /sandbox/{name}/files?path=…` works as
  of v0.37.0 (it was `404` through v0.35.0). `CopyFrom` starts a stopped sandbox first, matching
  `sbx cp`'s own behaviour; this differs from `exec`, which requires an explicit `WithAutoStart()`.
  `CopyTo` still shells out — there is no REST upload path. `CopyFrom` places no cap on extracted
  size.
- **`policy.List` is REST** — `GET /policy/network/rules` works as of v0.37.0 and always sends
  `type=all` (omitting it silently drops filesystem rules with no error). A shape change yields
  `client.ErrUnexpectedFormat`; use `policy.ListRaw` for the human table. `secret.List` still
  parses the CLI table (no `--json` upstream); `policy.Profiles` is raw text.
- **`SaveTemplate` requires a stopped sandbox** — the daemon refuses to snapshot a running one,
  and the CLI would otherwise block on an interactive stop prompt.
- **`UnpublishPort` is REST** — it first sends `GET /sandbox/{name}/ports` to resolve which keys
  match the spec, then `POST /sandbox/{name}/ports/unpublish` (body: a bare `[]PortKey` array);
  works as of v0.37.0.
- **`secret.SetCustom` is experimental** and exposes the value via the process list.
- **`secret.SetToken`/`SetRegistry` keep the secret off the argument vector** — both write the
  value to the child process's stdin rather than passing it as a CLI argument, so unlike
  `SetCustom` it does not appear in the host process list. Confirmed by a live run: `--force`
  **does** work on this stdin path, despite upstream's `--help` claiming it applies only "when
  `--token` is used".
- **`secret.SetToken`, `SetRegistry` and `Import` reject an existing entry** rather than
  overwriting it. Without `--force`, `sbx secret set`/`secret import` on an entry that already
  exists in the target scope prompts for confirmation; on non-interactive stdin that prompt reads
  EOF, cancels, and **exits 0 having stored nothing**. All three now check for an existing entry
  before invoking the CLI and return an error in that case instead. Pass `WithOverwrite()`
  (`SetToken`/`SetRegistry`) or `WithOverwriteExisting()` (`Import`) to opt into replacing it.
- **Two related paths may have the same silent-success shape, unverified.** `sbx` has a second
  overwrite prompt — `"%s OAuth token already exists. Overwrite? (y/N): "` — for OAuth-configured
  services; if `secret ls` doesn't surface those as rows, `SetToken` against such a service could
  still reach that prompt and exit 0 silently. `SetCustom` has no pre-flight check at all and
  pipes nothing to stdin, so `set-custom` against an existing entry may have the same shape.
  Neither can be confirmed without running `sbx secret` against real credentials, so both are
  recorded as known limitations rather than fixed here.
- **`settings`/`ssh` mutations are fire-and-forget** — `settings.Set/Unset` and
  `ssh.Enable/Disable/Setup` write host state (`settings.json`, `~/.ssh/config`) and return before
  the daemon's ~5s hot-reload; reads (`settings.Get/List`, `ssh.Enabled`) use `--json`.
  `ssh.Enable` sets only `feature.ssh` (SSH also needs `platform.allowExperimentalFeatures`, default true).
- **SSH connects by hostname** — v0.35.0 dropped the `ssh.port` loopback model; `ssh.Setup` writes a
  wildcard `Host *.sbx` block and you connect with `ssh <name>.sbx` (no port, no key). `ssh.TargetFor`
  builds that hostname.
- **`kit.Inspect` does not report a kit's files.** `sbx kit inspect --json` omits the `files/`
  payload, although `kit.Pack` writes it into the artifact. `Info` is a report, not the kit.
- **`kit.Push` and `kit.Pull` are unverified.** Neither has been run against a real OCI registry;
  both ship with argument-vector unit tests only.
- **Local kit paths are made absolute.** `AddKit` and `WithKit` rewrite a reference that exists on
  disk. The daemon records the kit list verbatim and resolves a relative path against its own
  working directory, which would record a path that does not exist and break every later add.
- **`Sandbox.AddKit` cannot apply every kit.** The CLI refuses, pre-flight, any kit declaring
  `credentials`, `publishedPorts`, `volumes`, `commands.startup` or `commands.initFiles`. Use
  `sandbox.WithKit` at creation for those.
- **`Sandbox.Kits` reads a label.** If upstream renames `com.docker.sandbox.kits`, it returns empty
  rather than an error.

See the design spec and Plan 2 doc under [`docs/`](docs/) for the full rationale.
