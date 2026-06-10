# sbx-go-sdk Resources & Run Implementation Plan (Plan 2 of 2)

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Extend the sbx-go-sdk foundation (Plan 1, merged to `master`) with the remaining resource surface — interactive agent `Run`, published `ports`, file `cp`, `template` images, network `policy`, and `secret` management — plus the deferred client/exec polish (`Diagnostics`/`DaemonHealth`, `exec.WithMultiplexed`, `WithAutoStart` wiring, and the unwired error sentinels).

**Architecture:** Same hybrid transport as Plan 1 (ADR-0001): REST over the unix socket for runtime ops, shell-out to `sbx` for orchestration-heavy ops. **Two spec revisions confirmed by live verification against daemon api 0.10.0 / sbx v0.32.0** (see "Verified deviations" below): cp is **shell-out** (`GET /sandbox/{name}/files` returns HTTP 501 "not implemented"); `policy`/`secret` list output is **text-only** (no `--json`). Reuses Plan 1's `internal/transport`, `internal/cli`, `internal/stdcopy`, `internal/api`, `client`, `sandbox`, `exec`.

**Tech Stack:** Go 1.25, stdlib only for the SDK; `testify` for tests. Module `github.com/squall-chua/sbx-go-sdk`.

**Spec:** [docs/superpowers/specs/2026-06-10-sbx-go-sdk-design.md](../specs/2026-06-10-sbx-go-sdk-design.md) §5–§8.

---

## Verified deviations from the spec (live-confirmed 2026-06-10)

These were checked against the running daemon and the installed `sbx` v0.32.0 before this plan was written. Treat them as authoritative; do not "fix" them back to the spec.

1. **cp is shell-out, not REST tar.** `GET /sandbox/{name}/files?path=…` returns `HTTP 501 {"message":"not implemented"}` (the route is registered — `OPTIONS` → `Allow: OPTIONS, GET, PUT` — but the GET handler is a stub). `sbx cp SRC DST` works in both directions. So `CopyTo`/`CopyFrom` shell out to `sbx cp`. The REST tar core (`CopyTarTo`/`CopyTarFrom` from spec §8.1) is **deferred** until the daemon implements `GET /files`.
2. **`policy`/`secret` list output is text-only.** `sbx policy ls --json` and `sbx policy profile ls --json` return `ERROR: unknown flag: --json`. `sbx secret ls` prints a text table. So `policy.List`, `policy.Profiles`, `secret.List` return the raw CLI **text** (a `string`); structured parsing is out of scope for v1. (`policy log` *does* support `--json`, but `policy.Log` uses the verified REST path instead.)
3. **`policy.Log` is REST.** `GET /network/log` returns `{"blocked_hosts":[…],"allowed_hosts":[{host,vm_name,proxy_type,rule,last_seen,since,count_since}]}` (HTTP 200). This is the lone working policy REST path (rule management is engine-layer; `/policy/network/rules` → 501).
4. **ports POST takes a JSON array.** `POST /sandbox/{name}/ports` body is a JSON array `[{sandbox_port,host_port,host_ip,protocol}]` (HTTP 200, additive — it adds, it does not replace). `GET` returns the same shape (`[]` when none). No verified REST *unpublish* path → `UnpublishPort` shells out to `sbx ports {name} --unpublish SPEC`.
5. **template list/inspect verified REST shapes.** `GET /docker/images` → `[{agent,created_at,id,repository,tag}]`. `GET /docker/images/inspect?name=<ref>` → `{agent,created_at,id}`. `DELETE /docker/images/remove` (Allow: DELETE) and `POST /docker/images/load` (Allow: POST) exist; their exact query-param/body were **not** fully verified — the relevant tasks flag a live check with a shell-out fallback (`sbx template rm|load`).

---

## File Structure

| Path | Responsibility |
|---|---|
| `client/daemon.go` (modify) | add `DaemonHealth`, `Diagnostics`, `DaemonStatus`; change `DaemonInfo.DockerSocket` to `*string` |
| `exec/options.go` (modify) | add `WithMultiplexed` (stdout/stderr writers) |
| `exec/exec.go` (modify) | wire `WithMultiplexed`; wire `WithAutoStart`; `ErrSandboxNotRunning` precondition; map exec-404 → `ErrExecNotFound` |
| `sandbox/options.go` (modify) | add `WithStdio`, `WithAgentArgs` |
| `sandbox/definition.go` (modify) | add `agentArgs`/stdio fields + `toRunArgs` |
| `sandbox/run.go` (create) | `Run` (package one-shot) + `(*Sandbox).Run` (re-attach) — shell-out, terminal-inherit |
| `sandbox/ports.go` (create) | `Port` type; `(*Sandbox).Ports`/`PublishPort` (REST) + `UnpublishPort` (shell-out) |
| `sandbox/cp.go` (create) | `CopyOption`/`WithFollowSymlinks`; `(*Sandbox).CopyTo`/`CopyFrom` (shell-out `sbx cp`) |
| `sandbox/template_save.go` (create) | `(*Sandbox).SaveTemplate` (shell-out `sbx template save`) |
| `template/template.go` (create) | `List`/`Inspect`/`Remove`/`Load` over `/docker/images*` (REST) |
| `policy/policy.go` (create) | `SetDefault`/`Allow`/`Deny`/`RemoveRule`/`Reset` (shell-out); `List`/`Profiles` (shell-out text); `Log` (REST) |
| `secret/secret.go` (create) | `SetCustom`/`List`/`Remove` (shell-out) |
| `internal/integration/resources_test.go` (create) | live ports-publish/list + cp round-trip + template-list smoke (`-tags integration`) |
| `README.md` (modify) | document the new surface |

**Import direction (unchanged, do not violate):** `client` imports nothing from `sandbox`/`exec`/`template`/`policy`/`secret`. `sandbox` imports `client` + `internal/*`. `exec`/`template`/`policy`/`secret` import `client` (+ `sandbox` for exec) + `internal/*`. **No `sb.Exec` forwarders** are added (they would require `sandbox`→`exec`, a cycle) — `exec.Exec(ctx, sb, …)` remains the API. `sb.Run`/`Ports`/`CopyTo`/`SaveTemplate` are pure shell-out/REST and live in `sandbox` with no `exec` dependency.

---

## Phase A — deferred Plan-1 polish

### Task 1: client DaemonHealth, Diagnostics, DaemonStatus + optional DockerSocket

**Files:**
- Modify: `client/daemon.go`
- Modify: `client/daemon_test.go`

- [ ] **Step 1: Update the existing DockerSocket assertion + add the failing test**

In `client/daemon_test.go`, the existing `TestDaemonInfoAndLogLevels` asserts `info.DockerSocket` as a `string`. Change that one line to dereference the new pointer:

```go
	require.Equal(t, "/d.sock", *info.DockerSocket)
```

Then append:

```go
func TestDaemonHealthAndDiagnostics(t *testing.T) {
	sock := stub(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/daemon/health":
			w.Write([]byte(`{"api_version":"0.10.0","release":false,"revision":"abc","status":"healthy","version":"v0.32.0"}`))
		case "/daemon/diagnostics":
			w.Write([]byte(`{"info":{"State":{"Sandboxes":{"Total":0}}}}`))
		default:
			t.Fatalf("unexpected %s", r.URL.Path)
		}
	}))
	c, _ := New(context.Background(), WithSocketPath(sock))
	dh, err := c.DaemonHealth(context.Background())
	require.NoError(t, err)
	require.Equal(t, "0.10.0", dh.APIVersion)
	require.Equal(t, "healthy", dh.Status)

	diag, err := c.Diagnostics(context.Background())
	require.NoError(t, err)
	require.Contains(t, string(diag), "Sandboxes")
}

func TestDaemonStatus_Running(t *testing.T) {
	sock := stub(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"status":"healthy"}`))
	}))
	c, _ := New(context.Background(), WithSocketPath(sock))
	st, err := c.DaemonStatus(context.Background())
	require.NoError(t, err)
	require.True(t, st.Running)
	require.Equal(t, sock, st.Socket)
}
```

- [ ] **Step 2: Run to verify it fails**

Run: `go test ./client/ -run 'TestDaemonHealth|TestDaemonStatus' -v`
Expected: FAIL — `c.DaemonHealth`/`c.Diagnostics`/`c.DaemonStatus` undefined (and a compile error on the `*info.DockerSocket` line until Step 3).

- [ ] **Step 3: Change DockerSocket to *string and add the methods**

In `client/daemon.go`: add `"encoding/json"` to the import block. Change the `DaemonInfo` struct's field:

```go
// DaemonInfo is the /daemon/info response.
type DaemonInfo struct {
	APISocket    string  `json:"api_socket"`
	DockerSocket *string `json:"docker_socket,omitempty"`
}
```

Append:

```go
// DaemonHealthResponse is the /daemon/health response (richer than /health).
type DaemonHealthResponse struct {
	APIVersion string `json:"api_version"`
	Release    bool   `json:"release"`
	Revision   string `json:"revision"`
	Status     string `json:"status"`
	Version    string `json:"version"`
}

// DaemonHealth returns the daemon's detailed health (api version, revision, …).
func (c *Client) DaemonHealth(ctx context.Context) (*DaemonHealthResponse, error) {
	var h DaemonHealthResponse
	if err := c.tr.DoJSON(ctx, http.MethodGet, "/daemon/health", nil, &h); err != nil {
		return nil, mapHTTPError("daemon-health", err)
	}
	return &h, nil
}

// Diagnostics returns the daemon self-check report as raw JSON (a large nested
// object under an "info" key); callers decode the fields they need.
func (c *Client) Diagnostics(ctx context.Context) (json.RawMessage, error) {
	var raw json.RawMessage
	if err := c.tr.DoJSON(ctx, http.MethodGet, "/daemon/diagnostics", nil, &raw); err != nil {
		return nil, mapHTTPError("diagnostics", err)
	}
	return raw, nil
}

// Status reports daemon liveness plus the socket it was probed on.
type Status struct {
	Running bool
	Socket  string
}

// DaemonStatus probes the socket via Health and reports running + path. A down
// daemon yields Running=false with a nil error (so callers can branch).
func (c *Client) DaemonStatus(ctx context.Context) (Status, error) {
	st := Status{Socket: c.tr.Socket()}
	if _, err := c.Health(ctx); err == nil {
		st.Running = true
	}
	return st, nil
}
```

- [ ] **Step 4: Run to verify it passes**

Run: `go test ./client/ -v`
Expected: PASS (all client tests, including the updated `*info.DockerSocket`).

- [ ] **Step 5: Commit**

```bash
git add client/daemon.go client/daemon_test.go
git commit -m "feat(client): DaemonHealth, Diagnostics, DaemonStatus; optional DockerSocket" -m "Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

### Task 2: exec.WithMultiplexed (split stdout/stderr)

**Files:**
- Modify: `exec/options.go`
- Modify: `exec/exec.go`
- Test: `exec/exec_test.go`

- [ ] **Step 1: Write the failing test (append to `exec/exec_test.go`)**

The existing `attachStub` streams a single stdout frame `frame(1, "hello\n")` on `/sandbox/s1/exec/attach` (shared by `exec_test.go` and `attach_test.go`). To make the stdout/stderr split observable WITHOUT breaking the other tests, **keep** the stdout frame and just **add** a stderr frame after it. In `serveConn`, immediately after `conn.Write(frame(1, "hello\n"))`, add:

```go
		conn.Write(frame(2, "err\n"))
```

(This is purely additive: `TestExec_CaptureAndExit` and `TestExecInteractive_StreamsAndWaits` still see `"hello\n"` on stdout and ignore the discarded stderr — no changes needed to them.)

Then append the test:

```go
func TestExec_Multiplexed(t *testing.T) {
	c, _ := attachStub(t)
	sb := sandbox.NewForTest(c, "s1")
	var outBuf, errBuf bytes.Buffer
	code, r, err := Exec(context.Background(), sb, []string{"sh", "-c", "..."},
		WithMultiplexed(&outBuf, &errBuf))
	require.NoError(t, err)
	require.Equal(t, 0, code)
	require.Equal(t, "hello\n", outBuf.String())
	require.Equal(t, "err\n", errBuf.String())
	// With WithMultiplexed the returned reader is drained into the writers, so empty.
	rest, _ := io.ReadAll(r)
	require.Empty(t, rest)
}
```

Add `"bytes"` to the `exec_test.go` import block.

- [ ] **Step 2: Run to verify it fails**

Run: `go test ./exec/ -run 'TestExec_Multiplexed|TestExec_Capture' -v`
Expected: FAIL — `WithMultiplexed` undefined.

- [ ] **Step 3: Add the option (append to `exec/options.go`)**

Add fields to `processConfig`:

```go
	muxOut io.Writer
	muxErr io.Writer
```

Add `"io"` to the `exec/options.go` import block, and append:

```go
// WithMultiplexed routes the demultiplexed stdout and stderr streams to the given
// writers instead of returning stdout via the reader. When set, the reader Exec
// returns is empty.
func WithMultiplexed(stdout, stderr io.Writer) ProcessOption {
	return func(c *processConfig) { c.muxOut = stdout; c.muxErr = stderr }
}
```

- [ ] **Step 4: Wire it into `exec.go`**

In `Exec`, replace the demux block. Currently:

```go
	body := buildBody(cmd, opts...)
	body.TTY = false
```

Capture the config so `Exec` can see `muxOut`/`muxErr`. Change `buildBody` calls are fine; add a parallel parse. Replace the capture section of `Exec` (the `io.Pipe()` goroutine + `io.ReadAll`) with:

```go
	cfg := parseConfig(opts...)
	...
	defer conn.Close()
	execID := hdr.Get("Sandboxes-Exec-Id")

	var out []byte
	if cfg.muxOut != nil || cfg.muxErr != nil {
		// Route demuxed streams straight to the caller's writers.
		_, derr := stdcopy.Demux(orDiscard(cfg.muxOut), orDiscard(cfg.muxErr), conn)
		if derr != nil {
			return 0, byteReader(nil), client.MapError("exec", derr)
		}
	} else {
		pr, pw := io.Pipe()
		go func() {
			var sink discardCloser
			_, derr := stdcopy.Demux(pw, &sink, conn)
			pw.CloseWithError(derr)
		}()
		out, _ = io.ReadAll(pr)
	}

	st, err := inspectExec(ctx, sb, execID)
	if err != nil {
		return 0, byteReader(out), err
	}
	return st.ExitCode, byteReader(out), nil
```

Add the helpers (in `exec.go`):

```go
// parseConfig applies opts to a processConfig (mirrors buildBody but returns the cfg).
func parseConfig(opts ...ProcessOption) processConfig {
	var c processConfig
	for _, o := range opts {
		o(&c)
	}
	return c
}

// orDiscard returns w, or an always-succeeding discard writer if w is nil.
func orDiscard(w io.Writer) io.Writer {
	if w == nil {
		return discardCloser{}
	}
	return w
}
```

(`discardCloser` already satisfies `io.Writer`.) Keep `body := buildBody(cmd, opts...)` for the JSON; `parseConfig(opts...)` re-derives the cfg for the mux writers — do not change `buildBody`.

- [ ] **Step 5: Run to verify it passes**

Run: `go test ./exec/ -v`
Expected: PASS (all exec tests).

- [ ] **Step 6: Commit**

```bash
git add exec/options.go exec/exec.go exec/exec_test.go
git commit -m "feat(exec): WithMultiplexed routes demuxed stdout/stderr to writers" -m "Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

### Task 3: wire WithAutoStart + ErrSandboxNotRunning precondition + ErrExecNotFound

**Files:**
- Modify: `exec/exec.go`
- Test: `exec/exec_test.go`

The spec (§7): exec requires a running sandbox; on a stopped one it returns `ErrSandboxNotRunning`; `WithAutoStart()` starts it first. And `InspectExec` on an unknown exec id should surface `ErrExecNotFound`, not `ErrSandboxNotFound`.

- [ ] **Step 1: Write the failing tests (append to `exec/exec_test.go`)**

The stub must answer `GET /sandbox/s1` (inspect, for the running check) and a `POST /sandbox/s1/start`. Add these cases to `serveConn`:

```go
	case req.URL.Path == "/sandbox/s1":
		// running sandbox by default
		body := `{"name":"s1","status":"SANDBOX_STATUS_RUNNING"}`
		conn.Write([]byte("HTTP/1.1 200 OK\r\nContent-Type: application/json\r\n" +
			"Content-Length: " + itoa(len(body)) + "\r\n\r\n" + body))
	case req.URL.Path == "/sandbox/stopped":
		body := `{"name":"stopped","status":"SANDBOX_STATUS_STOPPED"}`
		conn.Write([]byte("HTTP/1.1 200 OK\r\nContent-Type: application/json\r\n" +
			"Content-Length: " + itoa(len(body)) + "\r\n\r\n" + body))
	case req.URL.Path == "/sandbox/s1/exec/missing":
		conn.Write([]byte("HTTP/1.1 404 Not Found\r\nContent-Type: application/json\r\n" +
			"Content-Length: 27\r\n\r\n{\"message\":\"exec not found\"}"))
```

Append tests:

```go
func TestInspectExec_NotFoundMapsToErrExecNotFound(t *testing.T) {
	c, _ := attachStub(t)
	sb := sandbox.NewForTest(c, "s1")
	_, err := InspectExec(context.Background(), sb, "missing")
	require.ErrorIs(t, err, client.ErrExecNotFound)
}

func TestExec_StoppedSandboxWithoutAutoStart(t *testing.T) {
	c, _ := attachStub(t)
	sb := sandbox.NewForTest(c, "stopped")
	_, _, err := Exec(context.Background(), sb, []string{"echo", "hi"})
	require.ErrorIs(t, err, client.ErrSandboxNotRunning)
}
```

(`client` is already imported in `exec_test.go`.)

- [ ] **Step 2: Run to verify it fails**

Run: `go test ./exec/ -run 'TestInspectExec_NotFound|TestExec_Stopped' -v`
Expected: FAIL — exec returns the wrong error / no precondition.

- [ ] **Step 3: Implement the precondition + autostart + exec-404 mapping (`exec/exec.go`)**

Add a helper and call it at the top of `Exec`, `ExecInteractive`, and `ExecDetached` (right after `buildBody`/`parseConfig`, before `Hijack`):

```go
// ensureRunnable enforces the exec precondition: the sandbox must be running. If
// WithAutoStart is set it starts a stopped sandbox first; otherwise a stopped
// sandbox yields ErrSandboxNotRunning.
func ensureRunnable(ctx context.Context, sb *sandbox.Sandbox, cfg processConfig) error {
	info, err := sb.Inspect(ctx)
	if err != nil {
		return err
	}
	if string(info.Status) == sandbox.StatusRunning {
		return nil
	}
	if !cfg.autoStart {
		return client.ErrSandboxNotRunning
	}
	return sb.Start(ctx)
}
```

In `Exec`: after `cfg := parseConfig(opts...)` add:

```go
	if err := ensureRunnable(ctx, sb, cfg); err != nil {
		return 0, nil, err
	}
```

In `ExecInteractive` and `ExecDetached`: after building the body, add `cfg := parseConfig(opts...)` and:

```go
	if err := ensureRunnable(ctx, sb, cfg); err != nil {
		return nil, err   // ExecInteractive
	}
	// ExecDetached: return "", err
```

For the exec-404 mapping, change `inspectExec` to translate a 404 into `ErrExecNotFound`:

```go
func inspectExec(ctx context.Context, sb *sandbox.Sandbox, execID string) (State, error) {
	var st State
	err := sb.Client().Transport().DoJSON(ctx, http.MethodGet, "/sandbox/"+sb.Name()+"/exec/"+execID, nil, &st)
	if err != nil {
		return State{}, mapExecError(err)
	}
	return st, nil
}

// mapExecError maps a 404 from an exec endpoint to ErrExecNotFound; otherwise it
// defers to the generic client error mapper.
func mapExecError(err error) error {
	var se *transport.HTTPStatusError
	if errors.As(err, &se) && se.Status == 404 {
		return errors.Join(client.ErrExecNotFound, &client.APIError{Op: "inspect-exec", Status: 404, Message: "exec not found"})
	}
	return client.MapError("inspect-exec", err)
}
```

Add imports to `exec.go`: `"errors"` and `"github.com/squall-chua/sbx-go-sdk/internal/transport"`.

Note: `ensureRunnable` adds an inspect round-trip per exec. The `attachStub` answers `GET /sandbox/s1` as running, so `TestExec_CaptureAndExit` still passes.

- [ ] **Step 4: Run to verify it passes**

Run: `go test ./exec/ -v`
Expected: PASS (all exec tests).

- [ ] **Step 5: Commit**

```bash
git add exec/exec.go exec/exec_test.go
git commit -m "feat(exec): WithAutoStart wiring + ErrSandboxNotRunning/ErrExecNotFound" -m "Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

---

## Phase B — sandbox.Run (interactive agent attach, shell-out)

### Task 4: Run options + toRunArgs

**Files:**
- Modify: `sandbox/options.go`
- Modify: `sandbox/definition.go`
- Test: `sandbox/run_test.go` (create)

- [ ] **Step 1: Write the failing test (`sandbox/run_test.go`)**

```go
package sandbox

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDefinition_ToRunArgs(t *testing.T) {
	d := newDefinition(
		WithAgent("claude"),
		WithWorkspace("/abs/ws"),
		WithCPUs(4),
		WithAgentArgs("--model", "opus"),
	)
	args, err := d.toRunArgs()
	require.NoError(t, err)
	require.Equal(t, []string{
		"run", "claude", "/abs/ws", "--cpus", "4", "--", "--model", "opus",
	}, args)
}

func TestDefinition_ToRunArgs_NoAgentArgs(t *testing.T) {
	d := newDefinition(WithAgent("shell"), WithWorkspace("/w"))
	args, err := d.toRunArgs()
	require.NoError(t, err)
	require.Equal(t, []string{"run", "shell", "/w"}, args)
}
```

- [ ] **Step 2: Run to verify it fails**

Run: `go test ./sandbox/ -run TestDefinition_ToRunArgs -v`
Expected: FAIL — `WithAgentArgs`/`toRunArgs` undefined.

- [ ] **Step 3: Add the options (append to `sandbox/options.go`)**

Add `"io"` to the `sandbox/options.go` import block (new — the file currently has none; add an import block), and append:

```go
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
```

- [ ] **Step 4: Add fields + toRunArgs (`sandbox/definition.go`)**

Add `"io"` to the `sandbox/definition.go` import block. Add fields to `Definition`:

```go
	agentArgs []string
	stdin     io.Reader
	stdout    io.Writer
	stderr    io.Writer
```

Append:

```go
// toRunArgs builds the `sbx run AGENT [WORKSPACE...] [create-flags] [-- AGENT_ARGS]`
// vector for the package-level create-if-missing Run. Workspaces must already be
// absolute (resolved by the caller).
func (d *Definition) toRunArgs() ([]string, error) {
	if d.agent == "" {
		return nil, errors.New("sandbox: agent is required (WithAgent)")
	}
	if len(d.workspaces) == 0 {
		return nil, errors.New("sandbox: at least one workspace is required (WithWorkspace)")
	}
	args := []string{"run", d.agent}
	args = append(args, d.workspaces...)
	if d.cpus > 0 {
		args = append(args, "--cpus", strconv.Itoa(d.cpus))
	}
	if d.memory != "" {
		args = append(args, "--memory", d.memory)
	}
	if d.profile != "" {
		args = append(args, "--profile", d.profile)
	}
	if d.template != "" {
		args = append(args, "--template", d.template)
	}
	if d.clone {
		args = append(args, "--clone")
	}
	if len(d.agentArgs) > 0 {
		args = append(args, "--")
		args = append(args, d.agentArgs...)
	}
	return args, nil
}
```

(`errors` and `strconv` are already imported in `definition.go`.)

- [ ] **Step 5: Run to verify it passes**

Run: `go test ./sandbox/ -run TestDefinition_ToRunArgs -v`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add sandbox/options.go sandbox/definition.go sandbox/run_test.go
git commit -m "feat(sandbox): Run options (WithAgentArgs/WithStdio) + toRunArgs" -m "Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

### Task 5: Run (package one-shot) + (*Sandbox).Run

**Files:**
- Create: `sandbox/run.go`
- Test: `sandbox/run_test.go`

`Run` inherits the caller's terminal stdio (via `cli.Inherit`, built in Plan 1) and returns the agent's exit code. A non-zero agent exit is `(code, nil)`; only spawn failures are errors.

- [ ] **Step 1: Write the failing test (append to `sandbox/run_test.go`)**

```go
import (
	"context"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"github.com/squall-chua/sbx-go-sdk/client"
)

func TestRun_Package_InheritsExitCode(t *testing.T) {
	// fake sbx echoes the run args and exits 5; stub daemon needed for client.New.
	sock := filepath.Join(t.TempDir(), "d.sock")
	l, err := net.Listen("unix", sock)
	require.NoError(t, err)
	srv := &http.Server{Handler: http.NewServeMux()}
	go srv.Serve(l)
	t.Cleanup(func() { srv.Close() })
	bin := filepath.Join(t.TempDir(), "sbx")
	require.NoError(t, os.WriteFile(bin, []byte("#!/bin/sh\necho \"ran: $*\"; exit 5\n"), 0o755))
	c, err := client.New(context.Background(), client.WithSocketPath(sock), client.WithBinaryPath(bin))
	require.NoError(t, err)

	ws := filepath.Join(t.TempDir(), "wsr")
	require.NoError(t, os.Mkdir(ws, 0o755))
	var out bytes.Buffer
	code, err := Run(context.Background(), c,
		WithAgent("shell"), WithWorkspace(ws), WithStdio(nil, &out, &out))
	require.NoError(t, err)
	require.Equal(t, 5, code)
	require.Contains(t, out.String(), "ran: run shell")
}
```

Add `"bytes"` to the `run_test.go` imports.

- [ ] **Step 2: Run to verify it fails**

Run: `go test ./sandbox/ -run TestRun_Package -v`
Expected: FAIL — `Run` undefined.

- [ ] **Step 3: Implement `sandbox/run.go`**

```go
package sandbox

import (
	"context"
	"path/filepath"
	"strings"

	"github.com/squall-chua/sbx-go-sdk/client"
	"github.com/squall-chua/sbx-go-sdk/internal/cli"
)

// Run provisions-if-missing and interactively attaches an agent (`sbx run AGENT
// …`), inheriting the caller's terminal stdio (override with WithStdio). It blocks
// until the agent exits and returns its exit code. A non-zero agent exit is
// (code, nil); only spawn/wait failures return a non-nil error. It does not return
// a handle (use Create/Get for that).
func Run(ctx context.Context, c *client.Client, opts ...Option) (int, error) {
	d := newDefinition(opts...)
	for i, ws := range d.workspaces {
		path, ro, _ := strings.Cut(ws, ":")
		abs, err := filepath.Abs(path)
		if err != nil {
			return -1, err
		}
		if ro == "ro" {
			d.workspaces[i] = abs + ":ro"
		} else {
			d.workspaces[i] = abs
		}
	}
	args, err := d.toRunArgs()
	if err != nil {
		return -1, err
	}
	r, err := c.Runner()
	if err != nil {
		return -1, err
	}
	return r.Inherit(ctx, d.stdio(), nil, args...)
}

// Run re-attaches an existing sandbox (`sbx run SANDBOX [-- AGENT_ARGS]`),
// inheriting terminal stdio. Returns the agent's exit code.
func (s *Sandbox) Run(ctx context.Context, opts ...Option) (int, error) {
	d := newDefinition(opts...)
	args := []string{"run", s.info.Name}
	if len(d.agentArgs) > 0 {
		args = append(args, "--")
		args = append(args, d.agentArgs...)
	}
	r, err := s.cli.Runner()
	if err != nil {
		return -1, err
	}
	return r.Inherit(ctx, d.stdio(), nil, args...)
}

// stdio maps the Definition's stdio overrides into a cli.Stdio (zero values
// inherit os.Stdin/out/err inside cli.Inherit).
func (d *Definition) stdio() cli.Stdio {
	return cli.Stdio{In: d.stdin, Out: d.stdout, Err: d.stderr}
}
```

- [ ] **Step 4: Run to verify it passes**

Run: `go test ./sandbox/ -run 'TestRun_Package|TestDefinition_ToRunArgs' -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add sandbox/run.go sandbox/run_test.go
git commit -m "feat(sandbox): Run (one-shot) + (*Sandbox).Run (re-attach), terminal-inherit" -m "Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

> **Reconciliation note (run flags):** `sbx run` accepts `--clone/--cpus/--kit/--memory/--profile/--template` (verified via `--help`) but `--name` was **not** confirmed for `run`; the package-level `Run` therefore lets the daemon name the sandbox. If a later need arises to own the name for `Run`, verify `sbx run --name` support first.

---

## Phase C — ports (REST + one shell-out)

### Task 6: Ports / PublishPort (REST) + UnpublishPort (shell-out)

**Files:**
- Create: `sandbox/ports.go`
- Test: `sandbox/ports_test.go`

Verified: `GET /sandbox/{name}/ports` → `[]` or `[{host_ip,host_port,protocol,sandbox_port}]`; `POST` takes a JSON **array** of the same shape (additive).

- [ ] **Step 1: Write the failing test (`sandbox/ports_test.go`)**

```go
package sandbox

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestPorts_ListAndPublish(t *testing.T) {
	var published []Port
	c := stubClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/sandbox/s1/ports", r.URL.Path)
		switch r.Method {
		case http.MethodGet:
			w.Write([]byte(`[{"host_ip":"127.0.0.1","host_port":18080,"protocol":"tcp","sandbox_port":8080}]`))
		case http.MethodPost:
			body, _ := io.ReadAll(r.Body)
			require.NoError(t, json.Unmarshal(body, &published))
			w.Write([]byte(`[{"host_ip":"127.0.0.1","host_port":19090,"protocol":"tcp","sandbox_port":9090}]`))
		}
	}))
	sb := NewForTest(c, "s1")

	ports, err := sb.Ports(context.Background())
	require.NoError(t, err)
	require.Len(t, ports, 1)
	require.Equal(t, 8080, ports[0].SandboxPort)
	require.Equal(t, 18080, ports[0].HostPort)

	_, err = sb.PublishPort(context.Background(), Port{SandboxPort: 9090, HostPort: 19090, HostIP: "127.0.0.1", Protocol: "tcp"})
	require.NoError(t, err)
	require.Len(t, published, 1)
	require.Equal(t, 9090, published[0].SandboxPort)
}
```

(`stubClient` and `NewForTest` already exist in the `sandbox` package tests from Plan 1.)

- [ ] **Step 2: Run to verify it fails**

Run: `go test ./sandbox/ -run TestPorts -v`
Expected: FAIL — `Port`/`sb.Ports`/`sb.PublishPort` undefined.

- [ ] **Step 3: Implement `sandbox/ports.go`**

```go
package sandbox

import (
	"context"
	"net/http"

	"github.com/squall-chua/sbx-go-sdk/client"
)

// Port is a published port mapping (host <-> sandbox).
type Port struct {
	HostIP      string `json:"host_ip,omitempty"`
	HostPort    int    `json:"host_port,omitempty"`
	Protocol    string `json:"protocol,omitempty"`
	SandboxPort int    `json:"sandbox_port"`
}

// Ports lists the sandbox's published ports (REST GET /sandbox/{name}/ports).
func (s *Sandbox) Ports(ctx context.Context) ([]Port, error) {
	var ports []Port
	if err := s.cli.Transport().DoJSON(ctx, http.MethodGet, "/sandbox/"+s.info.Name+"/ports", nil, &ports); err != nil {
		return nil, client.MapError("ports", err)
	}
	return ports, nil
}

// PublishPort publishes one port mapping and returns the full published set
// (REST POST /sandbox/{name}/ports; the body is a one-element array — the endpoint
// is additive). A zero HostPort requests an ephemeral host port.
func (s *Sandbox) PublishPort(ctx context.Context, p Port) ([]Port, error) {
	var out []Port
	if err := s.cli.Transport().DoJSON(ctx, http.MethodPost, "/sandbox/"+s.info.Name+"/ports", []Port{p}, &out); err != nil {
		return nil, client.MapError("publish-port", err)
	}
	return out, nil
}

// UnpublishPort removes a published port. No REST unpublish path is confirmed in
// v0.32.0, so this shells out to `sbx ports {name} --unpublish SPEC`, where spec is
// the CLI port spec, e.g. "127.0.0.1:18080:8080/tcp" or "18080:8080".
func (s *Sandbox) UnpublishPort(ctx context.Context, spec string) error {
	r, err := s.cli.Runner()
	if err != nil {
		return err
	}
	_, err = r.Capture(ctx, nil, "ports", s.info.Name, "--unpublish", spec)
	return err
}
```

- [ ] **Step 4: Run to verify it passes**

Run: `go test ./sandbox/ -run TestPorts -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add sandbox/ports.go sandbox/ports_test.go
git commit -m "feat(sandbox): Ports/PublishPort (REST) + UnpublishPort (shell-out)" -m "Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

---

## Phase D — cp (shell-out)

### Task 7: CopyTo / CopyFrom (shell-out `sbx cp`)

**Files:**
- Create: `sandbox/cp.go`
- Test: `sandbox/cp_test.go`

`GET /sandbox/{name}/files` is 501 in v0.32.0; `sbx cp` works. cp shells out. `sbx cp SRC DST` where exactly one side is `SANDBOX:PATH`; flag `-L/--follow-link` (host→sandbox only).

- [ ] **Step 1: Write the failing test (`sandbox/cp_test.go`)**

```go
package sandbox

import (
	"context"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/squall-chua/sbx-go-sdk/client"
)

func clientWithRecordingSbx(t *testing.T, argFile string) *client.Client {
	t.Helper()
	sock := filepath.Join(t.TempDir(), "d.sock")
	l, err := net.Listen("unix", sock)
	require.NoError(t, err)
	srv := &http.Server{Handler: http.NewServeMux()}
	go srv.Serve(l)
	t.Cleanup(func() { srv.Close() })
	bin := filepath.Join(t.TempDir(), "sbx")
	script := "#!/bin/sh\nprintf '%s\\n' \"$*\" >> " + argFile + "\nexit 0\n"
	require.NoError(t, os.WriteFile(bin, []byte(script), 0o755))
	c, err := client.New(context.Background(), client.WithSocketPath(sock), client.WithBinaryPath(bin))
	require.NoError(t, err)
	return c
}

func TestCopyToAndFrom(t *testing.T) {
	argFile := filepath.Join(t.TempDir(), "args.txt")
	c := clientWithRecordingSbx(t, argFile)
	sb := NewForTest(c, "s1")

	require.NoError(t, sb.CopyTo(context.Background(), "/local/a.txt", "/home/user/a.txt", WithFollowSymlinks()))
	require.NoError(t, sb.CopyFrom(context.Background(), "/home/user/out.log", "/local/out.log"))

	data, _ := os.ReadFile(argFile)
	lines := string(data)
	require.Contains(t, lines, "cp -L /local/a.txt s1:/home/user/a.txt")
	require.Contains(t, lines, "cp s1:/home/user/out.log /local/out.log")
}
```

- [ ] **Step 2: Run to verify it fails**

Run: `go test ./sandbox/ -run TestCopyToAndFrom -v`
Expected: FAIL — `sb.CopyTo`/`CopyFrom`/`WithFollowSymlinks` undefined.

- [ ] **Step 3: Implement `sandbox/cp.go`**

```go
package sandbox

import "context"

// copyConfig accumulates cp options.
type copyConfig struct{ followSymlinks bool }

// CopyOption configures a copy.
type CopyOption func(*copyConfig)

// WithFollowSymlinks follows symlinks in the source when copying host -> sandbox
// (`sbx cp -L`).
func WithFollowSymlinks() CopyOption { return func(c *copyConfig) { c.followSymlinks = true } }

// CopyTo copies a host path into the sandbox (`sbx cp [-L] localPath name:sandboxPath`).
func (s *Sandbox) CopyTo(ctx context.Context, localPath, sandboxPath string, opts ...CopyOption) error {
	var cfg copyConfig
	for _, o := range opts {
		o(&cfg)
	}
	args := []string{"cp"}
	if cfg.followSymlinks {
		args = append(args, "-L")
	}
	args = append(args, localPath, s.info.Name+":"+sandboxPath)
	r, err := s.cli.Runner()
	if err != nil {
		return err
	}
	_, err = r.Capture(ctx, nil, args...)
	return err
}

// CopyFrom copies a sandbox path to the host (`sbx cp name:sandboxPath localPath`).
func (s *Sandbox) CopyFrom(ctx context.Context, sandboxPath, localPath string) error {
	r, err := s.cli.Runner()
	if err != nil {
		return err
	}
	_, err = r.Capture(ctx, nil, "cp", s.info.Name+":"+sandboxPath, localPath)
	return err
}
```

- [ ] **Step 4: Run to verify it passes**

Run: `go test ./sandbox/ -run TestCopyToAndFrom -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add sandbox/cp.go sandbox/cp_test.go
git commit -m "feat(sandbox): CopyTo/CopyFrom via shell-out sbx cp (files REST is 501)" -m "Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

---

## Phase E — template (REST + one shell-out)

### Task 8: template.List / Inspect (REST)

**Files:**
- Create: `template/template.go`
- Test: `template/template_test.go`

Verified: `GET /docker/images` → `[{agent,created_at,id,repository,tag}]`; `GET /docker/images/inspect?name=<ref>` → `{agent,created_at,id}`.

- [ ] **Step 1: Write the failing test (`template/template_test.go`)**

```go
package template

import (
	"context"
	"net"
	"net/http"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/squall-chua/sbx-go-sdk/client"
)

func stubClient(t *testing.T, h http.Handler) *client.Client {
	t.Helper()
	sock := filepath.Join(t.TempDir(), "d.sock")
	l, err := net.Listen("unix", sock)
	require.NoError(t, err)
	srv := &http.Server{Handler: h}
	go srv.Serve(l)
	t.Cleanup(func() { srv.Close() })
	c, err := client.New(context.Background(), client.WithSocketPath(sock))
	require.NoError(t, err)
	return c
}

func TestListAndInspect(t *testing.T) {
	c := stubClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/docker/images":
			w.Write([]byte(`[{"agent":"shell-docker","created_at":"2026-06-10T03:28:49Z","id":"sha256:0e27","repository":"docker.io/docker/sandbox-templates","tag":"shell-docker"}]`))
		case "/docker/images/inspect":
			require.Equal(t, "docker.io/docker/sandbox-templates:shell-docker", r.URL.Query().Get("name"))
			w.Write([]byte(`{"agent":"shell-docker","created_at":"2026-06-04T19:58:11Z","id":"sha256:0e27"}`))
		default:
			t.Fatalf("unexpected %s", r.URL.Path)
		}
	}))
	imgs, err := List(context.Background(), c)
	require.NoError(t, err)
	require.Len(t, imgs, 1)
	require.Equal(t, "shell-docker", imgs[0].Tag)
	require.Equal(t, "shell-docker", imgs[0].Agent)

	img, err := Inspect(context.Background(), c, "docker.io/docker/sandbox-templates:shell-docker")
	require.NoError(t, err)
	require.Equal(t, "sha256:0e27", img.ID)
}
```

- [ ] **Step 2: Run to verify it fails**

Run: `go test ./template/ -run TestListAndInspect -v`
Expected: FAIL — undefined symbols.

- [ ] **Step 3: Implement `template/template.go`**

```go
// Package template manages sandbox template images (templates ARE images) over the
// daemon's /docker/images* REST endpoints, plus the shell-out save path on Sandbox.
package template

import (
	"context"
	"net/http"
	"net/url"

	"github.com/squall-chua/sbx-go-sdk/client"
)

// Image is a template image (base or saved). Listing returns the full set.
type Image struct {
	Agent      string `json:"agent"`
	CreatedAt  string `json:"created_at"`
	ID         string `json:"id"`
	Repository string `json:"repository"`
	Tag        string `json:"tag"`
}

// List returns all template images (REST GET /docker/images).
func List(ctx context.Context, c *client.Client) ([]Image, error) {
	var imgs []Image
	if err := c.Transport().DoJSON(ctx, http.MethodGet, "/docker/images", nil, &imgs); err != nil {
		return nil, client.MapError("template-list", err)
	}
	return imgs, nil
}

// Inspect returns a single image by ref (REST GET /docker/images/inspect?name=).
func Inspect(ctx context.Context, c *client.Client, ref string) (Image, error) {
	var img Image
	path := "/docker/images/inspect?name=" + url.QueryEscape(ref)
	if err := c.Transport().DoJSON(ctx, http.MethodGet, path, nil, &img); err != nil {
		return Image{}, client.MapError("template-inspect", err)
	}
	return img, nil
}
```

- [ ] **Step 4: Run to verify it passes**

Run: `go test ./template/ -run TestListAndInspect -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add template/template.go template/template_test.go
git commit -m "feat(template): List + Inspect over /docker/images (REST)" -m "Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

### Task 9: template.Remove / Load (REST) + (*Sandbox).SaveTemplate (shell-out)

**Files:**
- Modify: `template/template.go`
- Create: `sandbox/template_save.go`
- Test: `template/template_test.go`, `sandbox/template_save_test.go`

> **Reconciliation note:** `DELETE /docker/images/remove` (Allow: DELETE) and `POST /docker/images/load` (Allow: POST) are registered, but their exact query-param/body were not fully verified. This task implements `Remove` with `?name=<ref>` (mirroring Inspect) and `Load` with a raw tar body. During the integration pass, verify against the live daemon; if either 4xx/501s, fall back to shell-out `sbx template rm TAG|ID` / `sbx template load FILE` (note the change in the task's reconciliation comment).

- [ ] **Step 1: Write the failing test (append to `template/template_test.go`)**

```go
func TestRemoveAndLoad(t *testing.T) {
	var loadBody []byte
	c := stubClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodDelete && r.URL.Path == "/docker/images/remove":
			require.Equal(t, "myimg:v1", r.URL.Query().Get("name"))
			w.WriteHeader(200)
		case r.Method == http.MethodPost && r.URL.Path == "/docker/images/load":
			loadBody, _ = io.ReadAll(r.Body)
			w.WriteHeader(200)
		default:
			t.Fatalf("unexpected %s %s", r.Method, r.URL.Path)
		}
	}))
	require.NoError(t, Remove(context.Background(), c, "myimg:v1"))
	require.NoError(t, Load(context.Background(), c, strings.NewReader("TARDATA")))
	require.Equal(t, "TARDATA", string(loadBody))
}
```

Add `"io"` and `"strings"` to the `template_test.go` imports.

- [ ] **Step 2: Run to verify it fails**

Run: `go test ./template/ -run TestRemoveAndLoad -v`
Expected: FAIL — `Remove`/`Load` undefined.

- [ ] **Step 3: Add Remove + Load (append to `template/template.go`)**

Add `"io"` and `"github.com/squall-chua/sbx-go-sdk/internal/transport"` to the `template.go` import block. `Remove`/`Load` use the low-level `Transport().Do` (not `DoJSON`) because the bodies aren't JSON, and build a `*transport.HTTPStatusError` on non-2xx so `client.MapError` can map it. Append:

```go
// httpStatus reads+closes resp and returns a transport.HTTPStatusError if the
// status is non-2xx, else nil.
func httpStatus(resp *http.Response) error {
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		return &transport.HTTPStatusError{Status: resp.StatusCode, Body: body}
	}
	return nil
}

// Remove deletes a template image by ref (tag or id). REST DELETE
// /docker/images/remove?name=<ref>.
func Remove(ctx context.Context, c *client.Client, ref string) error {
	path := "/docker/images/remove?name=" + url.QueryEscape(ref)
	resp, err := c.Transport().Do(ctx, http.MethodDelete, path, nil, nil)
	if err != nil {
		return client.MapError("template-remove", err)
	}
	if err := httpStatus(resp); err != nil {
		return client.MapError("template-remove", err)
	}
	return nil
}

// Load imports an image tar into the runtime image store (REST POST
// /docker/images/load with the tar as the request body).
func Load(ctx context.Context, c *client.Client, tar io.Reader) error {
	resp, err := c.Transport().Do(ctx, http.MethodPost, "/docker/images/load", tar, nil)
	if err != nil {
		return client.MapError("template-load", err)
	}
	if err := httpStatus(resp); err != nil {
		return client.MapError("template-load", err)
	}
	return nil
}
```

- [ ] **Step 4: Write the SaveTemplate test (`sandbox/template_save_test.go`)**

```go
package sandbox

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSaveTemplate(t *testing.T) {
	argFile := filepath.Join(t.TempDir(), "args.txt")
	c := clientWithRecordingSbx(t, argFile)
	sb := NewForTest(c, "s1")
	require.NoError(t, sb.SaveTemplate(context.Background(), "myimg:v1"))
	data, _ := os.ReadFile(argFile)
	require.Contains(t, string(data), "template save s1 myimg:v1")
}
```

(`clientWithRecordingSbx` is defined in `sandbox/cp_test.go` from Task 7.)

- [ ] **Step 5: Implement `sandbox/template_save.go`**

```go
package sandbox

import "context"

// SaveTemplate snapshots the sandbox as a reusable template image
// (`sbx template save NAME TAG`). Shell-out (no daemon REST builder).
func (s *Sandbox) SaveTemplate(ctx context.Context, tag string) error {
	r, err := s.cli.Runner()
	if err != nil {
		return err
	}
	_, err = r.Capture(ctx, nil, "template", "save", s.info.Name, tag)
	return err
}
```

- [ ] **Step 6: Run to verify both pass**

Run: `go test ./template/ ./sandbox/ -run 'TestRemoveAndLoad|TestSaveTemplate' -v`
Expected: PASS.

- [ ] **Step 7: Commit**

```bash
git add template/template.go template/template_test.go sandbox/template_save.go sandbox/template_save_test.go
git commit -m "feat(template): Remove/Load (REST) + (*Sandbox).SaveTemplate (shell-out)" -m "Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

---

## Phase F — policy (shell-out + one REST)

### Task 10: policy SetDefault / Allow / Deny / RemoveRule / Reset (shell-out)

**Files:**
- Create: `policy/policy.go`
- Test: `policy/policy_test.go`

`scope` is `""` for global or a sandbox name (→ `--sandbox NAME`). Verified CLI shapes: `sbx policy set-default <allow-all|balanced|deny-all>`; `sbx policy allow|deny network [--sandbox S] HOSTS...`; `sbx policy rm network [--sandbox S]`; `sbx policy reset`.

- [ ] **Step 1: Write the failing test (`policy/policy_test.go`)**

```go
package policy

import (
	"context"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/squall-chua/sbx-go-sdk/client"
)

func recordingClient(t *testing.T, argFile string) *client.Client {
	t.Helper()
	sock := filepath.Join(t.TempDir(), "d.sock")
	l, err := net.Listen("unix", sock)
	require.NoError(t, err)
	srv := &http.Server{Handler: http.NewServeMux()}
	go srv.Serve(l)
	t.Cleanup(func() { srv.Close() })
	bin := filepath.Join(t.TempDir(), "sbx")
	require.NoError(t, os.WriteFile(bin, []byte("#!/bin/sh\nprintf '%s\\n' \"$*\" >> "+argFile+"\nexit 0\n"), 0o755))
	c, err := client.New(context.Background(), client.WithSocketPath(sock), client.WithBinaryPath(bin))
	require.NoError(t, err)
	return c
}

func TestPolicyMutations(t *testing.T) {
	argFile := filepath.Join(t.TempDir(), "args.txt")
	c := recordingClient(t, argFile)
	ctx := context.Background()
	require.NoError(t, SetDefault(ctx, c, "balanced"))
	require.NoError(t, Allow(ctx, c, "", "example.com", "api.github.com"))
	require.NoError(t, Deny(ctx, c, "mysandbox", "evil.example"))
	require.NoError(t, RemoveRule(ctx, c, "mysandbox"))
	require.NoError(t, Reset(ctx, c))
	data, _ := os.ReadFile(argFile)
	lines := string(data)
	require.Contains(t, lines, "policy set-default balanced")
	require.Contains(t, lines, "policy allow network example.com api.github.com")
	require.Contains(t, lines, "policy deny network --sandbox mysandbox evil.example")
	require.Contains(t, lines, "policy rm network --sandbox mysandbox")
	require.Contains(t, lines, "policy reset")
}
```

- [ ] **Step 2: Run to verify it fails**

Run: `go test ./policy/ -run TestPolicyMutations -v`
Expected: FAIL — undefined symbols.

- [ ] **Step 3: Implement `policy/policy.go`**

```go
// Package policy manages sandbox network/egress policies. Rule management is
// engine-layer (no working daemon REST path in v0.32.0), so mutations and listing
// shell out to `sbx policy`; only Log uses REST (GET /network/log).
package policy

import (
	"context"

	"github.com/squall-chua/sbx-go-sdk/client"
)

// scopeArgs appends "--sandbox NAME" when scope is non-empty (global otherwise).
func scopeArgs(scope string) []string {
	if scope == "" {
		return nil
	}
	return []string{"--sandbox", scope}
}

func run(ctx context.Context, c *client.Client, args ...string) error {
	r, err := c.Runner()
	if err != nil {
		return err
	}
	_, err = r.Capture(ctx, nil, args...)
	return err
}

// SetDefault sets the baseline network policy: "allow-all", "balanced", or "deny-all".
func SetDefault(ctx context.Context, c *client.Client, policy string) error {
	return run(ctx, c, "policy", "set-default", policy)
}

// Allow adds an allow rule for the given hosts within scope ("" = global).
func Allow(ctx context.Context, c *client.Client, scope string, hosts ...string) error {
	args := append([]string{"policy", "allow", "network"}, scopeArgs(scope)...)
	return run(ctx, c, append(args, hosts...)...)
}

// Deny adds a deny rule for the given hosts within scope ("" = global).
func Deny(ctx context.Context, c *client.Client, scope string, hosts ...string) error {
	args := append([]string{"policy", "deny", "network"}, scopeArgs(scope)...)
	return run(ctx, c, append(args, hosts...)...)
}

// RemoveRule removes the network rule(s) for scope ("" = global).
func RemoveRule(ctx context.Context, c *client.Client, scope string) error {
	args := append([]string{"policy", "rm", "network"}, scopeArgs(scope)...)
	return run(ctx, c, args...)
}

// Reset clears all policies back to defaults.
func Reset(ctx context.Context, c *client.Client) error {
	return run(ctx, c, "policy", "reset")
}
```

- [ ] **Step 4: Run to verify it passes**

Run: `go test ./policy/ -run TestPolicyMutations -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add policy/policy.go policy/policy_test.go
git commit -m "feat(policy): SetDefault/Allow/Deny/RemoveRule/Reset (shell-out)" -m "Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

### Task 11: policy List / Profiles (shell-out text) + Log (REST)

**Files:**
- Modify: `policy/policy.go`
- Test: `policy/policy_test.go`

`sbx policy ls`/`profile ls` have **no** `--json` → return raw text. `Log` uses REST `GET /network/log` (verified JSON).

- [ ] **Step 1: Write the failing test (append to `policy/policy_test.go`)**

```go
func TestPolicyListProfilesAndLog(t *testing.T) {
	// List/Profiles: capturing runner returns the fake sbx stdout.
	argFile := filepath.Join(t.TempDir(), "args.txt")
	// fake sbx prints a banner to stdout so List returns non-empty text.
	sock := filepath.Join(t.TempDir(), "d.sock")
	l, err := net.Listen("unix", sock)
	require.NoError(t, err)
	srv := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/network/log", r.URL.Path)
		w.Write([]byte(`{"blocked_hosts":[],"allowed_hosts":[{"host":"api.github.com:443","vm_name":"s1","proxy_type":"forward","rule":"domain-allowed","last_seen":"2026-06-10T11:29:10Z","since":"2026-06-10T11:29:10Z","count_since":2}]}`))
	})}
	go srv.Serve(l)
	t.Cleanup(func() { srv.Close() })
	bin := filepath.Join(t.TempDir(), "sbx")
	require.NoError(t, os.WriteFile(bin, []byte("#!/bin/sh\nprintf '%s\\n' \"$*\" >> "+argFile+"\necho POLICY-TEXT\nexit 0\n"), 0o755))
	c, err := client.New(context.Background(), client.WithSocketPath(sock), client.WithBinaryPath(bin))
	require.NoError(t, err)
	ctx := context.Background()

	txt, err := List(ctx, c, "s1")
	require.NoError(t, err)
	require.Contains(t, txt, "POLICY-TEXT")
	data, _ := os.ReadFile(argFile)
	require.Contains(t, string(data), "policy ls s1")

	prof, err := Profiles(ctx, c)
	require.NoError(t, err)
	require.Contains(t, prof, "POLICY-TEXT")

	logs, err := Log(ctx, c)
	require.NoError(t, err)
	require.Len(t, logs.AllowedHosts, 1)
	require.Equal(t, "api.github.com:443", logs.AllowedHosts[0].Host)
}
```

- [ ] **Step 2: Run to verify it fails**

Run: `go test ./policy/ -run TestPolicyListProfilesAndLog -v`
Expected: FAIL — `List`/`Profiles`/`Log` undefined.

- [ ] **Step 3: Implement (append to `policy/policy.go`)**

Add `"net/http"` to the `policy.go` import block, and append:

```go
// capture runs an sbx subcommand and returns its stdout text.
func capture(ctx context.Context, c *client.Client, args ...string) (string, error) {
	r, err := c.Runner()
	if err != nil {
		return "", err
	}
	return r.Capture(ctx, nil, args...)
}

// List returns the raw `sbx policy ls [SCOPE]` text (no --json upstream). scope ""
// lists global+all.
func List(ctx context.Context, c *client.Client, scope string) (string, error) {
	args := []string{"policy", "ls"}
	if scope != "" {
		args = append(args, scope)
	}
	return capture(ctx, c, args...)
}

// Profiles returns the raw `sbx policy profile ls` text.
func Profiles(ctx context.Context, c *client.Client) (string, error) {
	return capture(ctx, c, "policy", "profile", "ls")
}

// LogEntry is one allowed/blocked host record from the proxy.
type LogEntry struct {
	Host       string `json:"host"`
	VMName     string `json:"vm_name"`
	ProxyType  string `json:"proxy_type"`
	Rule       string `json:"rule"`
	LastSeen   string `json:"last_seen"`
	Since      string `json:"since"`
	CountSince int    `json:"count_since"`
}

// PolicyLog is the /network/log response.
type PolicyLog struct {
	BlockedHosts []LogEntry `json:"blocked_hosts"`
	AllowedHosts []LogEntry `json:"allowed_hosts"`
}

// Log returns the proxy's allowed/blocked-host log (REST GET /network/log).
func Log(ctx context.Context, c *client.Client) (*PolicyLog, error) {
	var pl PolicyLog
	if err := c.Transport().DoJSON(ctx, http.MethodGet, "/network/log", nil, &pl); err != nil {
		return nil, client.MapError("policy-log", err)
	}
	return &pl, nil
}
```

- [ ] **Step 4: Run to verify it passes**

Run: `go test ./policy/ -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add policy/policy.go policy/policy_test.go
git commit -m "feat(policy): List/Profiles (shell-out text) + Log (REST /network/log)" -m "Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

---

## Phase G — secret (shell-out)

### Task 12: secret.SetCustom / List / Remove (shell-out)

**Files:**
- Create: `secret/secret.go`
- Test: `secret/secret_test.go`

Verified CLI: `sbx secret set-custom [-g | SANDBOX] --host H --env E --value V [--placeholder P]`; `sbx secret ls [SANDBOX]` (text); `sbx secret rm [-g | SANDBOX] [SERVICE] -f`. `scope` `""` = global → `-g`.

- [ ] **Step 1: Write the failing test (`secret/secret_test.go`)**

```go
package secret

import (
	"context"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/squall-chua/sbx-go-sdk/client"
)

func recordingClient(t *testing.T, argFile string) *client.Client {
	t.Helper()
	sock := filepath.Join(t.TempDir(), "d.sock")
	l, err := net.Listen("unix", sock)
	require.NoError(t, err)
	srv := &http.Server{Handler: http.NewServeMux()}
	go srv.Serve(l)
	t.Cleanup(func() { srv.Close() })
	bin := filepath.Join(t.TempDir(), "sbx")
	require.NoError(t, os.WriteFile(bin, []byte("#!/bin/sh\nprintf '%s\\n' \"$*\" >> "+argFile+"\necho SECRET-TEXT\nexit 0\n"), 0o755))
	c, err := client.New(context.Background(), client.WithSocketPath(sock), client.WithBinaryPath(bin))
	require.NoError(t, err)
	return c
}

func TestSecretOps(t *testing.T) {
	argFile := filepath.Join(t.TempDir(), "args.txt")
	c := recordingClient(t, argFile)
	ctx := context.Background()

	require.NoError(t, SetCustom(ctx, c, "", CustomSecret{Host: "api.example.com", Env: "API_KEY", Value: "sk-123"}))
	txt, err := List(ctx, c, "")
	require.NoError(t, err)
	require.Contains(t, txt, "SECRET-TEXT")
	require.NoError(t, Remove(ctx, c, "mysandbox", "openai"))

	data, _ := os.ReadFile(argFile)
	lines := string(data)
	require.Contains(t, lines, "secret set-custom -g --host api.example.com --env API_KEY --value sk-123")
	require.Contains(t, lines, "secret ls")
	require.Contains(t, lines, "secret rm mysandbox openai -f")
}
```

- [ ] **Step 2: Run to verify it fails**

Run: `go test ./secret/ -run TestSecretOps -v`
Expected: FAIL — undefined symbols.

- [ ] **Step 3: Implement `secret/secret.go`**

```go
// Package secret manages stored sandbox secrets via shell-out to `sbx secret`.
// For headless agent credentials, prefer exec.WithEnv; SetCustom is EXPERIMENTAL
// upstream.
package secret

import (
	"context"

	"github.com/squall-chua/sbx-go-sdk/client"
)

// CustomSecret describes a custom proxy-injected secret.
type CustomSecret struct {
	Host        string // target host whose outbound requests get the real secret
	Env         string // env var set (to the placeholder) inside the sandbox
	Value       string // the real secret
	Placeholder string // optional; supports a {rand} suffix
}

// scopeFlag returns "-g" for global ("") or the sandbox name as a positional arg.
func scopeArg(scope string) string {
	if scope == "" {
		return "-g"
	}
	return scope
}

// SetCustom creates/updates a custom secret in scope ("" = global). EXPERIMENTAL.
func SetCustom(ctx context.Context, c *client.Client, scope string, s CustomSecret) error {
	args := []string{"secret", "set-custom", scopeArg(scope), "--host", s.Host, "--env", s.Env, "--value", s.Value}
	if s.Placeholder != "" {
		args = append(args, "--placeholder", s.Placeholder)
	}
	r, err := c.Runner()
	if err != nil {
		return err
	}
	_, err = r.Capture(ctx, nil, args...)
	return err
}

// List returns the raw `sbx secret ls [SCOPE]` text (no --json upstream).
func List(ctx context.Context, c *client.Client, scope string) (string, error) {
	args := []string{"secret", "ls"}
	if scope != "" {
		args = append(args, scope)
	}
	r, err := c.Runner()
	if err != nil {
		return "", err
	}
	return r.Capture(ctx, nil, args...)
}

// Remove deletes a secret (service) in scope ("" = global). Uses -f to skip the
// confirmation prompt (the CLI would otherwise block on non-TTY stdin).
func Remove(ctx context.Context, c *client.Client, scope, service string) error {
	args := []string{"secret", "rm", scopeArg(scope)}
	if service != "" {
		args = append(args, service)
	}
	args = append(args, "-f")
	r, err := c.Runner()
	if err != nil {
		return err
	}
	_, err = r.Capture(ctx, nil, args...)
	return err
}
```

Note: for global scope `Remove(ctx, c, "", "github")` produces `secret rm -g github -f`; the test uses a sandbox scope (`mysandbox openai`). Both are valid per `sbx secret rm --help`.

- [ ] **Step 4: Run to verify it passes**

Run: `go test ./secret/ -run TestSecretOps -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add secret/secret.go secret/secret_test.go
git commit -m "feat(secret): SetCustom/List/Remove (shell-out)" -m "Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

---

## Phase H — integration + wrap-up

### Task 13: live resources integration smoke test

**Files:**
- Create: `internal/integration/resources_test.go`

Exercises the verified REST resource paths end-to-end against the live daemon: create a `shell` sandbox, publish + list a port, cp a file in and back out, list templates, then remove. Build-tagged; provisions a real micro-VM (~20–30s).

- [ ] **Step 1: Write the build-tagged test**

```go
//go:build integration

package integration

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/squall-chua/sbx-go-sdk/client"
	"github.com/squall-chua/sbx-go-sdk/sandbox"
	"github.com/squall-chua/sbx-go-sdk/template"
)

func TestSmoke_PortsCpTemplate(t *testing.T) {
	ctx := context.Background()
	c, err := client.New(ctx, client.WithAutoStart())
	require.NoError(t, err)

	sb, err := sandbox.Create(ctx, c, sandbox.WithAgent("shell"), sandbox.WithWorkspace(t.TempDir()))
	require.NoError(t, err)
	t.Cleanup(func() { sb.Remove(ctx) })

	// ports: publish + list
	_, err = sb.PublishPort(ctx, sandbox.Port{SandboxPort: 8080, HostPort: 0, HostIP: "127.0.0.1", Protocol: "tcp"})
	require.NoError(t, err)
	ports, err := sb.Ports(ctx)
	require.NoError(t, err)
	require.NotEmpty(t, ports)

	// cp: host -> sandbox -> host round-trip
	dir := t.TempDir()
	src := filepath.Join(dir, "in.txt")
	require.NoError(t, os.WriteFile(src, []byte("sdk-cp-roundtrip"), 0o644))
	require.NoError(t, sb.CopyTo(ctx, src, "/tmp/in.txt"))
	dst := filepath.Join(dir, "out.txt")
	require.NoError(t, sb.CopyFrom(ctx, "/tmp/in.txt", dst))
	got, _ := os.ReadFile(dst)
	require.Equal(t, "sdk-cp-roundtrip", string(got))

	// templates: list (base images always present)
	imgs, err := template.List(ctx, c)
	require.NoError(t, err)
	require.NotEmpty(t, imgs)
}
```

- [ ] **Step 2: Run it against the live daemon**

Run: `go test -tags integration ./internal/integration/ -run TestSmoke_PortsCpTemplate -v -timeout 300s`
Expected: PASS. If `Ports`/`PublishPort`/`cp`/`template.List` drift from the verified shapes, the failure pinpoints the layer. Verify no leftover sandbox afterward: `sbx ls` (cleanup is via `t.Cleanup`/`sb.Remove`; if one remains, `sbx rm --force <name>`).

- [ ] **Step 3: Confirm the no-tag build still skips it**

Run: `go test ./internal/integration/`
Expected: `[no test files]` (or skip) — the tag excludes it.

- [ ] **Step 4: Commit**

```bash
git add internal/integration/resources_test.go
git commit -m "test(integration): live ports/cp/template smoke test" -m "Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

### Task 14: module gate + README update

**Files:**
- Modify: `README.md`

- [ ] **Step 1: Run the full gate**

Run: `go build ./... && go vet ./... && go test ./... && gofmt -l .`
Expected: build/vet/test all PASS; `gofmt -l .` prints nothing.

- [ ] **Step 2: Extend the README with the Plan 2 surface**

Append to `README.md`:

```markdown

## Resources (Plan 2)

```go
// Interactive agent (terminal-inherit):
code, _ := sandbox.Run(ctx, c, sandbox.WithAgent("claude"), sandbox.WithWorkspace("."))

// Ports, files, templates:
sb.PublishPort(ctx, sandbox.Port{SandboxPort: 8080, HostIP: "127.0.0.1", Protocol: "tcp"})
ports, _ := sb.Ports(ctx)
sb.CopyTo(ctx, "./config.json", "/home/user/config.json")
sb.CopyFrom(ctx, "/home/user/out.log", "./out.log")
imgs, _ := template.List(ctx, c)
sb.SaveTemplate(ctx, "myimg:v1")

// Network policy + secrets:
policy.SetDefault(ctx, c, "balanced")
policy.Allow(ctx, c, "", "api.github.com")
log, _ := policy.Log(ctx)
secret.SetCustom(ctx, c, "", secret.CustomSecret{Host: "api.example.com", Env: "API_KEY", Value: "..."})
```

Verified deviations vs. the design spec: cp is shell-out (daemon `/files` GET is 501);
`policy`/`secret` list output is text. See the Plan 2 doc for details.
```

- [ ] **Step 3: Commit**

```bash
git add README.md
git commit -m "docs: document Plan 2 resource surface (run/ports/cp/template/policy/secret)" -m "Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

---

## Self-Review Notes (for the implementer)

- **No `sb.Exec` forwarders.** Adding `sb.Exec`/`sb.ExecInteractive` to the `sandbox` package would require `sandbox`→`exec` (a cycle, since `exec`→`sandbox`). The exec API stays package-level: `exec.Exec(ctx, sb, …)`. `sb.Run` is fine because it's shell-out (no `exec` dependency).
- **Spec revisions (verified live, authoritative):** cp = shell-out (`/files` GET → 501); `policy`/`secret` list = text (no `--json`); `policy.Log`/ports/template-list+inspect = verified REST. Don't "restore" the spec's REST-tar cp.
- **Reconciliation flags:** `template.Remove`/`Load` REST param/body and `sbx run --name` support were not fully verified — Tasks 9 and 5 carry notes; the integration test (Task 13) exercises ports/cp/template-list but not save/load/remove (those are heavier; verify manually if needed).
- **Internal-type leak (pre-existing):** `sb.Inspect` returns `internal/api.SandboxInfo`, which external importers can't name. Plan 2 avoids compounding this (`Port`, `template.Image`, `policy.PolicyLog` are public). A future cleanup could re-export a public `SandboxInfo`; out of scope here.
- **Error mapping:** `ensureRunnable` adds one inspect round-trip per exec (the spec's precondition). `mapExecError` maps exec-404 → `ErrExecNotFound`; sandbox-404 still maps to `ErrSandboxNotFound` via `client.MapError`.

## Out of scope (future)

- REST tar cp core (`CopyTarTo`/`CopyTarFrom`) — blocked on daemon `GET /files` (currently 501).
- Built-in-service OAuth `secret set SERVICE`, registry credentials, `POST /sandbox/{name}/credentials`.
- Structured parsing of `policy ls`/`secret ls` text; a public `SandboxInfo` re-export; an `image` package for raw image management.
