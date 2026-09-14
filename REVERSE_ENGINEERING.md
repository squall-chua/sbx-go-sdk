# `sbx` CLI — Reverse Engineering Notes

Reverse-engineered from `/usr/bin/sbx` (unstripped Go 1.26.8 binary, with DWARF).

> `docs/sbx-version-coverage.md` is the authority on which release a feature
> shipped in — it is reconciled against upstream release notes and wired into
> the drift gate. If a version marker below ever disagrees with that table,
> the table wins.

- **Module:** `github.com/docker/sandboxes` `v0.42.1`
- **Main package:** `github.com/docker/sandboxes/cli-plugin/cmd/sandboxes`
- **Daemon API version:** `0.28.0` (build `cc6e400a4a3ce3ce5e0b2b77b8ee352aac854c64`)
- **What it is:** Docker Sandboxes — isolated micro-VM sandboxes for AI coding agents.
  Shipped both as a standalone `sbx` binary and as a `docker sandboxes` CLI plugin.
- **Single-binary model (like docker/dockerd):** the same binary is both the CLI
  *and* the `sandboxd` daemon. The CLI re-execs itself to start the daemon.

> Refreshed for **v0.42.1** (daemon api `0.26.0` → `0.28.0`). v0.40.0 and v0.41.0 were never
> released; v0.42.1 is v0.42.0 plus one proxy fix. **The CLI still calls the same 46 REST ops**:
> the `sandboxapi.New<Op>Request` set is identical to v0.39.0's, and re-running `dwarfgen` gives
> only the known artifacts, so no wire type moved. The API bump is server-side — see the new
> handlers at the end of §3's table.
>
> **Most read-only commands gained `--json`**: `mcp ls`, `mcp inspect`, `secret ls`, `skills ls`,
> `ports`, `policy log`, `policy inspect`, `policy profile ls`, `prune --dry-run`, `kit validate`,
> `kit verify`, `kit provenance`, `template inspect`, `volume ls`, `ttl`, `version`, and
> `diagnose` (an alias for `-o json`). The SDK moved every text parser it could onto them. The
> JSON is not the table in new clothes, and the gaps are worth remembering:
>
> - `secret ls --json` spells scopes `global` and `host-only`, where the table said `(global)` and
>   `(host only)`; a sandbox scope is its name in both. `targets` is an array, which the table
>   joined with `", "`. A registry username is its own key, which the table showed as `USER/**`.
>   A resolver has `kind`, `source` and `refresh` in place of `secret`, which the table showed as
>   `kind:source (refresh)`; `refresh` is always set, `on-demand` by default.
> - `mcp ls --json` reports `transport` as `"local stdio"` / `"remote http"`, repeating the type.
>   `mcp inspect --json` gives `command` as an array and puts the issuer and registration endpoint
>   under `oauth_providers[]`. A server registered with `--skip-auth` lists as `needs-auth`.
> - `prune --json` works **only with `--dry-run`**; a real prune refuses it with an error. So the
>   real-prune path still reads the `Sandbox 'X' removed` lines.
>
> Each list key is printed even when the list is empty — a scope with no secrets still gives
> `"secrets": []` — which is what lets a decoder treat a missing key as a format error.
>
> Behaviour changes: a publish that names no protocol is now **`tcp4`**, one IPv4 key, over the
> CLI and REST alike (verified live); it used to be dual-stack `tcp`. Removing or pruning a
> sandbox now **deletes its sandbox-scoped secrets** (verified live on prune). `mcp add
> --skip_auth` is deprecated for `--skip-auth` and prints a warning. `secret rm --placeholder`
> without `--force` prints "Cancelled" on a non-interactive stdin and **exits 0**.
>
> New commands: `attach`, `move`, `ttl`, `volume` (`create`/`inspect`/`ls`/`rm`), `skills
> add`/`rm`/`update` (skills from Git), `template inspect`, `env plan`. New flags on `create` and
> `run`: `--volume`, `--ttl`, `--on-timeout`, `--platform`, `--image-ref`, `--allow-network`,
> `--kit-arg`, `--kit-args-file`, and `run --new`. `create` now accepts no workspace path, and a
> sandbox kit reference as the agent positional. A global `--cloud` / `--cloud-api-url` pair sends
> supported verbs to Docker Cloud Sandboxes. `mcp add --no-scope`. New agent: `devin`. `sbx env`
> reads a non-hidden `sbxenv.yaml` and gained an `args:` block (`--env-arg`).
>
> Settings: two new feature flags, `feature.diagnosticsAutoUpload` and `feature.sandbox-nested`,
> and eight new settings: `diagnostics.autoUpload` plus three `diagnostics.autoUpload*` tuning
> keys, `env.rememberHostCommands`, `sandbox.disk.dockerVolume`, `ssh.agentForwardingEnabled`
> and `ssh.agentSocketPath`. The `feature.sbx-api` family is still unprobed.
>
> Earlier, for **v0.39.0** (daemon api `0.26.0`, **unchanged**). The REST surface did not move:
> the `sandboxapi.New<Op>Request` symbol set is identical to v0.38.0's 46, so every change this
> release is CLI-side. Wire types gained one additive field, `SandboxInfo.StoppedAt` — and the
> daemon does not emit it yet, verified against a stopped sandbox.
>
> New commands: **`sbx env`** (declarative `.sbxenv.yaml` environments — `create`, `run`, `exec`,
> `rm`, deep-merged with docker-compose `-f` semantics); **`sbx prune`**; **kit signing**
> (`kit sign`, `kit verify`, `kit provenance`, and `push --sign`, cosign/Sigstore with a SLSA
> provenance attestation attached as an OCI referrer); `skills ls`; `setup ssh remove`. New flags:
> `--env` / `--env-file` on `create` and `run` (which finally reaches the `Environment` create
> field), external secret resolvers on `secret set` / `set-custom` (`--ref` for 1Password `op://`
> and AWS Secrets Manager ARNs, plus `--command`, `--refresh`, `--no-verify`, `--show-error`,
> `-t/--token`), `reset --preserve-secrets`, `template save -o`, `daemon status --json`.
> `sbx ssh` became hidden — both it and `sbx setup ssh` still work.
>
> Three output changes broke the SDK, and the shape of each is worth remembering.
> **`kit inspect --json` flattened onto the kit spec v2 document**: no `manifest` wrapper, and
> `caps`/`commands`/`agentContext` are now `permissions`/`setup`/`agentInstructions` on output
> too, for v1 and v2 kits alike (a v1 kit gains a deprecation entry in `warnings` per legacy key).
> Nothing is lost, but a lot moves, and the moves are not guessable from the rename list alone:
> the v1 `sandbox:` block's `entrypoint.run` / `.args` / `.ttyArgs` surface as
> `sandbox.entrypoint` / `sandbox.command.default` / `sandbox.command.interactive`,
> `sandbox.aiFilename` surfaces as `agentInstructions.filename`, `resources.memoryMB` becomes a
> unit string (`"2048m"`), and top-level `publishedPorts` is renamed to `ports` on output — while
> the v2 *decoder* rejects `publishedPorts` as an input key outright, so a v2 kit cannot declare
> ports at all. Probe a kind `sandbox` kit as well as a mixin, and probe both schema versions:
> a mixin alone shows none of this. **`sbx mcp ls` stopped being a table** — it is
> now grouped by gateway with a free-form header and footer, prints transport and readiness, and
> no longer carries the URL or command at all. There is still no `--json`, so a header-anchored
> parse fails *silently*: `coltable` reported `ErrNoHeader`, which the SDK read as "no servers".
> A parser anchored on a header must treat a missing header as an error, never as emptiness.
> **`sbx diagnose -o json` exits non-zero when any check fails** and still prints the whole
> report, so an exit-code-first wrapper throws away exactly the diagnosis a broken host needs.
>
> Settings grew a lot: 9 feature flags → 23, 18 non-flag settings → 23. The new
> `feature.sbx-api` family (`sbx-api` plus ten `sbx-api-*` sub-flags, covering events, files,
> images, mcp, netpolicy, process, sandbox and more) looks like a gated public REST API. Probe it
> next sync — if it exposes creation, `sandbox.Create` could stop shelling out. Also new:
> `feature.sandbox-usb` (with `SandboxCreateRequest.UsbDevices`), `feature.network-user-prompts`,
> `update.channel`, and the kit-signing settings `kit.requireSignature`, `kit.trustedSigners`,
> `kit.ignoreTransparencyLog`.
>
> Five routes answer `OPTIONS` but were never in §3's table: `GET /events`,
> `POST|DELETE /sandbox/{name}/mounts`, `POST /sandbox/{name}/kits`, `GET /policy/network/log`,
> `POST /daemon/shutdown`, and `DELETE /sandbox/{name}/mcp/gateway`. None is new — the matching
> op symbols were all present at v0.38.0. They were simply never probed.
>
> Earlier, for **v0.38.0** (daemon api `0.24.0` → `0.26.0`). **No wire-type changes again**: re-running
> `dwarfgen` produced the same 11 types with the same fields; the only diff was the four known
> generator artifacts the header of `internal/api/types_gen.go` already documents, so that file was
> reverted rather than regenerated. The whole existing SDK surface passes `internal/integration`
> at v0.38.0 — with one fixture change, see kit spec v2 below.
>
> New this release: **`sbx mcp`**, a top-level MCP-server registry and gateway (§1), backed by one
> new REST route `GET /mcp/gateway-mode`; `--static-mcp` and `--deny-network` on `create`/`run`;
> `sbx daemon restart`; `settings list --all` (feature flags are no longer hidden) plus
> `requires_restart` / `feature_flag` / `env_var` / `default` fields on a setting.
>
> Two behaviour changes bite a shell-out client. **Secret scope**: global is now the default for
> service and custom secrets, and both `-g` and the bare positional sandbox name are deprecated —
> they still work but print a warning into the output the SDK parses. `--sandbox NAME` replaces
> them. A registry credential's default scope became a *third* scope, `(host only)`, so a bare
> `secret set --registry` no longer means global; `--all-sandboxes` does. **Kit spec v2**:
> `schemaVersion: "2"` got a decoder of its own (`spec.specFileV2`), so v1 key names no longer parse
> under it — `caps` → `permissions`, `commands` → `setup`, `commands.initFiles` → `setup.files`,
> `agentContext` → `agentInstructions`. At v0.38.0 `kit inspect --json` still emitted the
> normalized v1 shape, so `kit.Info` was unaffected — v0.39.0 reversed that, see above.
> Also fixed upstream: a `sbx cp` copy-out destination escape
> (CVE-2026-17106) — the SDK's own extractor (`internal/untar`, `os.Root`-confined) was never
> affected.
>
> Earlier, for **v0.37.0** (daemon api `0.22.0` → `0.24.0`). **No wire-type changes**: re-running
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
    create claude|codex|copilot|cursor|devin|docker-agent(cagent)|droid|gemini|kiro|opencode|shell
      (devin is NEW in v0.42.0; see the v0.42.1 refresh note above for the v0.42.0 flags)
      flags: --clone --cpus --deny-network --env/-e --env-file --kit --memory/-m --name
             --profile --publish/-p --quiet/-q --static-mcp --template/-t
      (--env/-e and --env-file are NEW in v0.39.0; --env takes KEY=VALUE, or a bare
       KEY to inherit the caller's value. This is what finally reaches the
       SandboxCreateRequest.Environment field)
      (--publish/-p is NEW in v0.37.0; --deny-network and --static-mcp are NEW in v0.38.0;
       --no-share-skills exists but is gated off by a remote feature flag, so it is absent
       from --help on this host)
sbx diagnose [-o json|github-issue] [--upload]  # diagnose install issues (-o/--upload NEW in v0.38.0)
    (exits NON-ZERO when any check fails, and still prints the whole report)
sbx env COMMAND                               # NEW in v0.39.0 (experimental) .sbxenv.yaml environments
    env create [PATH...] [--clone] | run [PATH...] [--clone] [-d/--detached]
        | exec [flags] [PATH...] -- COMMAND [ARG...] | rm [PATH...] [-f] [--prune-bindings]
    (each PATH is a directory or the file itself; several deep-merge in order,
     docker-compose `-f` semantics. Values expand ${VAR} / $VAR / ${VAR:-default})
sbx exec [flags] SANDBOX COMMAND [ARG...]     # exec a command in a sandbox
sbx kit COMMAND                               # (experimental) kit artifacts
    kit add SANDBOX REFERENCE | inspect REFERENCE | pack DIR | pull REFERENCE
        | push DIR REFERENCE [--sign] [--key PEM] [--identity-token[-file] T] [--tlog-upload]
        | validate REFERENCE
    kit sign REFERENCE [--key PEM] [--identity-token[-file] T] [--tlog-upload]   # NEW in v0.39.0
    kit verify REFERENCE [--key PEM] [--certificate-identity[-regexp] I]         # NEW in v0.39.0
        [--certificate-oidc-issuer[-regexp] U] [--insecure-ignore-tlog]
    kit provenance REFERENCE [same verification flags as verify]                 # NEW in v0.39.0
    (signing is keyless via Fulcio+Rekor unless --key; a local directory gets a
     kit.sig.bundle sidecar, an OCI reference gets an OCI referrer. Every push
     also attaches a SLSA provenance attestation, unsigned unless --sign)
sbx login [flags]                             # sign in to Docker
sbx logout [flags]                            # stop sandboxes + sign out
sbx ls [flags]                                # list sandboxes
sbx mcp COMMAND                               # NEW in v0.38.0 — register/manage MCP servers
    mcp add NAME (--url URL | --command CMD [--args a,b] [--dir D])
        [--local] [--scope S]... [--client-id ID] [--oauth-authorization-server PATH|URL]
        [--skip-ssrf-check] [--skip-auth] [--no-scope]
        (--skip-auth and --no-scope are NEW in v0.42.0; --skip_auth still works, deprecated)
    mcp ls | inspect NAME | rm NAME            # --json on ls and inspect is NEW in v0.42.0
    (v0.39.0 reshaped `mcp ls`: no more NAME/TYPE/URL-COMMAND header. It now
     groups by gateway under a free-form header line, indents each server as
     `NAME  local|remote  stdio|http  ✓ ready`, and closes with an
     `N servers · …` footer. The URL/command is gone from the listing entirely —
     only `mcp inspect` still prints it. Columns are separated by a single
     space in places, so a two-space gutter rule does not split them.)
    mcp load NAME --sandbox SANDBOX            # attach to a RUNNING sandbox's gateway
    mcp auth [NAME|--all] [--scope S]... [--format text|json] [--verbose]
    mcp auth status [NAME|--all] [--format text|json] | auth rm [NAME|--all]
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
sbx prune [--dry-run] [--filter since=DURATION] [-f]   # NEW in v0.39.0 — remove stopped sandboxes
sbx reset [-f] [--preserve-secrets]           # reset all sandboxes + clean state
sbx rm [SANDBOX...] [--all] [-f/--force]       # remove sandboxes; --all is NEW in v0.37.0
sbx run [flags] SANDBOX | AGENT [PATH...] [-- AGENT_ARGS...]   # run/attach an agent
      flags: as create, plus --publish/-p (NEW) and a hidden --detached/-d
      (--detached prints the sandbox ID and exits without an interactive session)
sbx secret COMMAND                            # manage stored secrets
    secret ls [-g|--sandbox S] [--service SVC] | rm [SERVICE] [--sandbox S]
        | rm --placeholder PH | rm [--all-sandboxes] --registry REF
    secret set [SERVICE] [--sandbox S] [--oauth]
        [--ref op://…|arn:…] [--command CMD] [--refresh on-demand|DURATION]
        [--no-verify] [--show-error] [-t/--token V] [-f/--force]     # all NEW in v0.39.0
        | set [--all-sandboxes|--sandbox S] --registry HOST --password-stdin [--username U]
        | set-custom [--sandbox S] --host H... --env E --value V [--placeholder P]
    secret import [SERVICE] [--all] [--dry-run] [-f]   # NEW in v0.35.0
    (the positional SERVICE and --all are not new in v0.39.0 — the v0.37.0
     recon just never recorded them. The SDK has emitted both since then.)
    (v0.38.0 reshaped scope: global is the default for service and custom secrets,
     `-g` and a bare positional SANDBOX are deprecated-but-working and print a warning,
     and a registry credential's default is a third scope, "(host only)" — host-side
     pulls only, injected into no sandbox. `--all-sandboxes --registry` is the old
     global meaning.)
sbx setup                                     # (experimental) detect host config + prepare sbx
    setup ssh [--alias PATTERN]                 # NEW path in v0.37.0 for `sbx ssh setup` (both still work)
    setup ssh remove                            # NEW in v0.39.0 — undo what `setup ssh` wrote
sbx skills COMMAND                            # NEW in v0.37.0 (experimental) shared agent skills store
    skills import [--dry-run] [-f/--force]      # copy host skill dirs into the shared store
    skills ls                                   # NEW in v0.39.0 — list the store's skill folders
sbx ssh [flags]                               # HIDDEN again in v0.39.0 (still registered, still works)
    ssh setup [--alias PATTERN]                 # hidden alias of `sbx setup ssh`; same flags
sbx stop SANDBOX [SANDBOX...]                  # stop without removing
sbx template COMMAND                          # manage sandbox templates
    template load FILE | ls [--json] | rm TAG|ID | save SANDBOX TAG [-o FILE]
        (-o on save, which also exports the image to a tar, is NEW in v0.39.0)
sbx version                                   # version info
sbx completion bash|zsh|fish|powershell
```

### Hidden commands (not in `--help`, but registered)
```
sbx daemon                                    # manage the sandboxd daemon
    daemon start [-d/--detach] [--policy allow-all|balanced|deny-all]
    daemon status [--json]                      # --json NEW in v0.39.0
    daemon stop
    daemon restart                              # NEW in v0.38.0
    daemon log-level [set <proxy|general|all> <level>]
sbx inspect SANDBOX [--json]                  # human-readable sandbox summary
sbx settings                                  # persistent settings (JSON, hot-reloaded ~5s)
    settings get [--json] | list [--all] [--json] [--no-trunc] | set | unset
    (--all is NEW in v0.38.0 and is the only way to list feature flags)
```
`credentials*` symbols back the user-facing `secret` command. `mcp*` symbols were
internal-only through v0.37.0; v0.38.0 promoted them to the visible top-level `sbx mcp`
command. `save*` symbols remain internal (subcommands of `template`/`run`).

Agents supported by `create`/`run`: **claude, codex, copilot, cursor, devin, docker-agent
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
| GET  | `/daemon/diagnostics` | daemon self-check report `{info, socket_paths}` — see below |
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
| POST | `/policy/refresh` | re-fetch remote governance policy (`Allow: OPTIONS, POST`) |
| GET  | `/daemon/settings` | all settings → `{"settings":[…]}`; the JSON behind `sbx settings list` |
| GET  | `/mcp/gateway-mode` | **NEW in v0.38.0** — which MCP gateway sandboxes get: `{decision, gateway_url, reason}` |
| POST | `/daemon/reset` | reset all sandboxes + daemon state (`sbx reset`) |
| POST | `/daemon/shutdown` | stop the daemon (`Client.StopDaemon`) |
| GET  | `/events` | daemon event stream (`sandboxapi.Event`, `StreamEventsParams`) |
| GET  | `/policy/network/log` | network policy log (`sbx policy log`) |
| POST | `/sandbox/{name}/kits` | add a kit to a sandbox (`sbx kit add`) |
| POST\|DELETE | `/sandbox/{name}/mounts` | add/remove an allowed path (`AddAllowedPath`, `RemoveAllowedPath`) |
| DELETE | `/sandbox/{name}/mcp/gateway` | tear down a sandbox's MCP gateway |

The last six were first mapped at v0.39.0 but are **not new** — every matching op symbol was
already present at v0.38.0. They had simply never been probed. The SDK calls none of them.

At v0.42.1 the server registers 61 handlers
(`go tool nm /usr/bin/sbx | grep -oE 'sandboxapi\.\(\*ServerInterfaceWrapper\)\.[A-Za-z]+'`).
Eight have no row above: `AttachAgentSession`, `AttachSSH`, `SessionHold`, `ExportImage`,
`PullImage`, `GetMcpGateway`, `StreamUserPrompts` and `RespondToUserPrompt`. Their paths were
not probed, and the handler list was never recorded before, so which of them is new in v0.42.0
is not known. The CLI calls none of them — its op set did not change.

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

**Live-verified at v0.38.0:**

- `GET /mcp/gateway-mode` → `200`, `Allow: OPTIONS, GET`. On a host with no SaaS entitlement:
  `{"decision":"local","gateway_url":"none","reason":"not entitled to the SaaS gateway → local"}`.
  The `mcp.forceLocalGateway` setting pins it to `local`.
- There is **no** REST surface for the MCP registry itself: `/mcp` and `/mcp/servers` both `404`.
  The registrations are per-server JSON files under
  `…/sandboxd/mcp/servers/<name>.json` (with `…/sandboxd/mcp/gateways/` alongside), written by the
  CLI. The SDK's `mcp` package therefore shells out and parses `mcp ls` / `mcp inspect`, which have
  no `--json` flag. `mcp auth status` does take `--format json`.
- `sbx mcp rm <unregistered-name>` exits **0** — a silent no-op, not an error. `sbx mcp inspect
  <unregistered-name>` exits 1.
- The `?include_inactive=true` question on `GET /policy/network/rules` is still unresolved: the
  response is byte-identical with and without it.

### Gap-closing probes at v0.38.0

Four long-standing "gaps" turned out to be closable once probed properly. Recorded so the next
sync does not re-derive them:

- **`sbx inspect --json`** is a much wider view than `GET /sandbox/{name}`. Its Go shape is
  `commands.inspectView`: `Name, Agent, Kits, State, Running, Uptime, Image, ImageDigest,
  AuthMode, Workspace, Network, NetworkPolicy{Scope, Organization, OrganizationUnavailable},
  Proxy, MountPolicyDenied, Secrets[]{Name, Source}, MCPGateway, Ports, Sessions, DaemonVersion,
  DaemonUptime`. Four of those — auth mode, secrets, session count, MCP-gateway state — exist
  **nowhere** in `sandboxapi.SandboxInfo`. `Ports` here is pre-formatted text
  (`"127.0.0.1:44624->8080/tcp"`), not structured. `auth_mode`, `ports`, `kits`, `secrets` and
  `mount_policy_denied` are all omitted when empty or false, so absence is normal.
- **`sbx diagnose -o json`** (v0.38.0) emits `{version, checks[]{name,status,message,detail,hint},
  summary{pass,warn,fail,skip}}` — nine checks on this host, `status` in
  pass/warn/fail/skip. Entirely separate from `GET /daemon/diagnostics`.
- **`--no-share-skills` was never actually gated off.** The v0.37.0 recon recorded it as
  unusable because it is absent from `sbx create --help`. That is only cobra hiding it: setting
  `feature.shareSkills` true makes it appear in `--help`, and with the feature *off* the flag
  still parses and the create still succeeds on both `create` and `run` (a bogus flag exits 1 as
  the control). Hidden ≠ unregistered — don't infer a flag's absence from `--help` again.
- **`sbx mcp auth status --format json`** returns `[]commands.mcpAuthResult`, confirmed by
  registering a real OAuth server with `--skip_auth` — which records the registration *without*
  running the browser flow, and is the trick for probing any OAuth-shaped output:
  `[{"server_name":"…","status":"unauthorized"}]`. `auth rm --format json` returns the same
  shape. An OAuth server's `mcp inspect` also gains indented `Issuer:` and `Registration:` lines
  under `OAuth: required`.
- **`policy profile ls` is still empty with `feature.profiles` on.** Toggling the flag changes
  nothing: `GET /policy/network/profiles` stays `{"profiles":[]}`. Profiles come from remote
  governance, so this needs a governed org, not a local setting. **But an empty response is not an
  unknown shape**: `sandboxapi.PolicyProfilesListResponse` is a struct with a single
  `Profiles []string` field, so a profile is a *name* and there is no richer per-profile record to
  model. DWARF answered what a live host could not.
- **`sbx login` has a non-interactive path** — `--username U --password-stdin` — so only the
  bare browser-OAuth form is unwrappable. `sbx logout --yes` likewise.

### The interactive commands all degrade without a TTY

Recorded because three separate "unclosable, it needs a browser/terminal" conclusions were wrong.
None of these commands has a non-interactive *flag*; each detects the absent terminal and takes a
different path by itself. Run a command under `</dev/null` before writing it off.

- **`sbx setup </dev/null`** prints a read-only detection report — sections `PREREQUISITES`,
  `SECRETS`, `SKILLS`, `GOVERNANCE`, `MCP`, each row `name / detail / status` at two-space
  gutters — closes with `(non-interactive: showing detected configuration only)`, changes nothing,
  and exits **0**. The `MCP` row counts servers found in the *host's* agent config that setup
  would offer to import, which is a different set from `sbx mcp ls`.
- **`sbx mcp auth NAME </dev/null`** prints `Open this URL to authorize MCP server "…":` and the
  authorization URL on **stdout**, then blocks on a loopback callback listener
  (`redirect_uri=http://127.0.0.1:<port>/callback`) until consent lands. Interrupting it exits
  non-zero with `context canceled`.
- **`sbx secret set SERVICE --oauth </dev/null`** does the same thing on **stderr**
  (`Open this URL to sign in to Codex OAuth:`, `redirect_uri=http://localhost:1455/auth/callback`).

The two OAuth commands are what `internal/oauthflow` wraps: scan both streams for the first
`https://` URL, hand it to the caller, block until the child exits. One caveat that belongs to the
SDK rather than upstream — scanning means passing custom writers, so `exec` uses pipes instead of
the inherited `*os.File`s, and a grandchild holding a pipe open would delay the return until
`cli.Runner`'s 10s kill backstop. These commands leave no such grandchild.

`sbx setup`'s wizard half is genuinely a bubbletea TUI (`commands.setupModel` with `Init`/`Update`/
`View`), so only the detection path is reachable.
### `GET /daemon/diagnostics`

Probed at v0.37.0: `200 application/json`, `Allow: OPTIONS, GET`. Two top-level keys, `info` and
`socket_paths`. `info` is a large nested object whose keys are **Go field names in PascalCase**,
not the snake_case the rest of the API uses: `SchemaVersion`, `CapturedAt`, `Version`, `GoRuntime`,
`NerdboxID`, `Host`, `Process`, `State`, `ContainerdConfig`, `Goroutines`. The SDK's
`client.Diagnostics` hands it back as raw JSON rather than modelling it; decode the fields you
need.

This is the daemon's own self-check. It is **not** what `sbx diagnose` prints — that is a separate
CLI-side install checker, which the SDK does not wrap.

Which release the route first appeared in was not established; it was simply never recorded in the
table above until now.

### Feature flags (hidden from `sbx settings list` until v0.38.0's `--all`)

Feature flags read as structured objects (`{"enabled":bool,"variant":"","variantPayload":""}`).
`sbx settings set feature.ssh true` still works — the daemon coerces the bare bool into that
object. `feature.ssh` is **enabled by default** since v0.37.0.

Through v0.37.0 the flags were invisible to `settings list` and reachable only one at a time via
`settings get`. **v0.38.0 added `settings list --all`**, which lists all nine on this host:
`feature.claude-bedrock`, `feature.claude-vertex`, `feature.gordon`, `feature.model`,
`feature.profiles`, `feature.sandbox-display`, `feature.sandbox-gpu`, `feature.shareSkills`,
`feature.ssh`. `feature.shareSkills` is the remote flag gating `--no-share-skills`;
`SandboxCreateRequest` carries the matching `ShareSkills *bool` field. Note `--all` changes the
**table** and the `--json` array alike, and a flag's `source` can be `remote`.

**v0.39.0: 23 flags and 23 non-flag settings.** New flags: `feature.sandbox-usb` (paired with
`SandboxCreateRequest.UsbDevices`), `feature.network-user-prompts`, `update.channel`, and the
`feature.sbx-api` family — `sbx-api` plus `sbx-api-events`, `-files`, `-images`, `-localadmin`,
`-localops`, `-mcp`, `-netpolicy`, `-operations`, `-process`, `-sandbox`. That family reads like
a gated public REST API; probing what each unlocks is the first job of the next sync, because a
REST creation path would let `sandbox.Create` stop shelling out. New non-flag settings:
`claude.remoteControl`, `platform.images.registryMirror`, and the three kit-signing settings
`kit.requireSignature`, `kit.trustedSigners`, `kit.ignoreTransparencyLog`.

The 18 non-flag settings at v0.38.0: `clipboard.imagePaste`, `kit.allowLocalKits`,
`kit.allowedSources`, `mcp.forceLocalGateway`, `no_proxy`, `no_proxy.daemon`, `no_proxy.sandbox`,
`platform.allowExperimentalFeatures`, `platform.images.useDHI`, `proxy`, `proxy.daemon`,
`proxy.integratedAuth`, `proxy.sandbox`, `ssh.autoCreate`, `ssh.defaultAgent`,
`ssh.defaultTemplate`, `ssh.workspaceRoot`, `tls.allowNegativeSerial`.

A setting object gained four fields in v0.38.0: `default`, `env_var`, `feature_flag` and
`requires_restart`. The last is the `RESTART` column of the table, and `sbx daemon restart` (also
new) is what applies such a change.

### `SandboxCreateRequest` (DWARF, v0.39.0)

The full create body the daemon accepts, well beyond what the CLI exposes:
`Agent`, `Workspace` (both required), `AdditionalWorkspaces`, `AgentOptions`, `BindingsPath`,
`Clone`, `ClonedWorkspaceSize`, `Cpus`, `CredentialValues`, `Detached`, `DindVolumeSize`,
`Display`, `EnableVirtiofsCache`, `Environment`, `Gpu`, `KitArtifacts`, `Kits`, `Memory`, `Name`,
`Profile`, `PullPolicy`, `RootFilesystemSize`, `SecretsScope`, `ShareSkills`, `Template`,
`UsbDevices`.

`UsbDevices` is new in v0.39.0 and pairs with the `feature.sandbox-usb` flag; no CLI flag reaches
it. `ClonedWorkspaceSize` and `Gpu` were new in v0.38.0 (`Gpu` pairs with `feature.sandbox-gpu`).
`Environment` became reachable in v0.39.0 through `create --env` / `--env-file`; the other
CLI-invisible fields are unchanged. Neither `--static-mcp` nor `--deny-network` appears here:
both are applied by the CLI through separate calls after create, not carried in the create body.

CLI-side client ops, from the `sandboxapi.New<Op>Request` symbols — **byte-identical at v0.39.0
and again at v0.42.1**
(`go tool nm /usr/bin/sbx | grep -oE 'sandboxapi\.New[A-Za-z]+Request'`):

`AddAllowedPath, AddMcpGatewayServer, ApplyNetworkPolicySetup, CheckMcpRegistration,
CheckNetworkPolicy, CreateSandbox, DeleteDaemonSetting, DeleteSandbox, Exec, GetDaemonHealth,
GetDaemonLogLevel, GetDaemonSetting, GetDaemonSettings, GetDebugState, GetMcpGatewayMode,
GetNetworkLog, GetNetworkPolicySetupStatus, InspectExec, InspectImage,
InspectSandbox, ListImages, ListNetworkPolicyProfiles, ListNetworkPolicyRules,
ListPublishedPorts, ListSandboxes, LoadImage, ModifyNetworkPolicyRules, PublishPorts, PutFile,
RefreshPolicy, ReloadOAuthService, RemoveAllowedPath, RemoveImage, ReplaceDesiredPort,
ResetDaemon, ResizeExec, SaveSandbox, SetDaemonLogLevel, SetDaemonSetting, StartMcpGateway,
StartSandbox, StopMcpGateway, StopSandbox, SwapSandboxContainer, SyncCredentials,
UnpublishPorts`.

No symbol changed between v0.38.0 and v0.39.0 — the diff is empty, which is the single
strongest signal that v0.39.0 is a CLI-only release. Six were new at v0.38.0:
`GetMcpGatewayMode`, `RefreshPolicy`, and the four `*DaemonSetting(s)` ops. The MCP *registry* ops (`AddMcpGatewayServer`, `CheckMcpRegistration`,
`StartMcpGateway`, `StopMcpGateway`) were already present at v0.37.0 while `sbx mcp` was still
internal — their presence never meant the command existed.

Image/network/volume management is still primarily handled at the engine/`docker.sock` layer
rather than through top-level REST paths. Note that symbol *absence* is not proof an endpoint is
gone — the linker drops unreferenced builders, so probe with `OPTIONS` before concluding
(an unmatched Echo route returns `404` with no `Allow` header; a matched one returns the header).

### Kit spec schema

The `spec.yaml` schema is `github.com/docker/sbx-kits-contrib/spec`, reachable from the `sbx`
binary's DWARF. Two types matter:

- **`SpecFile`** — what a `schemaVersion: "1"` kit author writes. 24 fields, eight named `Legacy*`.
  `credentials` and `volumes` each accept either a list or a legacy map through custom
  unmarshalers. YAML decoding is strict: an unknown key fails validation.
- **`specFileV2`** — **NEW in v0.38.0.** `schemaVersion: "2"` used to decode through `SpecFile`;
  it now has a decoder of its own, and the v1 key names no longer parse under it (validation
  fails with e.g. `field caps not found in type spec.specFileV2`). 21 fields, no `Legacy*`. The
  renames a kit author must apply: `caps` → `permissions` (same `network.allow` / `network.deny`
  underneath), `commands` → `setup` (`install`, `startup`, and `initFiles` → `files`),
  `agentContext` → `agentInstructions` (now a block: `filename`, `content`). New: a top-level
  `security` block and `sandbox.resources`. Gone from the top level: `egress`, `secrets`,
  `network`, `mixins`-adjacent legacy keys.
- **`Artifact`** — what `sbx kit inspect --json` reports. 14 fields. Modelled as `kit.Info`, not
  `kit.Artifact`, because it omits the `files/` payload that `kit pack` writes into the ZIP.

The two shapes differ. Flat `template` / `binary` / `runOptions` keys are rejected on input ("use
the 'sandbox:' block instead") yet emitted on output, derived from the `sandbox:` block.

**v0.39.0 reversed the normalization direction.** Through v0.38.0 `kit inspect --json`
normalized v2 *back* to the v1 output shape — a spec written with `permissions:` was reported
under `caps:`, inside a `manifest` wrapper. It now emits the v2 document itself: flat, with the
v2 key names, for v1 and v2 kits alike. A v1 kit is normalized *up*, keeping its own
`schemaVersion: "1"` and gaining one `warnings` entry per legacy key, e.g.
`deprecated field "caps": use 'permissions:' block instead (kit-spec v2)`. So `kit.Info` follows
the v2 shape and `kit.Manifest` is gone. Verified at v0.39.0 against both a v1 and a v2 fixture,
of kind `mixin` and kind `sandbox`.

Two shapes only the `sandbox` kind emits: a `sandbox` block (`image`, `build`, `entrypoint`,
`command` — expanded on output into `{default, interactive}` — and `resources`, whose `memory`
is normalized to a unit string such as `"2048m"`), and the `security` block (`privileged`).

`volumes` parses under both schema versions. `publishedPorts` parses only under v1, and is
emitted as `ports` — the input key and the output key differ, which is easy to miss because a
v2 kit cannot produce the field at all to compare against.

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
# walk it: for each "Available Commands" block, recurse. Hidden commands
# (daemon, inspect, settings, ssh) never appear — ask for them by name.
sbx -D daemon status                             # prints socket + log paths
SOCK=~/.local/state/sandboxes/sandboxes/sandboxd/sandboxd.sock
curl -s --unix-socket "$SOCK" http://localhost/daemon/health
curl -s --unix-socket "$SOCK" http://localhost/daemon/info
curl -s --unix-socket "$SOCK" http://localhost/daemon/diagnostics
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
