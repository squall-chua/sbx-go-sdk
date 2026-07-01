# `sbx` CLI — Reverse Engineering Notes

Reverse-engineered from `/usr/bin/sbx` (unstripped Go 1.26.4 binary, with DWARF).

- **Module:** `github.com/docker/sandboxes` `v0.34.0`
- **Main package:** `github.com/docker/sandboxes/cli-plugin/cmd/sandboxes`
- **Daemon API version:** `0.16.0` (build `2eae0c4fc3894475da3318615f69783b0e7be747`, 2026-06-26)
- **What it is:** Docker Sandboxes — isolated micro-VM sandboxes for AI coding agents.
  Shipped both as a standalone `sbx` binary and as a `docker sandboxes` CLI plugin.
- **Single-binary model (like docker/dockerd):** the same binary is both the CLI
  *and* the `sandboxd` daemon. The CLI re-execs itself to start the daemon.

> Refreshed for **v0.34.0**: the header facts and the §1 command tree were re-verified
> against the installed binary. §2–§3 (architecture, REST surface) reflect the original
> v0.32.0 recon; the SDK-exercised REST endpoints are re-confirmed live at v0.34.0 by the
> `internal/integration` suite.

---

## 1. Command tree

Top-level CLI framework is **cobra**. Root command `sbx` launches an interactive
TUI (bubbletea) when run with no args; otherwise it dispatches subcommands.

Global flag: `-D, --debug`.

### Visible commands
```
sbx                                          # interactive TUI mode
sbx tui                                       # open the interactive TUI dashboard (explicit)
sbx cp [flags] SRC DST                        # copy files host <-> sandbox (SANDBOX:PATH)
sbx create [flags] AGENT PATH [PATH...]       # create a sandbox for an agent
    create claude|codex|copilot|cursor|docker-agent(cagent)|droid|gemini|kiro|opencode|shell
      flags: --clone --cpus --kit --memory/-m --name --profile --quiet/-q --template/-t
sbx diagnose                                  # diagnose install issues
sbx exec [flags] SANDBOX COMMAND [ARG...]     # exec a command in a sandbox
sbx kit COMMAND                               # (experimental) kit artifacts
    kit add SANDBOX REFERENCE | inspect REFERENCE | pack DIR | pull REFERENCE
        | push DIR REFERENCE | validate REFERENCE
sbx login [flags]                             # sign in to Docker
sbx logout [flags]                            # stop sandboxes + sign out
sbx ls [flags]                                # list sandboxes
sbx policy COMMAND                            # manage sandbox network/egress policies
    policy allow network [--sandbox S] RESOURCES
    policy deny  network [--sandbox S] RESOURCES
    policy rm    network [--sandbox S]
    policy log [SANDBOX] | ls [SANDBOX] | reset
    policy profile ls
    policy init <allow-all|balanced|deny-all>   # renamed from set-default in v0.34.0 (kept as hidden deprecated alias)
sbx ports SANDBOX [flags]                     # manage published ports
sbx reset [flags]                             # reset all sandboxes + clean state
sbx rm [SANDBOX...] [flags]                    # remove sandboxes
sbx run [flags] SANDBOX | AGENT [PATH...] [-- AGENT_ARGS...]   # run/attach an agent
sbx secret COMMAND                            # manage stored secrets
    secret ls [SANDBOX] | rm [-g|SANDBOX] [SERVICE] | rm --placeholder PH | rm --registry REF
    secret set [-g|SANDBOX] [SERVICE] | set-custom [-g|sandbox]
sbx setup                                     # (experimental) detect host config + prepare sbx
sbx stop SANDBOX [SANDBOX...]                  # stop without removing
sbx template COMMAND                          # manage sandbox templates
    template load FILE | ls | rm TAG|ID | save SANDBOX TAG
sbx version                                   # version info
sbx completion bash|zsh|fish|powershell
```

### Hidden commands (not in `--help`, but registered)
```
sbx daemon                                    # manage the sandboxd daemon
    daemon start [-d/--detach] [--policy allow-all|balanced|deny-all]
    daemon status
    daemon stop
    daemon log-level [set <proxy|general|all> <level>]
sbx settings                                  # persistent settings (JSON, hot-reloaded ~5s)
    settings get | list | set | unset
```
`credentials*` symbols back the user-facing `secret` command; `mcp*`/`save*` symbols
exist in the binary but are not registered as standalone top-level commands
(internal / used as subcommands of `template`/`run`).

Agents supported by `create`/`run`: **claude, codex, copilot, cursor, docker-agent
(alias cagent), droid, gemini, kiro, opencode, shell**.

---

## 2. How the CLI talks to the daemon

### Two-layer architecture

```
 sbx (CLI)  ──REST/HTTP over unix socket──▶  sandboxd (daemon)
                                               │
                                               ├─ docker.sock  (Docker-compatible engine endpoint)
                                               └─ Connect-RPC / containerd (docker-next) engine
                                                    serves docker.{container,image,network,
                                                    sandbox,volume}.v0 + governance.policy.v1
 sandbox (micro-VM, gVisor) ◀── per-sandbox vsock  ~/.sbx/run/<id>-vm.sock
```

The **CLI ⇄ daemon** channel is a plain **HTTP REST API** (generated by
`oapi-codegen`, served with the **Echo** router) over a **unix domain socket**.
It is NOT gRPC/Connect. (`sandboxapi` package: `ServerInterface`, `EchoRouter`,
`ServerInterfaceWrapper`, `HttpRequestDoer`, `NewClient`, `New<Op>Request…`.)

The **daemon ⇄ engine** channel underneath uses **Connect RPC** (`connectrpc.com/connect`)
and a containerd fork (`docker-next-containerd`). Those Connect services
(`docker.container.v0.ContainerService`, `docker.sandbox.v0.SandboxService`,
`docker.image.v0.ImageService`, `docker.network.v0.NetworkService`,
`docker.volume.v0.VolumeService`, `docker.governance.policy.v1.PolicyService`,
`docker.governance.events.v0.EventsIngestionService`) are internal to the daemon
and not what the CLI dials directly. A separate `gwapi`/`gateway.v1` Connect client
talks to the **Docker cloud gateway** (login, policy fetch, gateway env).

### Socket location
Resolved by `sandboxlib.DefaultSocketPath` (`constants_unix.go`):
```
$XDG_STATE_HOME/sandboxes/sandboxes/sandboxd/sandboxd.sock
# observed: /home/<user>/.local/state/sandboxes/sandboxes/sandboxd/sandboxd.sock
```
- Logs: `…/sandboxd/daemon.log`
- The daemon also opens a Docker-compatible socket: `…/sandboxd/docker.sock`
- Because unix paths are capped at 108 bytes (`ErrSocketPathTooLong`), a short
  symlink is used under `~/.sbx/run/` (`ShortStateDirSymlink`). Per-sandbox VM
  sockets live there too: `~/.sbx/run/<id>-vm.sock`.
- Transport: Go `http.Client` with a `DialContext` to the unix socket
  (`sandboxlib.SocketPathToURL`, `DialSocketWithTimeout`,
  `httpclient.NewWithBaseAndTimeout`). Headers are added via an
  `injectHeadersRoundTripper`.

### Daemon lifecycle / auto-start
`commands.ensureDaemon` → `startDaemon` (`daemon.go`):
1. `isDaemonLiveAtSocket()` — probe the socket; if healthy, reuse it.
2. `canOpenDatabase()` — guard the state DB (bbolt).
3. `os.Executable()` + `buildDaemonArgs()` → `exec.Command(self, …)` →
   `Cmd.Start()` (detached) → `waitForDaemon()` polls until the socket is live.
   I.e. **the CLI re-execs its own binary as `sandboxd`.**
   (Guard: refuses to spawn from a test binary to avoid a fork bomb.)
Env override: `DOCKER_SANDBOXES_IP_STACK` selects the network stack before start.

### Version negotiation
`POST /version` — CLI sends its version, daemon replies
`{"result":"compatible"|"incompatible"|"unknown"}`. A mismatch prompts a daemon
restart.

### Auth
- Local CLI ⇄ daemon over the unix socket: not bearer-authenticated (filesystem
  perms on the socket are the boundary); `/health`, `/daemon/info`, `/sandbox`
  answer without a token.
- Outbound to Docker cloud / gateway and to in-sandbox MCP/agents uses
  `Authorization: Bearer <token>` (Docker login token seeded from
  `~/.docker/.token_seed`; gateway/OAuth tokens for policy + agent creds).

---

## 3. Daemon REST API (verified live against the running daemon)

Base: `http://localhost` over the unix socket. Echo router. `{name}` = sandbox id/name,
`{exec}` = exec id.

| Method | Path | Purpose |
|--------|------|---------|
| GET  | `/health` | liveness `{release,status,version}` |
| POST | `/version` | client/daemon version compatibility check |
| GET  | `/daemon/info` | `{api_socket, docker_socket}` |
| GET  | `/daemon/health` | `{api_version, revision, release, status, version}` |
| GET  | `/daemon/loglevel` | `{general, proxy}` log levels |
| POST | `/daemon/loglevel/set` | set a category's log level |
| GET  | `/sandbox` | list sandboxes |
| POST | `/sandbox` | create a sandbox |
| GET  | `/sandbox/{name}` | inspect sandbox |
| DELETE | `/sandbox/{name}` | remove sandbox |
| POST | `/sandbox/{name}/start` | start sandbox |
| POST | `/sandbox/{name}/stop` | stop sandbox |
| POST | `/sandbox/{name}/exec` | create/run an exec (attach via conn upgrade/hijack) |
| GET  | `/sandbox/{name}/exec/{exec}` | inspect an exec |
| POST | `/sandbox/{name}/exec/{exec}/resize` | resize exec TTY |
| GET  | `/sandbox/{name}/ports` | list published ports |
| POST | `/sandbox/{name}/ports` | publish ports |
| GET  | `/sandbox/{name}/files` | read files/archive out of sandbox (`cp` from) |
| PUT  | `/sandbox/{name}/files` | write files/archive into sandbox (`cp` to) |
| POST | `/sandbox/{name}/save` | save sandbox as a template image |
| POST | `/sandbox/{name}/credentials` | set sandbox secrets/credentials |

CLI-side client ops (from `sandboxapi` symbols) map onto the above:
`ListSandboxes, InspectSandbox, DeleteSandbox, ExecCommand, InspectExec,
AttachExec, ListPublishedPorts, PublishPorts, ListImages, InspectImage, LoadImage,
ExportImage, FetchHealth, FetchDaemonInfo, FetchLogLevels, FetchDebugState`.
(Image load/export & debug-state operations exist as client request builders;
image/network/volume management is primarily handled at the engine/`docker.sock`
layer rather than top-level REST paths.)

---

## 4. Reproduce the recon

```bash
go version -m /usr/bin/sbx                       # module + deps
go tool nm /usr/bin/sbx | grep docker/sandboxes  # symbol map
sbx --help ; sbx <cmd> --help                    # cobra command tree
sbx -D daemon status                             # prints socket + log paths
SOCK=~/.local/state/sandboxes/sandboxes/sandboxd/sandboxd.sock
curl -s --unix-socket "$SOCK" http://localhost/health
curl -s --unix-socket "$SOCK" http://localhost/daemon/info
curl -s --unix-socket "$SOCK" http://localhost/sandbox
curl -s -X OPTIONS -D- -o/dev/null --unix-socket "$SOCK" http://localhost/sandbox  # Allow: header
```
