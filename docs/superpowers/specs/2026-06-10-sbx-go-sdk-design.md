# sbx-go-sdk — Design Spec

**Date:** 2026-06-10
**Status:** Approved (design); open questions deferred to spec-resolution phase
**Goal:** A Go SDK to automate Docker Sandboxes (`sbx`) agent sandboxes — create, run agents,
exec, attach interactively, copy files, and manage the daemon — against a local `sandboxd`.

---

## 1. Background & grounding

Reverse-engineered from `/usr/bin/sbx` (unstripped Go 1.26 binary, DWARF present).
Full notes in [`REVERSE_ENGINEERING.md`](../../../REVERSE_ENGINEERING.md).

- Module reverse-engineered: `github.com/docker/sandboxes` v0.32.0; daemon API version `0.10.0`.
- `sbx` is a single binary that is both the CLI and the `sandboxd` daemon (docker/dockerd model).
- **CLI ⇄ daemon = REST/HTTP over a unix socket** (oapi-codegen + Echo router). Not gRPC.
- **daemon ⇄ engine** uses Connect RPC + a containerd fork; `sandboxd` also exposes a
  Docker-compatible `docker.sock`. These lower layers are out of scope for the SDK.
- Attach/exec use a **hijacked HTTP connection carrying a Docker multiplexed stdcopy stream**
  (raw when a TTY is allocated). Server uses `stdcopymux.StdCopy` and embeds a `moby/moby/client`.
- No OpenAPI spec is embedded in the binary (stripped), so the SDK is **hand-written**, with
  request/response structs extracted from **DWARF** and validated against the live daemon.

### Socket resolution (replicates `sandboxlib.DefaultSocketPath`)
- Default: `$XDG_STATE_HOME/sandboxes/sandboxes/sandboxd/sandboxd.sock`
  (observed: `~/.local/state/sandboxes/sandboxes/sandboxd/sandboxd.sock`).
- Short-symlink fallback under `~/.sbx/run/` to avoid the 108-byte unix path limit
  (`ErrSocketPathTooLong` / `ShortStateDirSymlink`).
- Override precedence: `WithSocketPath` option > env var **`DOCKER_SANDBOXES_API`** (resolved from
  `sandboxlib.SocketPath`'s `Getenv`) > default XDG path.

---

## 2. Scope

**In scope (v1):**
- Sandbox lifecycle: create / list / inspect / start / stop / remove.
- Exec (non-interactive run + capture) and interactive exec/attach (streaming + TTY resize).
- Files (cp in/out), ports, secrets/credentials, network policy, templates (save/load/ls/rm).
- Full daemon lifecycle: locate / ensure-running / start / stop / status / log-level / health / version / info.

**Out of scope (v1, future extensions):** kit artifacts, interactive TUI, login/logout (Docker cloud
auth), the engine `docker.sock` / Connect-RPC layer, MCP wiring.

**Non-goals:** wrapping the cloud gateway API; reimplementing the daemon.

---

## 3. Chosen approach

Hand-written, layered client following the **`docker/go-sdk` design pattern** (functional options,
per-resource packages, `Run()`/lifecycle/object-methods, low-level client exposed for advanced use),
as a **single Go module with per-resource packages** (not a multi-module workspace — YAGNI for a
single-consumer automation SDK).

The transport is **hybrid** (see [ADR-0001](../../adr/0001-hybrid-cli-shellout-plus-rest.md)):
the SDK **shells out to the `sbx` binary** for orchestration-heavy operations that have no REST
client path — `Create`, agent `Run`, `template save` — and uses the **daemon REST API** for
everything that does (list/inspect/start/stop/remove, exec, ports, images, cp/attach via their
hijack endpoints, secrets, daemon lifecycle). This was forced by reverse-engineering: the REST
client ships no `CreateSandbox` builder; the CLI orchestrates creation client-side.

Rejected alternatives:
- **Reimplement create orchestration** over the engine layer — large, fragile, re-tracks a moving
  internal target.
- **Depend on REST `POST /sandbox`** — unused by the CLI, incomplete, returns `"not implemented"`.
- **Shell out for everything** — loses typed structs/streaming/structured errors for runtime ops.
- **oapi-codegen from a reconstructed spec** — the spec doesn't exist; authoring it is more work
  than writing structs, and the generated client is bypassed for hijack/file-tar/lifecycle anyway.
- **Multi-module workspace** — full docker/go-sdk fidelity, but unjustified ceremony here.

> Domain vocabulary is fixed in [`CONTEXT.md`](../../../CONTEXT.md). Crucially, **`Run` means
> "launch + interactively attach to the agent"** (matching `sbx run`), **not** docker/go-sdk's
> "create + start". `Create` provisions without attaching; `Start`/`Stop` are VM lifecycle.

---

## 4. Package layout

Module path: `github.com/mwchua/sbx-go-sdk` (placeholder — confirm before `go mod init`).

```
sbx-go-sdk/
├── internal/tools/dwarfgen/  # one-shot: dump sandboxapi.* structs from the binary's DWARF (Q14)
├── internal/transport/   # unix-socket http.Client; connection-hijack helper for attach
├── internal/api/         # low-level typed REST: DWARF-extracted structs + 1:1 route calls
├── internal/cli/         # `sbx`-binary driver: locate binary, run create/run/save, parse output
├── client/               # Client (connection + daemon lifecycle); New(); DefaultClient; options
├── sandbox/              # core resource: Create + Sandbox object; lifecycle/feature files
├── exec/                 # ProcessOption, ExecResult, AttachSession (shared exec types)
├── cp/                   # cp.Option (WithFollowSymlinks) for file copy
├── secret/               # secrets (minimal; shell-out; experimental upstream)
├── policy/               # network egress policy (REST)
└── template/             # template = image: list/remove/load (REST), save (shell-out)
```

**Layering.** `internal/api` is the low-level typed REST client (our equivalent of the moby client
that `docker/go-sdk` wraps); `internal/cli` is the `sbx`-binary driver for orchestrated ops.
`client` + resource packages are the high-level layer; `client.Client` exposes the low-level REST
surface via an `API()` accessor for advanced use. Resource methods route to whichever driver fits
(per ADR-0001).

**Sandbox identity:** name-primary. Sandboxes are addressed by name (unique per daemon, matches
`/sandbox/{name}`); the `Sandbox` handle exposes `Name()` and `ID()` (id informational, from Inspect).

**Resource packages depend on `client`**, defaulting to `client.DefaultClient` unless
`WithClient(cli)` is passed — mirroring `container.Run(ctx, …)`.

**Per-package file conventions** (from docker/go-sdk): `definition.go`, `options.go`,
`lifecycle.create.go`/`lifecycle.start.go`/`lifecycle.stop.go`/`lifecycle.terminate.go`,
`<type>.go`, `<type>.run.go`, `<type>.exec.go`, `<type>.files.go`, `ports.go`, `inspect.go`.

---

## 5. `client` package — connection + daemon lifecycle

```go
cli, err := client.New(ctx,
    client.WithSocketPath("/custom/sandboxd.sock"), // default: resolved XDG path
    client.WithBinaryPath("/usr/bin/sbx"),          // default: look up "sbx" on PATH
    client.WithAutoStart(),       // EnsureRunning before first call
    client.WithStrictVersion(),   // hard-fail on incompatible; default is warn+proceed
    client.WithHTTPTimeout(30*time.Second),
)
// or the lazy default:
cli := client.DefaultClient
```

**Version policy** (lenient by default, strict opt-in): on connect the SDK declares its client
version and calls `POST /version`. On `incompatible`/`unknown` it logs a warning and proceeds;
`WithStrictVersion()` makes it return `ErrIncompatibleVersion`. Tested range: `sbx` v0.32.0 /
daemon api 0.10.0; a contract test warns on drift.

| Method | Route | Notes |
|---|---|---|
| `Health(ctx)` | `GET /health` | `{release,status,version}` |
| `CheckVersion(ctx)` | `POST /version` | result: `compatible`/`incompatible`/`unknown` |
| `Info(ctx)` | `GET /daemon/info` | `{api_socket, docker_socket}` |
| `DaemonHealth(ctx)` | `GET /daemon/health` | api_version, revision, release |
| `LogLevels(ctx)` | `GET /daemon/loglevel` | `{general, proxy}` |
| `SetLogLevel(ctx, category, level)` | `POST /daemon/loglevel/set` | category: proxy/general/all |
| `Diagnostics(ctx)` | `GET /daemon/diagnostics` | daemon self-check |
| `EnsureRunning(ctx)` | locate socket + `Health`; **shell-out** spawn if down | idempotent |
| `StartDaemon(ctx, opts)` | **shell-out** `sbx daemon start --detach …` | `--policy` passthrough |
| `StopDaemon(ctx)` | REST `POST /daemon/shutdown` (`ShutdownDaemon`) | |
| `Reset(ctx)` | REST `POST /daemon/reset` (`ResetDaemon`) | |
| `DaemonStatus(ctx)` | socket probe + health | returns running + paths |

Only `EnsureRunning`/`StartDaemon` shell out (you can't start a process via REST); stop/reset are
REST. The `sbx` binary is resolved from PATH or `WithBinaryPath`. The SDK does **not** re-exec
arbitrary binaries beyond the documented `sbx` calls.

### 5.1 Shell-out driver (`internal/cli`)

Backs `Create`, agent `Run`, `template save`, and daemon `start`. Design (grilled D1–D8):

- **Binary discovery (D7):** `exec.LookPath("sbx")` or `client.WithBinaryPath`; resolved once and
  cached on `Client`; `ErrBinaryNotFound` if absent.
- **Identity contract (D1) — SDK owns the name:** the driver always passes an explicit `--name`
  and never parses `sbx create` output. Then `Get(name)` hydrates the handle deterministically.
- **Name generation (D2):** `name = WithName` (else `<agent>-<sanitize(basename(primaryWorkspace))>`),
  sanitized to `[A-Za-z0-9.+-]`, collision-resolved against `sandbox.List` by appending `-N`
  (replicates the CLI). Explicit `WithName` that collides → `ErrSandboxExists`.
- **Arg/injection safety (D8):** `exec.Command` (no shell, no interpolation). Workspaces resolved to
  absolute (always `/…`, never mistaken for flags); agent args passed after `--` for `Run`; name
  charset validated.
- **Env (D3):** inherit `os.Environ()` + `WithEnv` additions (`sbx` needs `HOME`/`PATH`/`DOCKER_*`).
- **Cancellation (D4):** `exec.CommandContext` with `Cmd.Cancel` → SIGTERM and `Cmd.WaitDelay` →
  SIGKILL escalation.
- **Output (D5):** non-interactive ops capture stdout+stderr; optional `WithProgressWriter` streams
  `sbx create` progress (image pulls) live; non-zero exit → `CLIError{Args, ExitCode, Stderr}`.
- **TTY (D6) — inherit-only in v1:** `Run` inherits the caller's terminal (`WithStdio` may redirect);
  **no PTY allocation**; non-TTY stdin → clear error (mirrors sbx's `"not a terminal"` guard).

---

## 6. `sandbox` package — core resource

Verb model (domain-faithful to sbx; see `CONTEXT.md`):
- `sandbox.Create(ctx, …)` → **provision without attaching** (sbx `create`). **Shell-out** (ADR-0001).
- `sb.Run(ctx, opts…) (int, error)` → **launch + interactively attach the agent** (sbx `run`),
  create-if-missing. **Shell-out**; stdio inherits the caller's terminal by default
  (`sandbox.WithStdio(in,out,err)` to override), blocks until the agent exits, returns its exit
  code. It does **not** return an `AttachSession` — that mold fits only the hijack-backed
  `ExecInteractive`, not a PTY/child-process session (Q8). A package-level one-shot
  `sandbox.Run(ctx, WithAgent("claude"), WithWorkspace("."))` mirrors `sbx run AGENT` (create-if-
  missing + attach); the method re-attaches an existing handle (Q15).
- `sb.Start(ctx)` / `sb.Stop(ctx)` → sandbox VM lifecycle (REST). `sb.Exec(…)` → arbitrary command.
- There is **no** `sandbox.Run = create+start`. Don't reintroduce docker/go-sdk's meaning.

```go
sb, err := sandbox.Create(ctx,
    sandbox.WithAgent("claude"),     // claude|codex|copilot|cursor|docker-agent|droid|gemini|kiro|opencode|shell
    sandbox.WithWorkspace("."),      // repeatable; ":ro" suffix supported
    sandbox.WithName("my-proj"),
    sandbox.WithCPUs(4),
    sandbox.WithMemory("8g"),
    sandbox.WithProfile("balanced"),
    sandbox.WithTemplate("img:tag"),
    sandbox.WithClone(),             // in-container git clone instead of bind-mount
)
defer sb.Remove(ctx)

// Interactive agent session (rarely scripted; for human/terminal use):
// sess, err := sb.Run(ctx, sandbox.WithAgentArgs("--model", "opus"))
```

| API | Transport |
|---|---|
| `sandbox.Create(ctx, opts…)` | **shell-out** `sbx create …` |
| `sb.Run(ctx, opts…) (int, error)` | **shell-out** `sbx run …` (inherit terminal stdio) |
| `sandbox.List(ctx)` | REST `GET /sandbox` |
| `sandbox.Get(ctx, name)` | REST `GET /sandbox/{name}` |
| `sb.Start(ctx)` | REST `POST /sandbox/{name}/start` |
| `sb.Stop(ctx)` | REST `POST /sandbox/{name}/stop` |
| `sb.Remove(ctx)` | REST `DELETE /sandbox/{name}` |
| `sb.Inspect(ctx)` / `sb.State()` / `sb.ID()` / `sb.Name()` | REST `GET /sandbox/{name}` |
| `sb.SaveTemplate(ctx, tag)` | **shell-out** `sbx template save …` |

**Option semantics** (from docker/go-sdk): maps cumulative; slices last-write-wins with
`WithAdditional*` helpers; `WithWorkspace` is additive (repeatable). Options map to `sbx create`
flags for the shell-out (`--name`, `--cpus`, `--memory`, `--profile`, `--template`, `--clone`,
`--kit`).

**RESOLVED (was O1) — creation is shell-out.** The REST client has no `CreateSandbox` builder; the
CLI orchestrates creation. `Create`/`Run`/`SaveTemplate` shell out to `sbx` (ADR-0001). Remaining
detail for implementation: exact flag mapping and parsing `sbx create` success output to obtain the
new sandbox name/id (then `Get` to hydrate the handle).

---

## 7. `exec` package — exec & attach

`Exec` is the **automation workhorse** (sbx `run` is interactive-only, so headless agent
automation = exec-ing each agent's own non-interactive CLI, e.g. `claude -p`). Three methods,
one per return shape (explicit over option-driven return magic). All take shared
`exec.ProcessOption`s: `WithEnv`, `WithEnvFile`, `WithWorkdir`, `WithUser`, `WithPrivileged`,
`WithTTY`, `WithStdin`.

**Precondition / auto-start (resolved):** exec methods require a **running** sandbox; on a stopped
one they return `ErrSandboxNotRunning`. Opt-in transparent start via `exec.WithAutoStart()` (or a
client-level toggle) — no hidden VM boots by default.

**Wire protocol (O3 — RESOLVED by live capture).** All exec paths use one endpoint,
`POST /sandbox/{name}/exec/attach`, a Docker-style hijack:
```
POST /sandbox/{name}/exec/attach HTTP/1.1
Connection: Upgrade
Upgrade: tcp
Content-Type: application/json
{"cmd":[...], ...opts}                ← env/workdir/user/tty/privileged as JSON fields
---
HTTP/1.1 101 Switching Protocols
Content-Type: application/vnd.docker.raw-stream
Connection: Upgrade
Upgrade: tcp
Sandboxes-Exec-Id: <execID>           ← exec id returned in this response header
<hijacked conn: stdin written up, stdout/stderr streamed down>
```
- **Framing:** non-TTY → Docker **stdcopy** 8-byte frames `[type,0,0,0,size_be32][payload]` (verified:
  `\x01\x00\x00\x00\x00\x00\x00\x10hello…` = stdout, 16 bytes); TTY → raw passthrough.
- **Exit code:** `GET /sandbox/{name}/exec/{execID}` (`InspectExec`) after the stream ends.
- **Resize:** `POST /sandbox/{name}/exec/{execID}/resize`.
- **Exec preamble (what the CLI does before attach; SDK replicates as needed):** sync credentials
  (`POST /sandbox/{name}/credentials`) → `GET /policy/setup` → `POST /sandbox/{name}/start` (ensure
  running) → **SessionHold** `GET /runtime/{name}/session` (chunked keep-alive, held for the session)
  → `GET /sandbox/{name}` (inspect) → attach.

### 7.1 `Exec` — run to completion, capture
```go
func (sb *Sandbox) Exec(ctx context.Context, cmd []string, opts ...exec.ProcessOption) (int, io.Reader, error)

code, r, err := sb.Exec(ctx, []string{"claude", "-p", "summarise the repo"},
    exec.WithWorkdir("/work"), exec.WithEnv(map[string]string{"CI": "1"}))
```
- Uses `POST /sandbox/{name}/exec/attach` (no TTY); reads the stdcopy stream to completion; the
  returned `io.Reader` is the raw stream by default.
- `exec.WithMultiplexed(stdout, stderr io.Writer)` copies demuxed streams to the given writers
  (via `moby/moby/api/pkg/stdcopy`) before returning. Signature unchanged either way.
- Exit code via `GET /sandbox/{name}/exec/{execID}` (id from the `Sandboxes-Exec-Id` header).

### 7.2 `ExecInteractive` — bidirectional stream
```go
func (sb *Sandbox) ExecInteractive(ctx context.Context, cmd []string, opts ...exec.ProcessOption) (*exec.AttachSession, error)

sess, _ := sb.ExecInteractive(ctx, []string{"bash"}, exec.WithTTY(), exec.WithStdin(os.Stdin))
go io.Copy(os.Stdout, sess.Stdout)
sess.Resize(ctx, 120, 40)   // POST /sandbox/{name}/exec/{id}/resize
code, _ := sess.Wait(ctx)
sess.Close()
```
- `AttachSession` wraps the **hijacked connection** (same `/exec/attach` endpoint, with `tty:true`
  in the JSON body); TTY → raw passthrough, else stdcopy-demuxed. `ctx` cancellation closes the conn.
  `Resize` → `POST /sandbox/{name}/exec/{execID}/resize`; `Wait` → `GET …/exec/{execID}`. (Per Q8,
  `sb.Run` does **not** return this type — it shells out and returns an exit code.)

### 7.3 `ExecDetached` — background + poll
```go
func (sb *Sandbox) ExecDetached(ctx context.Context, cmd []string, opts ...exec.ProcessOption) (execID string, err error)
func (sb *Sandbox) InspectExec(ctx context.Context, execID string) (exec.State, error)
```
- Starts the command in the background (sbx `exec -d`), returns the exec id; poll `InspectExec`
  for running/exit state.

---

## 8. files/cp, ports, `template`, `policy`, `secret`

Authoritative transport map (daemon `apiHandler` set + REST client builders + **live route tracing**):
- **Shell-out:** `Create`, agent `Run`, `template save` (`SaveSandbox`), **all `policy` except Log**,
  `secret`, daemon `start`.
- **REST (live-confirmed paths):** sandbox `list`=`/sandbox`, `inspect/start/stop/remove`,
  `exec`=`/sandbox/{name}/exec` (+`/exec/{id}/resize`), cp=`GET|PUT /sandbox/{name}/files`,
  ports=`/sandbox/{name}/ports`, credentials=`POST /sandbox/{name}/credentials`,
  templates/images=`/docker/images`,`/docker/images/inspect?name=`,`/docker/images/load`,
  `/docker/images/remove`, `policy.Log`=`/network/log`, daemon
  `health|info|loglevel|shutdown|reset|diagnostics`.

### 8.1 files / cp (REST; custom-coded, no client builder)
`GetFile`/`PutFile` handlers stream **tar archives** (`docker cp` semantics: `SANDBOX:PATH` ↔ local
only, directory placed at destination). Path helpers over a tar core (Q10):
```go
func (sb *Sandbox) CopyTo(ctx, localPath, sandboxPath string, opts ...cp.Option) error  // cp.WithFollowSymlinks()
func (sb *Sandbox) CopyFrom(ctx, sandboxPath, localPath string, opts ...cp.Option) error
func (sb *Sandbox) CopyTarTo(ctx, sandboxPath string, tar io.Reader) error               // lower-level core
func (sb *Sandbox) CopyTarFrom(ctx, sandboxPath string) (io.ReadCloser, error)
```

### 8.2 ports (REST)
`sb.Ports(ctx)` → `ListPublishedPorts`; `sb.PublishPort(ctx, spec)` → `PublishPorts`;
`sb.UnpublishPort(ctx, spec)` → `UnpublishPorts`.

### 8.3 template (REST + one shell-out) — templates ARE images
`sbx template ls` lists all template images (base + saved); has `--json`. Domain-faithful `template`
package over the image endpoints:
```go
template.List(ctx)              // REST GET /docker/images        (fields: agent, created_at, id, …)
template.Inspect(ctx, name)     // REST GET /docker/images/inspect?name=
template.Remove(ctx, ref)       // REST DELETE /docker/images/remove
template.Load(ctx, file)        // REST POST /docker/images/load
sb.SaveTemplate(ctx, tag)       // SHELL-OUT `sbx template save` (SaveSandbox; no client builder)
```
No separate `image` package in v1 (raw image mgmt deferred).

### 8.4 policy (SHELL-OUT — Q13 REVISED by live tracing)
**Correction:** policy is **not** daemon-REST. Live tracing shows the CLI's `runPolicyLsCmd` calls
`sandboxlib/runtime.(*DockerNext).ListPolicyRules` (an engine-layer client), and the REST route
`/policy/network/rules` returns **501 "not implemented"** in v0.32.0. So policy rule management goes
through the engine, not the daemon REST API — and the SDK uses **shell-out** (ADR-0001 principle),
not REST:
```go
policy.List(ctx, scope)                          // SHELL-OUT `sbx policy ls`
policy.Allow(ctx, scope, resources...) / Deny    // SHELL-OUT `sbx policy allow|deny network`
policy.RemoveRule(ctx, scope, resources...)      // SHELL-OUT `sbx policy rm network`
policy.SetDefault(ctx, "allow-all"|"balanced"|"deny-all")  // SHELL-OUT `sbx policy set-default`
policy.Profiles(ctx)                             // SHELL-OUT `sbx policy profile ls` (has --json)
policy.Log(ctx, scope)                            // REST `GET /network/log` (200, the one working path)
```
`scope` = global or a sandbox name. (`policy.Log` is the lone REST exception — `/network/log` works.)

### 8.5 secret (minimal + deferred — Q12)
Built-in service login is interactive (OAuth) and `set-custom` is experimental; for headless creds,
**prefer `exec.WithEnv`**. v1 ships a thin `secret` package shelling out to `sbx secret`:
```go
secret.SetCustom(ctx, opts)   // SHELL-OUT `sbx secret set-custom` (placeholder/proxy; EXPERIMENTAL upstream)
secret.List(ctx, scope)       // SHELL-OUT `sbx secret ls`
secret.Remove(ctx, scope, service)  // SHELL-OUT `sbx secret rm`
```
**Deferred:** interactive built-in-service OAuth (`secret set SERVICE`), registry credentials,
and the `SyncCredentials`/per-sandbox `POST /credentials` REST path (revisit if env injection proves
insufficient).

---

## 9. Cross-cutting concerns

- **Schemas (Q14 + DG1–DG6 — DWARF extraction with a tag caveat):** a one-shot generator
  `internal/tools/dwarfgen` (stdlib `debug/elf` + `debug/dwarf`) walks the unstripped `sbx` binary's
  `debug_info`. **Verified empirically:** DWARF yields exact field **names, Go types, nesting, and
  pointer→optional** info, but **NOT struct tags** (`dwarf.StructField` has no tag). So:
  - **Structure** from DWARF: field names/types, `*T`→`T`+`omitempty`, `*[]T`→`[]T,omitempty`,
    `time.Time` preserved, defined string types kept as named enums (DG3).
  - **JSON tags** by convention: `json:"snake_case(FieldName)"` (DG1), **backstopped by a generated
    round-trip test** that feeds real daemon JSON through the structs and fails on any unmapped or
    mismatched field. This test doubles as drift detection.
  - **Roots + closure (DG2):** a curated root list (~20 REST req/resp types) + automatic transitive
    closure; not all `sandboxapi.*`.
  - **Enum values (DG4):** type from DWARF; values (e.g. `SANDBOX_STATUS_RUNNING/STOPPED`) from binary
    string constants + live JSON, emitted as typed constants.
  - **Output (DG5):** checked-in generated `internal/api/types_gen.go`; header records the source
    binary build-id. Not build-time codegen (the `sbx` binary is not a build dependency).
  - **Re-sync (DG6):** on `sbx` update, re-run dwarfgen, diff, review, bump the tested version range;
    the api_version contract test signals when due.
  **No guessed structs.** (First implementation slice = build `dwarfgen` + the validation test.)
- **Errors (Q11):** two typed errors — `APIError{Op string; Status int; Message string}` (REST,
  parsed from `{"message": …}`) and `CLIError{Args []string; ExitCode int; Stderr string}`
  (shell-out) — plus curated sentinels (`ErrSandboxNotFound`, `ErrSandboxExists`,
  `ErrSandboxNotRunning`, `ErrExecNotFound`, `ErrIncompatibleVersion`, `ErrDaemonNotRunning`,
  `ErrBinaryNotFound`),
  `errors.Is`/`As`-friendly. REST status/messages map to sentinels; **no fragile stderr→sentinel
  parsing** (expose `CLIError` raw).
- **Context (E2):** every method takes `context.Context`. REST → cancels the HTTP request;
  shell-out → SIGTERM then SIGKILL to the `sbx` child; `ExecInteractive` → closes the hijacked conn.
- **Workspace paths (E1):** `WithWorkspace` resolved to absolute (caller CWD) before shell-out;
  `:ro` preserved; writable host paths validated.
- **Handle staleness (E3):** `Sandbox` = name + lazily-cached metadata; removed out-of-band →
  next REST call returns `ErrSandboxNotFound`; `Inspect` refreshes; no background polling.
- **Concurrency (E4):** `client.DefaultClient` is a lazy singleton; `Client` is concurrency-safe;
  resource funcs default to it unless `WithClient(cli)` is passed.
- **Logging:** pluggable `slog.Logger`; off by default.

---

## 10. Testing strategy

- **Unit:** stub `http.Server` bound to a temp unix socket per route; table-driven request/response checks.
- **Integration (build-tagged `integration`):** against a real `sbx daemon`; covers create→exec→cp→remove.
- **Attach/exec:** a real-TTY test (pty) exercising raw + demuxed paths and resize.
- **Contract test:** pins daemon `api_version` (`0.10.0`); warns (not fails) on drift.
- CI runs unit + contract; integration gated behind a label/local run (needs the daemon).

---

## 11. Open questions

Resolved by the grilling sessions (2026-06-10):
- **O1 — create:** no REST `CreateSandbox` client; `Create`/`Run`/`SaveTemplate` shell out (ADR-0001).
- **O2 — transport map:** RESOLVED at design level. Shell-out only for `Create`, `Run`, `template
  save`, daemon `start`; **policy, cp, ports, template list/remove/load, daemon stop/reset are REST**.
  Exact image/policy **route path strings** still need extraction from `RegisterHandlersWithBaseURL`.
- **Verb model, automation model, exec shape, auto-start, version policy, identity, Run shape,
  cp shape, error taxonomy, secret scope, policy scope:** resolved — see §5–9, `CONTEXT.md`, ADR-0001.

Resolved by live tracing + live attach capture (2026-06-10):
- **Route paths:** images `/docker/images*`, credentials `/sandbox/{name}/credentials`,
  exec/start/stop/ports under `/sandbox/{name}/…`, policy-status `/policy/setup`, session
  `/runtime/{name}/session` — all confirmed live.
- **Policy transport:** NOT daemon-REST (`/policy/network/rules` is a 501 stub; CLI uses
  `runtime.DockerNext`); SDK uses **shell-out** (§8.4, revises Q13).
- **Socket env override:** `DOCKER_SANDBOXES_API`.
- **O3 — attach handshake:** RESOLVED by live capture — `POST /sandbox/{name}/exec/attach`,
  HTTP 101 Upgrade (`Upgrade: tcp`), `application/vnd.docker.raw-stream`, stdcopy 8-byte framing,
  exec id in `Sandboxes-Exec-Id` header, exit via `GET …/exec/{id}` (§7).
- **`SessionHold`:** `GET /runtime/{name}/session` (chunked keep-alive) — held during attach.
- **Create auto-starts:** `sbx create` leaves the sandbox **running** (relevant to Q4 lifecycle).

Still open (small; resolve in the matching slice):
1. **TTY-mode framing detail:** confirm raw (un-multiplexed) stream + a live resize frame with `-t`
   (the non-TTY path is fully captured; TTY is the Docker raw convention, just unverified here).
2. Module path confirmation before `go mod init` (default `github.com/mwchua/sbx-go-sdk`).

---

## 12. Milestones (indicative)

0. `internal/tools/dwarfgen` → extract `sandboxapi.*` structs into `internal/api` (unblocks typing).
1. `internal/transport` + `internal/cli` + `client` (daemon lifecycle, health/version) — smallest slice.
2. `sandbox` lifecycle — `Create`/`Run` (shell-out) + list/inspect/start/stop/remove (REST).
3. `exec` non-interactive (stdcopy demux).
4. `exec` interactive attach + resize (resolve O3).
5. files / ports.
6. secret / policy / template (resolve O2).
7. Docs, examples, contract test, CI.
