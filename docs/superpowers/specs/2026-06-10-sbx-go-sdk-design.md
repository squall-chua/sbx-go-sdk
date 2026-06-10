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
- Override precedence: `WithSocketPath` option > env override (`SBX_SOCKET`-style, exact name TBD) > default.

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

Rejected alternatives:
- **oapi-codegen from a reconstructed spec** — the spec doesn't exist; authoring it is more work
  than writing structs, and the generated client is bypassed for hijack/file-tar/lifecycle anyway.
- **Thin single-package client** — undershoots the end-to-end automation ergonomics.
- **Multi-module workspace** — full docker/go-sdk fidelity, but unjustified ceremony here.

---

## 4. Package layout

Module path: `github.com/mwchua/sbx-go-sdk` (placeholder — confirm before `go mod init`).

```
sbx-go-sdk/
├── internal/transport/   # unix-socket http.Client; connection-hijack helper for attach
├── internal/api/         # low-level typed REST: DWARF-grounded structs + 1:1 route calls
├── client/               # Client (connection + daemon lifecycle); New(); DefaultClient; options
├── sandbox/              # core resource: Run/Create + Sandbox object; lifecycle/feature files
├── exec/                 # ProcessOption, ExecResult, AttachSession (shared exec types)
├── secret/               # secrets / sandbox credentials
├── policy/               # network egress policy
└── template/             # save / load / ls / rm templates
```

**Layering.** `internal/api` is the low-level typed REST client (our equivalent of the moby client
that `docker/go-sdk` wraps). `client` + resource packages are the high-level layer; `client.Client`
exposes the low-level surface via an `API()` accessor for advanced use.

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
    client.WithAutoStart(),     // EnsureRunning before first call
    client.WithVersionCheck(),  // POST /version; error on "incompatible"
    client.WithHTTPTimeout(30*time.Second),
)
// or the lazy default:
cli := client.DefaultClient
```

| Method | Route | Notes |
|---|---|---|
| `Health(ctx)` | `GET /health` | `{release,status,version}` |
| `CheckVersion(ctx)` | `POST /version` | result: `compatible`/`incompatible`/`unknown` |
| `Info(ctx)` | `GET /daemon/info` | `{api_socket, docker_socket}` |
| `DaemonHealth(ctx)` | `GET /daemon/health` | api_version, revision, release |
| `LogLevels(ctx)` | `GET /daemon/loglevel` | `{general, proxy}` |
| `SetLogLevel(ctx, category, level)` | `POST /daemon/loglevel/set` | category: proxy/general/all |
| `EnsureRunning(ctx)` | locate socket + `Health`; spawn if down | idempotent |
| `StartDaemon(ctx, opts)` | `exec.Command("sbx","daemon","start","--detach",…)` | `--policy` passthrough |
| `StopDaemon(ctx)` | `sbx daemon stop` (or signal) | |
| `DaemonStatus(ctx)` | socket probe + health | returns running + paths |

`StartDaemon` shells out to the `sbx` binary (resolved from PATH or `WithBinaryPath`). The SDK does
**not** re-exec arbitrary binaries beyond this documented call.

---

## 6. `sandbox` package — core resource

`Run` = create + start + wait-ready (mirrors `container.Run`); `Create` = create only.

```go
sb, err := sandbox.Run(ctx,
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
```

| API | Route |
|---|---|
| `sandbox.List(ctx)` | `GET /sandbox` |
| `sandbox.Get(ctx, name)` | `GET /sandbox/{name}` |
| `sb.Start(ctx)` | `POST /sandbox/{name}/start` |
| `sb.Stop(ctx)` | `POST /sandbox/{name}/stop` |
| `sb.Remove(ctx)` | `DELETE /sandbox/{name}` |
| `sb.Inspect(ctx)` / `sb.State()` / `sb.ID()` / `sb.Name()` | `GET /sandbox/{name}` |
| `sb.SaveTemplate(ctx, tag)` | `POST /sandbox/{name}/save` |

**Option semantics** (from docker/go-sdk): maps cumulative; slices last-write-wins with
`WithAdditional*` helpers; `WithWorkspace` is additive (repeatable).

**OPEN QUESTION (O1) — sandbox creation.** `POST /sandbox {}` returned `{"message":"not implemented"}`
on the live daemon, suggesting creation may be CLI-side orchestration rather than a single REST call.
**Resolution:** before implementing, trace a real `sbx -D create …` against the live socket and/or
read the create flow from DWARF. If creation is genuinely multi-step, `sandbox.Run` replicates the
documented CLI orchestration sequence. This part will not be hand-waved.

---

## 7. `exec` package — exec & attach

### 7.1 Non-interactive (mirrors docker/go-sdk `Exec`)
Fixed signature (no arity changes via options):
```go
func (sb *Sandbox) Exec(ctx context.Context, cmd []string, opts ...exec.ProcessOption) (int, io.Reader, error)

code, r, err := sb.Exec(ctx, []string{"go", "test", "./..."},
    exec.WithWorkdir("/work"),
    exec.WithEnv(map[string]string{"CI": "1"}),
)
```
- `POST /sandbox/{name}/exec` creates the exec; response carries an exec id.
- Output is a **Docker multiplexed stdcopy stream**. By default the returned `io.Reader` is that
  raw stream as-is.
- Demuxing is opt-in via a `ProcessOption` that supplies destination writers —
  `exec.WithMultiplexed(stdout, stderr io.Writer)` — which copies the demuxed streams to those
  writers (via `moby/moby/api/pkg/stdcopy`) before returning; the returned `io.Reader` is then drained.
  The signature is unchanged either way.
- Exit code retrieved via `GET /sandbox/{name}/exec/{id}` (`InspectExec`).

### 7.2 Interactive attach
```go
sess, err := sb.Attach(ctx, exec.WithTTY(), exec.WithStdin(os.Stdin))
go io.Copy(os.Stdout, sess.Stdout)
sess.Resize(ctx, 120, 40)  // POST /sandbox/{name}/exec/{id}/resize
code, err := sess.Wait(ctx)
sess.Close()
```
- `AttachSession` wraps the **hijacked connection** (HTTP `Upgrade`/raw stream).
- TTY allocated → raw passthrough; otherwise stdcopy-demuxed. Same semantics as `docker attach`.
- Cancellation of `ctx` closes the hijacked conn.

**OPEN QUESTION (O3) — attach handshake.** Exact `Upgrade` header values, content-type
(`application/vnd.docker.*`), and half-close behavior to be confirmed by tracing a live attach and
reading `apiHandler.AttachExec` / `bridgeAttachStreams` in DWARF.

---

## 8. `secret`, `policy`, `template`, files, ports

| API | Route(s) |
|---|---|
| `sb.CopyTo(ctx, localPath, sandboxPath)` | `PUT /sandbox/{name}/files` (tar stream) |
| `sb.CopyFrom(ctx, sandboxPath, localPath)` | `GET /sandbox/{name}/files` (tar stream) |
| `sb.Ports(ctx)` | `GET /sandbox/{name}/ports` |
| `sb.PublishPort(ctx, spec)` | `POST /sandbox/{name}/ports` |
| `sb.SetCredentials(ctx, creds)` / `secret.Set/List/Remove` | `POST /sandbox/{name}/credentials` + global secret store |
| `policy.AllowNetwork/DenyNetwork/Reset/List/SetDefault` | network policy (routes TBD) |
| `template.Save/Load/List/Remove` | `POST /sandbox/{name}/save` + template store |

**OPEN QUESTION (O2) — secret/policy/template routes.** Not all appeared as top-level REST paths in
probing. Trace each from the CLI during implementation; mark any that require the engine `docker.sock`
layer, and either implement or explicitly defer those to a later version.

---

## 9. Cross-cutting concerns

- **Schemas:** extracted from DWARF (exact field names, types, json tags), validated against live JSON.
  No guessed structs. A generation/extraction note is kept alongside the structs for future re-sync.
- **Errors:** typed sentinels (`ErrSandboxNotFound`, `ErrExecNotFound`, `ErrIncompatibleVersion`),
  wrapping a structured `APIError{Status int, Message string}` parsed from `{"message": …}` bodies.
- **Context:** every network method takes `context.Context`; cancellation closes hijacked conns.
- **Concurrency:** `Client` is safe for concurrent use (stateless over a shared `http.Client`).
- **Logging:** pluggable `slog.Logger`; off by default.

---

## 10. Testing strategy

- **Unit:** stub `http.Server` bound to a temp unix socket per route; table-driven request/response checks.
- **Integration (build-tagged `integration`):** against a real `sbx daemon`; covers create→exec→cp→remove.
- **Attach/exec:** a real-TTY test (pty) exercising raw + demuxed paths and resize.
- **Contract test:** pins daemon `api_version` (`0.10.0`); warns (not fails) on drift.
- CI runs unit + contract; integration gated behind a label/local run (needs the daemon).

---

## 11. Open questions (resolved during spec-resolution / first implementation slice)

1. **O1 — create:** real `POST /sandbox` body, or is `Run` an orchestration sequence? (trace `sbx -D create`)
2. **O2 — routes:** exact secret/policy/template endpoints; which need the engine layer.
3. **O3 — attach:** `Upgrade` handshake, content-types, half-close.
4. Module path confirmation before `go mod init`.
5. Env var name for socket override (verify against `sandboxlib`).

---

## 12. Milestones (indicative)

1. `internal/transport` + `client` (daemon lifecycle, health/version) — smallest end-to-end slice.
2. `sandbox` lifecycle (resolve O1) — create/list/inspect/start/stop/remove.
3. `exec` non-interactive (stdcopy demux).
4. `exec` interactive attach + resize (resolve O3).
5. files / ports.
6. secret / policy / template (resolve O2).
7. Docs, examples, contract test, CI.
