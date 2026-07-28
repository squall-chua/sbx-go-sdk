# `sbx` CLI — Reverse Engineering Notes

Reverse-engineered from `/usr/bin/sbx` (unstripped Go 1.26.5 binary, with DWARF).

> `docs/sbx-version-coverage.md` is the authority on which release a feature
> shipped in — it is reconciled against upstream release notes and wired into
> the drift gate. If a version marker below ever disagrees with that table,
> the table wins.

- **Module:** `github.com/docker/sandboxes` `v0.37.0`
- **Main package:** `github.com/docker/sandboxes/cli-plugin/cmd/sandboxes`
- **Daemon API version:** `0.24.0` (build `8b65b864b0d49c29f05a55170d6b5eea4c0d11e7`, 2026-07-27)
- **What it is:** Docker Sandboxes — isolated micro-VM sandboxes for AI coding agents.
  Shipped both as a standalone `sbx` binary and as a `docker sandboxes` CLI plugin.
- **Single-binary model (like docker/dockerd):** the same binary is both the CLI
  *and* the `sandboxd` daemon. The CLI re-execs itself to start the daemon.

> Refreshed for **v0.37.0** (daemon api `0.22.0` → `0.24.0`). **No wire-type changes**: re-running
> `dwarfgen` produced a byte-identical `internal/api/types_gen.go` apart from the known generator
> artifacts. The one rename is internal-only — the `SandboxStatus` enum type is now
> `SandboxInfoStatus` — and it does not affect JSON. The whole existing SDK surface passes the
> `internal/integration` suite unchanged at v0.37.0.
>
> New this release: a `skills` command (shared agent skills store) and `-p/--publish` on
> `create`/`run`. `policy check` / `policy inspect` and `secret import` shipped in v0.35.0 and were
> missed by that sync — this recon is what caught the gap; see docs/sbx-version-coverage.md. Three
> REST paths the SDK had written off as absent are now live: `GET /sandbox/{name}/files`,
> `POST /sandbox/{name}/ports/unpublish`, and `GET /policy/network/rules`. See §3.
>
> Earlier, for **v0.35.0** (daemon api `0.16.0` → `0.22.0`): the standalone `GET /health` endpoint
> was **removed** (now `404`); `/daemon/health` is the liveness signal. `SandboxInfo` gained a
> `credential_sources` field (`map[string]{source,type}`). §2 (architecture) reflects the original
> v0.32.0 recon.

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
      flags: --clone --cpus --kit --memory/-m --name --profile --publish/-p --quiet/-q --template/-t
      (--publish/-p is NEW in v0.37.0; --no-share-skills exists but is gated off by a
       remote feature flag, so it is absent from --help on this host)
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
    policy check network [--sandbox S] [--json] [--verbose] TARGET   # NEW in v0.35.0
    policy inspect <policy-or-rule>             # NEW in v0.35.0 (by policy/rule ID or name)
sbx ports SANDBOX [flags]                     # manage published ports
sbx reset [flags]                             # reset all sandboxes + clean state
sbx rm [SANDBOX...] [--all] [-f/--force]       # remove sandboxes; --all is NEW in v0.37.0
sbx run [flags] SANDBOX | AGENT [PATH...] [-- AGENT_ARGS...]   # run/attach an agent
      flags: as create, plus --publish/-p (NEW) and a hidden --detached/-d
      (--detached prints the sandbox ID and exits without an interactive session)
sbx secret COMMAND                            # manage stored secrets
    secret ls [SANDBOX] | rm [-g|SANDBOX] [SERVICE] | rm --placeholder PH | rm --registry REF
    secret set [-g|SANDBOX] [SERVICE] | set-custom [-g|sandbox]
    secret import                               # NEW in v0.35.0 (import secrets found in host env vars)
sbx setup                                     # (experimental) detect host config + prepare sbx
    setup ssh [--alias PATTERN]                 # NEW path in v0.37.0 for `sbx ssh setup` (both still work)
sbx skills COMMAND                            # NEW in v0.37.0 (experimental) shared agent skills store
    skills import [--dry-run] [-f/--force]      # copy host skill dirs into the shared store
sbx ssh [flags]                               # now a visible top-level command (provisioning helper)
    ssh setup [--alias PATTERN]                 # hidden alias of `sbx setup ssh`; same flags
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
**Removed in v0.37.0.** `POST /version` used to take the CLI's version and reply
`{"result":"compatible"|"incompatible"|"unknown"}`. At v0.37.0 the route is gone —
`OPTIONS`, `GET`, `POST` and `PUT` all return `404` with no `Allow` header, and the daemon
reports `"release":true`, so this is a real removal and not a non-release-build quirk.
The `api_version` field of `GET /daemon/health` is now the only version signal.

### Auth
- Local CLI ⇄ daemon over the unix socket: not bearer-authenticated (filesystem
  perms on the socket are the boundary); `/daemon/health`, `/daemon/info`, `/sandbox`
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
| ~~POST~~ | ~~`/version`~~ | **REMOVED in v0.37.0** — 404 on every verb, no route. Was the client/daemon compatibility check. `/daemon/health`'s `api_version` is now the only version signal. |
| GET  | `/daemon/info` | `{api_socket, docker_socket}` |
| GET  | `/daemon/health` | liveness `{api_version, revision, release, status, version}` (replaces the removed `/health`) |
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
| POST | `/sandbox/{name}/ports/unpublish` | unpublish ports — body is a **bare `[]PortKey` array** |
| GET  | `/sandbox/{name}/files?path=<abs>` | read files/archive out of sandbox (`cp` from); `200 application/x-tar` |
| PUT  | `/sandbox/{name}/files` | write files/archive into sandbox (`cp` to) |
| POST | `/sandbox/{name}/save` | save sandbox as a template image |
| POST | `/sandbox/{name}/credentials` | set sandbox secrets/credentials |
| GET  | `/policy/network/rules` | list network policy rules — same JSON shape as `sbx policy ls --json` |
| POST | `/policy/network/rules` | add/remove network policy rules (allow/deny/rm) |
| POST | `/policy/network/check` | evaluate a network access request (`sbx policy check network`) |
| GET  | `/policy/network/profiles` | list policy profiles → `{"profiles":[…]}` |
| POST | `/daemon/reset` | reset all sandboxes + daemon state (`sbx reset`) |

**Live-verified at v0.37.0** (paths absent or `404` at v0.35.0):

- `GET /sandbox/{name}/files?path=<abs>` → `200`, `Content-Type: application/x-tar`. `path` is a
  **required** query param and must be the absolute path *inside* the sandbox (the workspace is
  mounted at its host path, so it is usually the host path). An unknown path gives `404`
  `{"message":"path … not found in sandbox …"}`; a missing `path` gives `400`.
- `POST /sandbox/{name}/ports/unpublish` → `200 {"message":"unpublished N port key(s)"}`. The body
  is a bare JSON array of `PortKey` (`{host_ip?, host_port?, protocol?, sandbox_port}`), **not** an
  object with a `ports` field. Unpublish both address families to fully release a port — a
  loopback publish creates a `127.0.0.1` key *and* a `::1` key.
- `POST /policy/network/check` → request `PolicyCheckRequest{type:"network", target, sandbox_id?}`;
  `type` is required and must be `"network"`, `target` must be non-empty. Response
  `PolicyCheckResponse{action, allowed, context, deny_kind?, governance{active}, origin?, reason?,
  resource_type, resource_value, rule?, target, type}`. Example deny:
  `{"allowed":false,"deny_kind":"implicit","reason":"No matching allow rule (default deny)"}`.
- `GET /policy/network/profiles` → `{"profiles":[]}` on this host, because the `feature.profiles`
  flag is disabled.

### Hidden feature flags (not in `sbx settings list`)

`feature.ssh`, `feature.profiles`, `feature.gordon` read as structured flag objects
(`{"enabled":bool,"variant":"","variantPayload":""}`). `sbx settings set feature.ssh true` still
works — the daemon coerces the bare bool into that object. `feature.ssh` is **enabled by default**
at v0.37.0. There is no local settings key for the shared skills store; its `--no-share-skills`
flag is gated by a remote flag, and `SandboxCreateRequest` carries the `ShareSkills *bool` field.

### `SandboxCreateRequest` (DWARF, v0.37.0)

The full create body the daemon accepts, well beyond what the CLI exposes:
`Agent`, `Workspace` (both required), `AdditionalWorkspaces`, `AgentOptions`, `BindingsPath`,
`Clone`, `Cpus`, `CredentialValues`, `Detached`, `DindVolumeSize`, `Display`,
`EnableVirtiofsCache`, `Environment`, `KitArtifacts`, `Kits`, `Memory`, `Name`, `Profile`,
`PullPolicy`, `RootFilesystemSize`, `SecretsScope`, `ShareSkills`, `Template`.

CLI-side client ops, from the `sandboxapi.New<Op>Request` symbols at v0.37.0
(`go tool nm /usr/bin/sbx | grep -oE 'sandboxapi\.New[A-Za-z]+Request'`):

`AddAllowedPath, AddMcpGatewayServer, ApplyNetworkPolicySetup, CheckMcpRegistration,
CheckNetworkPolicy, CreateSandbox, DeleteSandbox, Exec, GetDaemonHealth, GetDaemonLogLevel,
GetDebugState, GetNetworkLog, GetNetworkPolicySetupStatus, InspectExec, InspectImage,
InspectSandbox, ListImages, ListNetworkPolicyProfiles, ListNetworkPolicyRules,
ListPublishedPorts, ListSandboxes, LoadImage, ModifyNetworkPolicyRules, PublishPorts, PutFile,
ReloadOAuthService, RemoveAllowedPath, RemoveImage, ReplaceDesiredPort, ResetDaemon, ResizeExec,
SaveSandbox, SetDaemonLogLevel, StartMcpGateway, StartSandbox, StopMcpGateway, StopSandbox,
SwapSandboxContainer, SyncCredentials, UnpublishPorts`.

Image/network/volume management is still primarily handled at the engine/`docker.sock` layer
rather than through top-level REST paths. Note that symbol *absence* is not proof an endpoint is
gone — the linker drops unreferenced builders, so probe with `OPTIONS` before concluding
(an unmatched Echo route returns `404` with no `Allow` header; a matched one returns the header).

### Kit spec schema

The `spec.yaml` schema is `github.com/docker/sbx-kits-contrib/spec`, reachable from the `sbx`
binary's DWARF. Two types matter:

- **`SpecFile`** — what a kit author writes. 24 fields, eight named `Legacy*`. `credentials` and
  `volumes` each accept either a list or a legacy map through custom unmarshalers. YAML decoding is
  strict: an unknown key fails validation.
- **`Artifact`** — what `sbx kit inspect --json` reports. 14 fields. Modelled as `kit.Info`, not
  `kit.Artifact`, because it omits the `files/` payload that `kit pack` writes into the ZIP.

The two shapes differ. Flat `template` / `binary` / `runOptions` keys are rejected on input ("use
the 'sandbox:' block instead") yet emitted on output, derived from the `sandbox:` block.

**Method note: DWARF carries no Go struct tags at all** — confirmed empirically (a throwaway
binary's member DIEs expose `Name`, `Type`, `DataMemberLoc`, and one Go-vendor boolean, nothing
tag-shaped), not just taken on faith from `internal/tools/dwarfgen/main.go`'s header comment. So
DWARF can confirm a field's Go name, never its JSON tag; `Manifest.Build`'s `json:"build,omitempty"`
tag was instead confirmed by running `strings` on `/usr/bin/sbx` (a single occurrence, corroborated
by four sibling `BuildConfig` tags matching DWARF field names one-for-one). Don't go looking for
tags in DWARF output — they aren't there.

`sandboxapi.SandboxInfo` has **no** kit field — 14 fields in DWARF, matching `types_gen.go` exactly.
A sandbox's kit list is the container label `com.docker.sandbox.kits`, holding a JSON string array,
returned inside `labels` by `GET /sandbox/{name}`.

The binary also carries `/sandbox/:name/swap-container`, the endpoint behind the `kit add` recreate.
The SDK does not call it: `kit add` re-resolves the recorded kit list and runs the accept/refuse
check around it, and shelling out gets both for free.

---

## 4. Reproduce the recon

```bash
go version -m /usr/bin/sbx                       # module + deps
go tool nm /usr/bin/sbx | grep docker/sandboxes  # symbol map
sbx --help ; sbx <cmd> --help                    # cobra command tree
sbx -D daemon status                             # prints socket + log paths
SOCK=~/.local/state/sandboxes/sandboxes/sandboxd/sandboxd.sock
curl -s --unix-socket "$SOCK" http://localhost/daemon/health
curl -s --unix-socket "$SOCK" http://localhost/daemon/info
curl -s --unix-socket "$SOCK" http://localhost/sandbox
curl -s -X OPTIONS -D- -o/dev/null --unix-socket "$SOCK" http://localhost/sandbox  # Allow: header
curl -s --unix-socket "$SOCK" http://localhost/policy/network/rules
curl -s -X POST -H 'Content-Type: application/json' \
  -d '{"type":"network","target":"api.anthropic.com:443"}' \
  --unix-socket "$SOCK" http://localhost/policy/network/check
```

Route discovery: `OPTIONS` every candidate path and keep the ones that answer with an `Allow`
header (unmatched Echo routes return `404` and no header):

```bash
for p in /policy/network/rules /policy/network/check /policy/network/profiles /daemon/reset; do
  curl -s -X OPTIONS -D- -o/dev/null --unix-socket "$SOCK" "http://localhost$p" | grep -i '^allow:'
done
```
