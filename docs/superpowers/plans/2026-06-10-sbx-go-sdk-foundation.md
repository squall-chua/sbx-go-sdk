# sbx-go-sdk Foundation Implementation Plan (Plan 1 of 2)

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build the foundation and core automation loop of a Go SDK for Docker Sandboxes (`sbx`): resolve/manage the `sandboxd` daemon, create sandboxes, manage their lifecycle, and exec commands (capture / interactive / detached) — enough to automate `create → exec → remove`.

**Architecture:** Hybrid transport (per [ADR-0001](../../adr/0001-hybrid-cli-shellout-plus-rest.md)): shell out to the `sbx` binary for orchestration-heavy ops (`Create`, daemon `start`), use the daemon's REST/HTTP-over-unix-socket API for runtime ops (list/inspect/start/stop/remove, exec). Layered packages: `internal/transport` (unix HTTP + hijack), `internal/cli` (sbx-binary driver), `internal/stdcopy` (Docker stream demux), `internal/api` (DWARF-extracted structs), and public `client` / `sandbox` / `exec` packages following the `docker/go-sdk` functional-options idiom.

**Tech Stack:** Go 1.25, stdlib only for the SDK (`net`, `net/http`, `debug/elf`, `debug/dwarf`, `os/exec`, `encoding/json`); `testify` for tests. No `moby/moby` dependency (internal stdcopy demuxer instead).

**Spec:** [docs/superpowers/specs/2026-06-10-sbx-go-sdk-design.md](../specs/2026-06-10-sbx-go-sdk-design.md). **Module:** `github.com/squall-chua/sbx-go-sdk`.

**Out of scope for Plan 1** (→ Plan 2): cp/files, ports, template, policy, secret. Those reuse the transport + cli driver built here.

---

## File Structure

| Path | Responsibility |
|---|---|
| `go.mod`, `go.sum` | Module `github.com/squall-chua/sbx-go-sdk`, Go 1.25, testify dep |
| `internal/tools/dwarfgen/main.go` | One-shot: walk `sbx` binary DWARF, emit `internal/api/types_gen.go` |
| `internal/api/types_gen.go` | Generated REST request/response structs (committed) |
| `internal/api/types_validate_test.go` | Round-trip test: real daemon JSON ↔ generated structs (drift detector) |
| `internal/stdcopy/stdcopy.go` | Docker 8-byte-frame stream demuxer (stdout/stderr) |
| `internal/stdcopy/stdcopy_test.go` | Demuxer tests |
| `internal/transport/socket.go` | Socket path resolution (`DOCKER_SANDBOXES_API` > default XDG) |
| `internal/transport/transport.go` | `http.Client` over a unix socket; `Do`, JSON helpers |
| `internal/transport/hijack.go` | Connection-hijack (HTTP 101 Upgrade) for exec attach |
| `internal/transport/*_test.go` | Stub-unix-socket HTTP server tests |
| `internal/cli/cli.go` | `sbx` binary discovery + command runner (capture / streamed / TTY-inherit) |
| `internal/cli/cli_test.go` | Fake-`sbx`-on-PATH tests |
| `client/errors.go` | `APIError`, `CLIError`, sentinels |
| `client/options.go` | `Option`, `With*` (socket path, binary path, version, http timeout) |
| `client/client.go` | `Client`, `New`, `DefaultClient`, low-level `Do` accessor |
| `client/daemon.go` | Health, Version, Info, DaemonHealth, LogLevels/Set, Diagnostics, lifecycle |
| `client/*_test.go` | Client + daemon tests against the stub server / fake binary |
| `sandbox/definition.go` | `Definition` (agent, workspaces, cpus, …) + `Option` |
| `sandbox/options.go` | `WithAgent`, `WithWorkspace`, `WithName`, `WithCPUs`, … |
| `sandbox/sandbox.go` | `Sandbox` handle (`Name()`, `ID()`, `State()`, `Inspect`) |
| `sandbox/list.go` | `List`, `Get` (REST) |
| `sandbox/lifecycle.go` | `Create` (shell-out), `Start`/`Stop`/`Remove` (REST) |
| `sandbox/*_test.go` | Sandbox tests |
| `exec/options.go` | `ProcessOption`, `WithEnv`/`WithWorkdir`/`WithUser`/`WithTTY`/… |
| `exec/exec.go` | `Exec` (capture), `ExecDetached`, `InspectExec` |
| `exec/attach.go` | `ExecInteractive`, `AttachSession` (Resize/Wait/Close) |
| `exec/*_test.go` | Exec tests against the stub server |

`sandbox` and `exec` methods hang off `*sandbox.Sandbox`; the `exec` package defines the methods on `Sandbox` via a shared interface to avoid an import cycle (see Task 19).

---

## Phase 0 — Scaffold

### Task 1: Initialize the module

**Files:**
- Create: `go.mod`
- Create: `.gitignore`

- [ ] **Step 1: Init module and add testify**

```bash
cd /home/mwchua/sbx-go-sdk
go mod init github.com/squall-chua/sbx-go-sdk
go get github.com/stretchr/testify@v1.10.0
```

- [ ] **Step 2: Pin Go version in go.mod**

Ensure `go.mod` contains:

```
module github.com/squall-chua/sbx-go-sdk

go 1.25
```

- [ ] **Step 3: Add .gitignore**

```
# build artifacts
/bin/
*.test
*.out
```

- [ ] **Step 4: Verify it builds**

Run: `go build ./... && go vet ./...`
Expected: no output, exit 0 (no packages yet is fine).

- [ ] **Step 5: Commit**

```bash
git add go.mod go.sum .gitignore
git commit -m "chore: initialize go module github.com/squall-chua/sbx-go-sdk"
```

---

## Phase 1 — internal/stdcopy (Docker stream demuxer)

The attach stream is Docker's multiplexed format: repeating frames of `[streamType(1), 0,0,0, size(uint32 big-endian)][payload]`. `streamType` 1 = stdout, 2 = stderr. Verified on the wire: `\x01\x00\x00\x00\x00\x00\x00\x10hello-from-exec\n`.

### Task 2: stdcopy demuxer

**Files:**
- Create: `internal/stdcopy/stdcopy.go`
- Test: `internal/stdcopy/stdcopy_test.go`

- [ ] **Step 1: Write the failing test**

```go
package stdcopy

import (
	"bytes"
	"encoding/binary"
	"testing"

	"github.com/stretchr/testify/require"
)

func frame(t byte, payload string) []byte {
	h := make([]byte, 8)
	h[0] = t
	binary.BigEndian.PutUint32(h[4:], uint32(len(payload)))
	return append(h, []byte(payload)...)
}

func TestDemux_SplitsStdoutStderr(t *testing.T) {
	var src bytes.Buffer
	src.Write(frame(1, "out1"))
	src.Write(frame(2, "err1"))
	src.Write(frame(1, "out2"))

	var out, errb bytes.Buffer
	n, err := Demux(&out, &errb, &src)
	require.NoError(t, err)
	require.Equal(t, int64(12), n)
	require.Equal(t, "out1out2", out.String())
	require.Equal(t, "err1", errb.String())
}

func TestDemux_HandlesPayloadSpanningReads(t *testing.T) {
	// a frame whose payload is larger than the internal buffer chunk
	big := bytes.Repeat([]byte("x"), 70000)
	var src bytes.Buffer
	src.Write(frame(1, string(big)))
	var out, errb bytes.Buffer
	_, err := Demux(&out, &errb, &src)
	require.NoError(t, err)
	require.Equal(t, 70000, out.Len())
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/stdcopy/ -run TestDemux -v`
Expected: FAIL — `undefined: Demux`.

- [ ] **Step 3: Write minimal implementation**

```go
// Package stdcopy demultiplexes Docker's multiplexed stream format used by the
// sandboxd exec/attach endpoint: repeating [type, 0,0,0, size_be32][payload]
// frames where type 1 = stdout, 2 = stderr. This mirrors moby's stdcopy without
// pulling in the moby/moby dependency.
package stdcopy

import (
	"encoding/binary"
	"errors"
	"io"
)

const (
	stdin  = 0
	stdout = 1
	stderr = 2
	hdrLen = 8
)

// Demux reads the multiplexed stream from src, writing stdout frames to outW and
// stderr frames to errW. It returns the total number of payload bytes written and
// the first non-EOF error encountered. It returns nil error on clean EOF.
func Demux(outW, errW io.Writer, src io.Reader) (int64, error) {
	var written int64
	hdr := make([]byte, hdrLen)
	buf := make([]byte, 32*1024)
	for {
		if _, err := io.ReadFull(src, hdr); err != nil {
			if err == io.EOF || err == io.ErrUnexpectedEOF {
				return written, nil
			}
			return written, err
		}
		var dst io.Writer
		switch hdr[0] {
		case stdin, stdout:
			dst = outW
		case stderr:
			dst = errW
		default:
			return written, errors.New("stdcopy: unknown stream type")
		}
		size := int64(binary.BigEndian.Uint32(hdr[4:8]))
		for size > 0 {
			chunk := int64(len(buf))
			if size < chunk {
				chunk = size
			}
			n, err := src.Read(buf[:chunk])
			if n > 0 {
				if _, werr := dst.Write(buf[:n]); werr != nil {
					return written, werr
				}
				written += int64(n)
				size -= int64(n)
			}
			if err != nil {
				if err == io.EOF && size > 0 {
					return written, io.ErrUnexpectedEOF
				}
				if err == io.EOF {
					return written, nil
				}
				return written, err
			}
		}
	}
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/stdcopy/ -v`
Expected: PASS (both tests).

- [ ] **Step 5: Commit**

```bash
git add internal/stdcopy/
git commit -m "feat(stdcopy): add Docker multiplexed-stream demuxer"
```

---

## Phase 2 — internal/transport (socket resolution, unix HTTP, hijack)

### Task 3: Socket path resolution

**Files:**
- Create: `internal/transport/socket.go`
- Test: `internal/transport/socket_test.go`

- [ ] **Step 1: Write the failing test**

```go
package transport

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestResolveSocketPath_EnvOverride(t *testing.T) {
	t.Setenv("DOCKER_SANDBOXES_API", "/custom/sandboxd.sock")
	got, err := ResolveSocketPath("")
	require.NoError(t, err)
	require.Equal(t, "/custom/sandboxd.sock", got)
}

func TestResolveSocketPath_ExplicitWins(t *testing.T) {
	t.Setenv("DOCKER_SANDBOXES_API", "/env.sock")
	got, err := ResolveSocketPath("/explicit.sock")
	require.NoError(t, err)
	require.Equal(t, "/explicit.sock", got)
}

func TestResolveSocketPath_DefaultXDG(t *testing.T) {
	t.Setenv("DOCKER_SANDBOXES_API", "")
	t.Setenv("XDG_STATE_HOME", "/home/u/.local/state")
	got, err := ResolveSocketPath("")
	require.NoError(t, err)
	want := filepath.Join("/home/u/.local/state", "sandboxes", "sandboxes", "sandboxd", "sandboxd.sock")
	require.Equal(t, want, got)
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/transport/ -run TestResolveSocketPath -v`
Expected: FAIL — `undefined: ResolveSocketPath`.

- [ ] **Step 3: Write minimal implementation**

```go
package transport

import (
	"os"
	"path/filepath"
)

// EnvSocket is the env var sandboxlib.SocketPath reads to override the socket path.
const EnvSocket = "DOCKER_SANDBOXES_API"

// ResolveSocketPath returns the daemon socket path. Precedence:
// explicit arg > $DOCKER_SANDBOXES_API > $XDG_STATE_HOME/.../sandboxd.sock
// (default XDG_STATE_HOME is ~/.local/state).
func ResolveSocketPath(explicit string) (string, error) {
	if explicit != "" {
		return explicit, nil
	}
	if v := os.Getenv(EnvSocket); v != "" {
		return v, nil
	}
	state := os.Getenv("XDG_STATE_HOME")
	if state == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		state = filepath.Join(home, ".local", "state")
	}
	return filepath.Join(state, "sandboxes", "sandboxes", "sandboxd", "sandboxd.sock"), nil
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/transport/ -run TestResolveSocketPath -v`
Expected: PASS (3 tests).

- [ ] **Step 5: Commit**

```bash
git add internal/transport/socket.go internal/transport/socket_test.go
git commit -m "feat(transport): resolve daemon socket path (env + XDG default)"
```

### Task 4: Unix-socket HTTP transport

**Files:**
- Create: `internal/transport/transport.go`
- Test: `internal/transport/transport_test.go`

- [ ] **Step 1: Write the failing test (stub unix-socket server)**

```go
package transport

import (
	"context"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

// startStub serves handler on a temp unix socket and returns its path.
func startStub(t *testing.T, handler http.Handler) string {
	t.Helper()
	dir := t.TempDir()
	sock := filepath.Join(dir, "d.sock")
	l, err := net.Listen("unix", sock)
	require.NoError(t, err)
	srv := &http.Server{Handler: handler}
	go srv.Serve(l)
	t.Cleanup(func() { srv.Close(); os.Remove(sock) })
	return sock
}

func TestTransport_GetJSON(t *testing.T) {
	sock := startStub(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/health", r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"status":"healthy"}`))
	}))
	tr := New(sock)
	var out struct {
		Status string `json:"status"`
	}
	err := tr.DoJSON(context.Background(), http.MethodGet, "/health", nil, &out)
	require.NoError(t, err)
	require.Equal(t, "healthy", out.Status)
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/transport/ -run TestTransport -v`
Expected: FAIL — `undefined: New`.

- [ ] **Step 3: Write minimal implementation**

```go
package transport

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"time"
)

// Transport is an HTTP client bound to a single unix-domain socket.
type Transport struct {
	socket string
	hc     *http.Client
}

// New returns a Transport that dials the given unix socket for every request.
func New(socket string) *Transport {
	return &Transport{
		socket: socket,
		hc: &http.Client{
			Transport: &http.Transport{
				DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
					var d net.Dialer
					return d.DialContext(ctx, "unix", socket)
				},
				DisableCompression: true,
			},
		},
	}
}

// SetTimeout sets the per-request timeout (0 = none).
func (t *Transport) SetTimeout(d time.Duration) { t.hc.Timeout = d }

// Socket returns the unix socket path this transport dials.
func (t *Transport) Socket() string { return t.socket }

// Do issues an HTTP request. The URL host is a placeholder ("sandboxd"); only the
// path matters because every connection goes to the bound socket.
func (t *Transport) Do(ctx context.Context, method, path string, body io.Reader, hdr http.Header) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, method, "http://sandboxd"+path, body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "sbx-go-sdk")
	for k, vs := range hdr {
		for _, v := range vs {
			req.Header.Add(k, v)
		}
	}
	return t.hc.Do(req)
}

// DoJSON sends an optional JSON body and decodes a JSON response into out (if non-nil).
// It returns the raw status and body to the caller via error on non-2xx.
func (t *Transport) DoJSON(ctx context.Context, method, path string, in, out any) error {
	var rdr io.Reader
	hdr := http.Header{}
	if in != nil {
		b, err := json.Marshal(in)
		if err != nil {
			return err
		}
		rdr = bytes.NewReader(b)
		hdr.Set("Content-Type", "application/json")
	}
	resp, err := t.Do(ctx, method, path, rdr, hdr)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return &HTTPStatusError{Status: resp.StatusCode, Body: data}
	}
	if out != nil && len(data) > 0 {
		return json.Unmarshal(data, out)
	}
	return nil
}

// HTTPStatusError carries a non-2xx response for the client layer to map to typed errors.
type HTTPStatusError struct {
	Status int
	Body   []byte
}

func (e *HTTPStatusError) Error() string {
	return fmt.Sprintf("sbx daemon returned HTTP %d: %s", e.Status, bytes.TrimSpace(e.Body))
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/transport/ -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/transport/transport.go internal/transport/transport_test.go
git commit -m "feat(transport): unix-socket HTTP client with JSON helper"
```

### Task 5: Connection hijack for exec attach

The attach endpoint upgrades the connection (HTTP 101). We must take over the raw `net.Conn`. `http.Client` won't surface the hijacked conn, so we hand-write the request/response over a raw dialed conn.

**Files:**
- Create: `internal/transport/hijack.go`
- Test: `internal/transport/hijack_test.go`

- [ ] **Step 1: Write the failing test**

```go
package transport

import (
	"bufio"
	"context"
	"net"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestHijack_UpgradesAndReturnsConn(t *testing.T) {
	dir := t.TempDir()
	sock := dir + "/d.sock"
	l, err := net.Listen("unix", sock)
	require.NoError(t, err)
	t.Cleanup(func() { l.Close() })

	go func() {
		c, _ := l.Accept()
		br := bufio.NewReader(c)
		// read request line + headers until blank line
		for {
			line, _ := br.ReadString('\n')
			if line == "\r\n" || line == "" {
				break
			}
		}
		c.Write([]byte("HTTP/1.1 101 Switching Protocols\r\n" +
			"Sandboxes-Exec-Id: exec123\r\n" +
			"Connection: Upgrade\r\nUpgrade: tcp\r\n\r\n"))
		c.Write([]byte("payload-bytes"))
	}()

	tr := New(sock)
	conn, hdr, err := tr.Hijack(context.Background(), "/sandbox/x/exec/attach", []byte(`{"cmd":["echo"]}`))
	require.NoError(t, err)
	defer conn.Close()
	require.Equal(t, "exec123", hdr.Get("Sandboxes-Exec-Id"))
	got := make([]byte, len("payload-bytes"))
	_, err = conn.Read(got)
	require.NoError(t, err)
	require.Equal(t, "payload-bytes", string(got))
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/transport/ -run TestHijack -v`
Expected: FAIL — `tr.Hijack undefined`.

- [ ] **Step 3: Write minimal implementation**

```go
package transport

import (
	"bufio"
	"context"
	"fmt"
	"net"
	"net/http"
)

// Hijack POSTs body to path with Connection: Upgrade / Upgrade: tcp, reads the
// 101 response, and returns the raw connection (positioned at the start of the
// stream body) plus the response headers (which carry Sandboxes-Exec-Id).
// The caller owns conn and must Close it.
func (t *Transport) Hijack(ctx context.Context, path string, jsonBody []byte) (net.Conn, http.Header, error) {
	var d net.Dialer
	conn, err := d.DialContext(ctx, "unix", t.socket)
	if err != nil {
		return nil, nil, err
	}
	if dl, ok := ctx.Deadline(); ok {
		conn.SetDeadline(dl)
	}
	req := fmt.Sprintf("POST %s HTTP/1.1\r\nHost: sandboxd\r\n"+
		"User-Agent: sbx-go-sdk\r\nContent-Type: application/json\r\n"+
		"Connection: Upgrade\r\nUpgrade: tcp\r\nContent-Length: %d\r\n\r\n",
		path, len(jsonBody))
	if _, err := conn.Write(append([]byte(req), jsonBody...)); err != nil {
		conn.Close()
		return nil, nil, err
	}
	br := bufio.NewReader(conn)
	resp, err := http.ReadResponse(br, &http.Request{Method: http.MethodPost})
	if err != nil {
		conn.Close()
		return nil, nil, err
	}
	if resp.StatusCode != http.StatusSwitchingProtocols {
		conn.Close()
		return nil, nil, fmt.Errorf("attach: expected 101, got %d", resp.StatusCode)
	}
	// Reset the deadline so the stream isn't killed by the handshake deadline.
	conn.SetDeadline(time.Time{})
	// bufio may have buffered stream bytes after the headers; wrap so they aren't lost.
	return &bufferedConn{Conn: conn, r: br}, resp.Header, nil
}

// bufferedConn returns any bytes the header reader already buffered before
// falling through to the underlying conn.
type bufferedConn struct {
	net.Conn
	r *bufio.Reader
}

func (b *bufferedConn) Read(p []byte) (int, error) { return b.r.Read(p) }
```

Note: add `"time"` to the import block of `hijack.go`.

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/transport/ -run TestHijack -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/transport/hijack.go internal/transport/hijack_test.go
git commit -m "feat(transport): HTTP 101 hijack for exec attach"
```

---

## Phase 3 — internal/cli (sbx binary driver)

### Task 6: Binary discovery + command runner

**Files:**
- Create: `internal/cli/cli.go`
- Test: `internal/cli/cli_test.go`

- [ ] **Step 1: Write the failing test (fake sbx on PATH)**

```go
package cli

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

// fakeSbx writes a fake `sbx` script that echoes its args and exits with `code`.
func fakeSbx(t *testing.T, body string) string {
	t.Helper()
	dir := t.TempDir()
	p := filepath.Join(dir, "sbx")
	require.NoError(t, os.WriteFile(p, []byte("#!/bin/sh\n"+body), 0o755))
	return p
}

func TestRunner_Capture_Success(t *testing.T) {
	bin := fakeSbx(t, `echo "created $3"; exit 0`)
	r, err := NewRunner(bin)
	require.NoError(t, err)
	out, err := r.Capture(context.Background(), nil, "create", "shell", "myws", "--name", "n1")
	require.NoError(t, err)
	require.Contains(t, out, "created myws")
}

func TestRunner_Capture_NonZeroIsCLIError(t *testing.T) {
	bin := fakeSbx(t, `echo "boom" 1>&2; exit 3`)
	r, _ := NewRunner(bin)
	_, err := r.Capture(context.Background(), nil, "create", "shell", ".")
	require.Error(t, err)
	var ce *Error
	require.ErrorAs(t, err, &ce)
	require.Equal(t, 3, ce.ExitCode)
	require.Contains(t, ce.Stderr, "boom")
}

func TestNewRunner_MissingBinary(t *testing.T) {
	_, err := NewRunner("/no/such/sbx")
	require.ErrorIs(t, err, ErrBinaryNotFound)
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/cli/ -v`
Expected: FAIL — `undefined: NewRunner`.

- [ ] **Step 3: Write minimal implementation**

```go
// Package cli drives the `sbx` binary for orchestration-heavy operations that
// have no daemon REST path (sandbox create, daemon start, etc.).
package cli

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
)

// ErrBinaryNotFound is returned when the sbx binary cannot be located.
var ErrBinaryNotFound = errors.New("sbx binary not found")

// Error is a non-zero exit from an sbx shell-out.
type Error struct {
	Args     []string
	ExitCode int
	Stderr   string
}

func (e *Error) Error() string {
	return fmt.Sprintf("sbx %v failed (exit %d): %s", e.Args, e.ExitCode, e.Stderr)
}

// Runner runs the resolved sbx binary.
type Runner struct{ bin string }

// NewRunner resolves the binary: if path is set it must exist; otherwise PATH
// is searched for "sbx".
func NewRunner(path string) (*Runner, error) {
	if path != "" {
		if _, err := os.Stat(path); err != nil {
			return nil, fmt.Errorf("%w: %s", ErrBinaryNotFound, path)
		}
		return &Runner{bin: path}, nil
	}
	p, err := exec.LookPath("sbx")
	if err != nil {
		return nil, ErrBinaryNotFound
	}
	return &Runner{bin: p}, nil
}

// Bin returns the resolved binary path.
func (r *Runner) Bin() string { return r.bin }

// Capture runs `sbx args...` with extra env (KEY=VALUE), inheriting os.Environ,
// and returns combined stdout. Non-zero exit yields *Error.
func (r *Runner) Capture(ctx context.Context, extraEnv []string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, r.bin, args...)
	cmd.Env = append(os.Environ(), extraEnv...)
	cmd.Cancel = func() error { return cmd.Process.Signal(os.Interrupt) }
	var out, errb bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errb
	err := cmd.Run()
	if err != nil {
		var ee *exec.ExitError
		if errors.As(err, &ee) {
			return out.String(), &Error{Args: args, ExitCode: ee.ExitCode(), Stderr: errb.String()}
		}
		return out.String(), &Error{Args: args, ExitCode: -1, Stderr: err.Error()}
	}
	return out.String(), nil
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/cli/ -v`
Expected: PASS (3 tests).

- [ ] **Step 5: Commit**

```bash
git add internal/cli/
git commit -m "feat(cli): sbx binary discovery + capturing command runner"
```

### Task 7: Interactive (terminal-inherit) runner

**Files:**
- Modify: `internal/cli/cli.go`
- Test: `internal/cli/cli_test.go`

- [ ] **Step 1: Write the failing test**

```go
func TestRunner_Inherit_ReturnsExitCode(t *testing.T) {
	bin := fakeSbx(t, `exit 7`)
	r, _ := NewRunner(bin)
	code, err := r.Inherit(context.Background(), Stdio{}, nil, "run", "shell")
	require.NoError(t, err) // non-zero exit is reported via code, not err
	require.Equal(t, 7, code)
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/cli/ -run TestRunner_Inherit -v`
Expected: FAIL — `undefined: Stdio` / `r.Inherit undefined`.

- [ ] **Step 3: Add the Inherit runner (append to cli.go)**

```go
import "io" // add to the existing import block

// Stdio overrides the child's stdio; zero values inherit os.Stdin/out/err.
type Stdio struct {
	In  io.Reader
	Out io.Writer
	Err io.Writer
}

// Inherit runs `sbx args...` wired to the given (or inherited) terminal stdio and
// returns the child's exit code. A non-zero exit is returned as (code, nil); only
// failures to start/wait are returned as a non-nil error.
func (r *Runner) Inherit(ctx context.Context, s Stdio, extraEnv []string, args ...string) (int, error) {
	cmd := exec.CommandContext(ctx, r.bin, args...)
	cmd.Env = append(os.Environ(), extraEnv...)
	cmd.Stdin = orDefault[io.Reader](s.In, os.Stdin)
	cmd.Stdout = orDefault[io.Writer](s.Out, os.Stdout)
	cmd.Stderr = orDefault[io.Writer](s.Err, os.Stderr)
	cmd.Cancel = func() error { return cmd.Process.Signal(os.Interrupt) }
	if err := cmd.Run(); err != nil {
		var ee *exec.ExitError
		if errors.As(err, &ee) {
			return ee.ExitCode(), nil
		}
		return -1, err
	}
	return 0, nil
}

func orDefault[T comparable](v, def T) T {
	var zero T
	if v == zero {
		return def
	}
	return v
}
```

Note: `os.Stdin` is `*os.File` which satisfies `io.Reader`; `orDefault` compares the interface value against nil.

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/cli/ -v`
Expected: PASS (4 tests).

- [ ] **Step 5: Commit**

```bash
git add internal/cli/
git commit -m "feat(cli): terminal-inherit runner returning exit code"
```

---

## Phase 4 — dwarfgen + generated api types

### Task 8: dwarfgen tool

**Files:**
- Create: `internal/tools/dwarfgen/main.go`

dwarfgen is a developer tool run by hand against an `sbx` binary; its output (`internal/api/types_gen.go`) is committed. It walks DWARF for a curated root list and emits Go structs, deriving json tags as `snake_case(FieldName)` with `omitempty` for pointer fields (DWARF has no tags — verified).

- [ ] **Step 1: Write the tool**

```go
// Command dwarfgen extracts sandboxapi request/response structs from an unstripped
// sbx binary's DWARF and emits Go definitions into internal/api/types_gen.go.
// Usage: go run ./internal/tools/dwarfgen -bin /usr/bin/sbx -out internal/api/types_gen.go
//
// DWARF carries field names/types/optionality but NOT struct tags, so json tags are
// derived as snake_case(FieldName) (+omitempty for pointer fields) and validated by
// internal/api/types_validate_test.go against live daemon JSON.
package main

import (
	"bytes"
	"debug/dwarf"
	"debug/elf"
	"flag"
	"fmt"
	"go/format"
	"os"
	"sort"
	"strings"
)

// roots is the curated set of sandboxapi types to extract (closure followed automatically).
var roots = []string{
	"github.com/docker/sandboxes/sandboxapi.SandboxInfo",
	"github.com/docker/sandboxes/sandboxapi.WorkspaceMount",
	"github.com/docker/sandboxes/sandboxapi.PublishedPort",
	"github.com/docker/sandboxes/sandboxapi.SandboxWorktree",
	"github.com/docker/sandboxes/sandboxapi.HealthResponse",
	"github.com/docker/sandboxes/sandboxapi.DaemonInfo",
}

func main() {
	bin := flag.String("bin", "/usr/bin/sbx", "path to unstripped sbx binary")
	out := flag.String("out", "internal/api/types_gen.go", "output file")
	flag.Parse()

	f, err := elf.Open(*bin)
	must(err)
	defer f.Close()
	d, err := f.DWARF()
	must(err)

	emitted := map[string]bool{}
	queue := append([]string{}, roots...)
	var b bytes.Buffer
	for len(queue) > 0 {
		name := queue[0]
		queue = queue[1:]
		if emitted[name] {
			continue
		}
		st := findStruct(d, name)
		if st == nil {
			fmt.Fprintf(os.Stderr, "warning: type not found in DWARF: %s\n", name)
			emitted[name] = true
			continue
		}
		emitStruct(&b, name, st, &queue)
		emitted[name] = true
	}

	src := "// Code generated by dwarfgen from sbx " + *bin + "; DO NOT EDIT.\n\n" +
		"package api\n\nimport \"time\"\n\nvar _ = time.Time{}\n\n" + b.String()
	formatted, err := format.Source([]byte(src))
	if err != nil {
		formatted = []byte(src) // emit unformatted on error so it can be inspected
	}
	must(os.WriteFile(*out, formatted, 0o644))
	fmt.Printf("wrote %s (%d types)\n", *out, len(emitted))
}

func findStruct(d *dwarf.Data, fqName string) *dwarf.StructType {
	r := d.Reader()
	for {
		e, err := r.Next()
		if err != nil || e == nil {
			return nil
		}
		if e.Tag != dwarf.TagStructType {
			continue
		}
		if n, _ := e.Val(dwarf.AttrName).(string); n == fqName {
			typ, err := d.Type(e.Offset)
			if err != nil {
				return nil
			}
			if st, ok := typ.(*dwarf.StructType); ok {
				return st
			}
		}
	}
}

func emitStruct(b *bytes.Buffer, fqName string, st *dwarf.StructType, queue *[]string) {
	fmt.Fprintf(b, "// %s\ntype %s struct {\n", short(fqName), goTypeName(short(fqName)))
	type fld struct{ name, gotype, tag string }
	var fields []fld
	for _, m := range st.Field {
		if m.Name == "" || !exported(m.Name) {
			continue
		}
		gt, dep := mapType(m.Type.String())
		if dep != "" {
			*queue = append(*queue, dep)
		}
		jsonName := snake(m.Name)
		tag := jsonName
		if strings.HasPrefix(gt, "*") || strings.HasPrefix(gt, "[]") {
			tag += ",omitempty"
		}
		fields = append(fields, fld{m.Name, gt, tag})
	}
	sort.SliceStable(fields, func(i, j int) bool { return fields[i].name < fields[j].name })
	for _, fl := range fields {
		fmt.Fprintf(b, "\t%s %s `json:\"%s\"`\n", fl.name, fl.gotype, fl.tag)
	}
	fmt.Fprint(b, "}\n\n")
}

// mapType converts a DWARF type string to a Go type and, if it references another
// sandboxapi type, returns that type's fully-qualified name to enqueue.
func mapType(s string) (gotype, dep string) {
	s = strings.TrimPrefix(s, "struct ")
	switch {
	case s == "string" || s == "bool" || s == "int" || s == "int64" || s == "uint32" || s == "float64":
		return s, ""
	case s == "*time.Time" || s == "time.Time":
		return s, ""
	case strings.HasPrefix(s, "*"):
		inner, dep := mapType(strings.TrimPrefix(s, "*"))
		return "*" + inner, dep
	case strings.HasPrefix(s, "[]"):
		inner, dep := mapType(strings.TrimPrefix(s, "[]"))
		return "[]" + inner, dep
	case strings.Contains(s, "sandboxapi."):
		return goTypeName(short(s)), s
	default:
		return "string", "" // fallback: enum/defined string types serialize as strings
	}
}

func short(fq string) string {
	if i := strings.LastIndex(fq, "."); i >= 0 {
		return fq[i+1:]
	}
	return fq
}
func goTypeName(s string) string { return s }
func exported(name string) bool  { return name != "" && name[0] >= 'A' && name[0] <= 'Z' }

func snake(s string) string {
	var b strings.Builder
	for i, r := range s {
		if r >= 'A' && r <= 'Z' {
			if i > 0 {
				b.WriteByte('_')
			}
			b.WriteRune(r + 32)
		} else {
			b.WriteRune(r)
		}
	}
	return b.String()
}

func must(err error) {
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
```

- [ ] **Step 2: Run dwarfgen against the installed binary**

Run: `mkdir -p internal/api && go run ./internal/tools/dwarfgen -bin /usr/bin/sbx -out internal/api/types_gen.go`
Expected: `wrote internal/api/types_gen.go (N types)` and the file contains `type SandboxInfo struct` with fields like `Name string \`json:"name"\``, `Agent *string \`json:"agent,omitempty"\``, `CreatedAt *time.Time \`json:"created_at,omitempty"\``.

- [ ] **Step 3: Verify it compiles**

Run: `go build ./internal/api/`
Expected: exit 0. If a field's type fell back incorrectly, hand-fix it in `types_gen.go` (it's curated generated code).

- [ ] **Step 4: Commit**

```bash
git add internal/tools/dwarfgen/ internal/api/types_gen.go
git commit -m "feat(api): dwarfgen tool + generated sandboxapi structs"
```

### Task 9: Live-JSON round-trip validation test

**Files:**
- Create: `internal/api/types_validate_test.go`

This test runs only when a live daemon is present (build-tagged), and fails on any field the daemon emits that the generated structs drop — the drift detector.

- [ ] **Step 1: Write the test**

```go
//go:build integration

package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net"
	"net/http"
	"os"
	"testing"

	"github.com/stretchr/testify/require"
)

func liveGet(t *testing.T, path string) []byte {
	t.Helper()
	sock := os.Getenv("DOCKER_SANDBOXES_API")
	if sock == "" {
		home, _ := os.UserHomeDir()
		sock = home + "/.local/state/sandboxes/sandboxes/sandboxd/sandboxd.sock"
	}
	hc := &http.Client{Transport: &http.Transport{
		DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
			return (&net.Dialer{}).DialContext(ctx, "unix", sock)
		},
	}}
	resp, err := hc.Get("http://d" + path)
	require.NoError(t, err)
	defer resp.Body.Close()
	var buf bytes.Buffer
	buf.ReadFrom(resp.Body)
	return buf.Bytes()
}

// requireNoUnknownFields round-trips raw JSON through the typed struct and back,
// failing if any key present in raw is missing from the typed re-encoding.
func requireNoUnknownFields(t *testing.T, raw []byte, typed any) {
	require.NoError(t, json.Unmarshal(raw, typed))
	reencoded, err := json.Marshal(typed)
	require.NoError(t, err)
	var a, b map[string]any
	json.Unmarshal(raw, &a)
	json.Unmarshal(reencoded, &b)
	for k := range a {
		_, ok := b[k]
		require.Truef(t, ok, "field %q present in daemon JSON but missing from struct", k)
	}
}

func TestSandboxInfo_NoDrift(t *testing.T) {
	raw := liveGet(t, "/sandbox")
	var arr []SandboxInfo
	requireNoUnknownFields(t, raw, &arr) // arr-level; per-element checked below
	var rawArr []json.RawMessage
	require.NoError(t, json.Unmarshal(raw, &rawArr))
	for _, el := range rawArr {
		var si SandboxInfo
		requireNoUnknownFields(t, el, &si)
	}
}
```

- [ ] **Step 2: Run it against the live daemon**

Run: `go test -tags integration ./internal/api/ -run TestSandboxInfo_NoDrift -v`
Expected: PASS if no sandboxes (empty array trivially passes). To exercise it fully, create one first (`sbx create shell /tmp --name drift`), run, then `sbx rm --force drift`. Any missing field → fix `types_gen.go`.

- [ ] **Step 3: Commit**

```bash
git add internal/api/types_validate_test.go
git commit -m "test(api): live-JSON drift detector for generated structs"
```

---

## Phase 5 — client package

### Task 10: Error types and sentinels

**Files:**
- Create: `client/errors.go`
- Test: `client/errors_test.go`

- [ ] **Step 1: Write the failing test**

```go
package client

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/squall-chua/sbx-go-sdk/internal/transport"
)

func TestMapHTTPError_NotFound(t *testing.T) {
	err := mapHTTPError("inspect", &transport.HTTPStatusError{Status: 404, Body: []byte(`{"message":"sandbox not found"}`)})
	require.ErrorIs(t, err, ErrSandboxNotFound)
	var ae *APIError
	require.ErrorAs(t, err, &ae)
	require.Equal(t, 404, ae.Status)
	require.Equal(t, "sandbox not found", ae.Message)
}

func TestMapHTTPError_PassthroughNon404(t *testing.T) {
	err := mapHTTPError("x", &transport.HTTPStatusError{Status: 500, Body: []byte(`{"message":"boom"}`)})
	require.False(t, errors.Is(err, ErrSandboxNotFound))
	var ae *APIError
	require.ErrorAs(t, err, &ae)
	require.Equal(t, 500, ae.Status)
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./client/ -run TestMapHTTPError -v`
Expected: FAIL — undefined symbols.

- [ ] **Step 3: Write minimal implementation**

```go
package client

import (
	"encoding/json"
	"errors"
	"fmt"

	"github.com/squall-chua/sbx-go-sdk/internal/cli"
	"github.com/squall-chua/sbx-go-sdk/internal/transport"
)

// Sentinels callers branch on.
var (
	ErrSandboxNotFound     = errors.New("sandbox not found")
	ErrSandboxExists       = errors.New("sandbox already exists")
	ErrSandboxNotRunning   = errors.New("sandbox not running")
	ErrExecNotFound        = errors.New("exec not found")
	ErrIncompatibleVersion = errors.New("incompatible sbx/daemon version")
	ErrDaemonNotRunning    = errors.New("sandboxd not running")
	ErrBinaryNotFound      = cli.ErrBinaryNotFound
)

// APIError is a structured non-2xx response from the daemon REST API.
type APIError struct {
	Op      string
	Status  int
	Message string
}

func (e *APIError) Error() string {
	return fmt.Sprintf("sbx %s: HTTP %d: %s", e.Op, e.Status, e.Message)
}

// CLIError re-exports a shell-out failure for callers (errors.As friendly).
type CLIError = cli.Error

// parseMessage extracts {"message":...} from a daemon error body, falling back to
// the raw body.
func parseMessage(body []byte) string {
	var m struct {
		Message string `json:"message"`
	}
	if json.Unmarshal(body, &m) == nil && m.Message != "" {
		return m.Message
	}
	return string(body)
}

// mapHTTPError converts a transport.HTTPStatusError into an *APIError, joined with
// a sentinel when the status maps to one. Non-transport errors pass through.
func mapHTTPError(op string, err error) error {
	var se *transport.HTTPStatusError
	if !errors.As(err, &se) {
		return err
	}
	ae := &APIError{Op: op, Status: se.Status, Message: parseMessage(se.Body)}
	switch se.Status {
	case 404:
		return errors.Join(ErrSandboxNotFound, ae)
	case 409:
		return errors.Join(ErrSandboxExists, ae)
	}
	return ae
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./client/ -run TestMapHTTPError -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add client/errors.go client/errors_test.go
git commit -m "feat(client): typed errors (APIError, CLIError) + sentinels"
```

### Task 11: Client + options + construction

**Files:**
- Create: `client/options.go`
- Create: `client/client.go`
- Test: `client/client_test.go`

- [ ] **Step 1: Write the failing test**

```go
package client

import (
	"context"
	"net"
	"net/http"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func stub(t *testing.T, h http.Handler) string {
	t.Helper()
	sock := filepath.Join(t.TempDir(), "d.sock")
	l, err := net.Listen("unix", sock)
	require.NoError(t, err)
	srv := &http.Server{Handler: h}
	go srv.Serve(l)
	t.Cleanup(func() { srv.Close() })
	return sock
}

func TestNew_WithSocketPath(t *testing.T) {
	sock := stub(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"status":"healthy","version":"v0.32.0"}`))
	}))
	c, err := New(context.Background(), WithSocketPath(sock))
	require.NoError(t, err)
	require.Equal(t, sock, c.SocketPath())
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./client/ -run TestNew_WithSocketPath -v`
Expected: FAIL — undefined `New`/`WithSocketPath`.

- [ ] **Step 3: Implement options.go**

```go
package client

import "time"

type config struct {
	socketPath   string
	binaryPath   string
	autoStart    bool
	strictVer    bool
	httpTimeout  time.Duration
}

// Option configures a Client.
type Option func(*config)

// WithSocketPath overrides the daemon socket path (highest precedence).
func WithSocketPath(p string) Option { return func(c *config) { c.socketPath = p } }

// WithBinaryPath overrides the sbx binary path (default: looked up on PATH).
func WithBinaryPath(p string) Option { return func(c *config) { c.binaryPath = p } }

// WithAutoStart makes New ensure the daemon is running before returning.
func WithAutoStart() Option { return func(c *config) { c.autoStart = true } }

// WithStrictVersion makes the client hard-fail on an incompatible daemon version.
func WithStrictVersion() Option { return func(c *config) { c.strictVer = true } }

// WithHTTPTimeout sets the per-request REST timeout (0 = none).
func WithHTTPTimeout(d time.Duration) Option { return func(c *config) { c.httpTimeout = d } }
```

- [ ] **Step 4: Implement client.go**

```go
package client

import (
	"context"

	"github.com/squall-chua/sbx-go-sdk/internal/cli"
	"github.com/squall-chua/sbx-go-sdk/internal/transport"
)

// Client talks to a local sandboxd daemon (REST over a unix socket) and drives
// the sbx binary for orchestration-heavy operations.
type Client struct {
	cfg    config
	tr     *transport.Transport
	runner *cli.Runner // lazily created on first shell-out use
}

// New constructs a Client. By default it resolves the socket path (explicit >
// $DOCKER_SANDBOXES_API > XDG default) and does not start the daemon.
func New(ctx context.Context, opts ...Option) (*Client, error) {
	var cfg config
	for _, o := range opts {
		o(&cfg)
	}
	sock, err := transport.ResolveSocketPath(cfg.socketPath)
	if err != nil {
		return nil, err
	}
	tr := transport.New(sock)
	if cfg.httpTimeout > 0 {
		tr.SetTimeout(cfg.httpTimeout)
	}
	c := &Client{cfg: cfg, tr: tr}
	if cfg.autoStart {
		if err := c.EnsureRunning(ctx); err != nil {
			return nil, err
		}
	}
	if cfg.strictVer {
		res, err := c.CheckVersion(ctx)
		if err != nil {
			return nil, err
		}
		if res != "compatible" {
			return nil, ErrIncompatibleVersion
		}
	}
	return c, nil
}

// SocketPath returns the resolved daemon socket path.
func (c *Client) SocketPath() string { return c.tr.Socket() }

// transport exposes the low-level transport to sibling packages within the module.
func (c *Client) Transport() *transport.Transport { return c.tr }

// runnerOrErr lazily resolves the sbx binary runner.
func (c *Client) runnerOrErr() (*cli.Runner, error) {
	if c.runner != nil {
		return c.runner, nil
	}
	r, err := cli.NewRunner(c.cfg.binaryPath)
	if err != nil {
		return nil, err
	}
	c.runner = r
	return r, nil
}

// DefaultClient is a lazily-initialized client over the default socket.
// It is created on first use by callers that want zero-config access.
var DefaultClient = mustDefault()

func mustDefault() *Client {
	sock, _ := transport.ResolveSocketPath("")
	return &Client{tr: transport.New(sock)}
}
```

- [ ] **Step 5: Run test to verify it passes**

Run: `go test ./client/ -run TestNew_WithSocketPath -v`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add client/options.go client/client.go client/client_test.go
git commit -m "feat(client): Client construction with functional options"
```

### Task 12: Daemon health & version

**Files:**
- Create: `client/daemon.go`
- Test: `client/daemon_test.go`

- [ ] **Step 1: Write the failing test**

```go
package client

import (
	"context"
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestHealth(t *testing.T) {
	sock := stub(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/health", r.URL.Path)
		w.Write([]byte(`{"release":false,"status":"healthy","version":"v0.32.0 abc"}`))
	}))
	c, _ := New(context.Background(), WithSocketPath(sock))
	h, err := c.Health(context.Background())
	require.NoError(t, err)
	require.Equal(t, "healthy", h.Status)
	require.Equal(t, "v0.32.0 abc", h.Version)
}

func TestCheckVersion(t *testing.T) {
	sock := stub(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, http.MethodPost, r.Method)
		require.Equal(t, "/version", r.URL.Path)
		w.Write([]byte(`{"result":"compatible"}`))
	}))
	c, _ := New(context.Background(), WithSocketPath(sock))
	res, err := c.CheckVersion(context.Background())
	require.NoError(t, err)
	require.Equal(t, "compatible", res)
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./client/ -run 'TestHealth|TestCheckVersion' -v`
Expected: FAIL — undefined `Health`/`CheckVersion`.

- [ ] **Step 3: Write minimal implementation**

```go
package client

import (
	"context"
	"net/http"
)

// Health is the /health response.
type Health struct {
	Release bool   `json:"release"`
	Status  string `json:"status"`
	Version string `json:"version"`
}

// Health returns daemon liveness.
func (c *Client) Health(ctx context.Context) (*Health, error) {
	var h Health
	if err := c.tr.DoJSON(ctx, http.MethodGet, "/health", nil, &h); err != nil {
		return nil, mapHTTPError("health", err)
	}
	return &h, nil
}

// versionRequest is the body the CLI sends to /version (its own version string).
type versionRequest struct {
	Version string `json:"version"`
}

type versionResponse struct {
	Result string `json:"result"`
}

// ClientVersion is the sbx/daemon version this SDK was built/tested against.
const ClientVersion = "v0.32.0"

// CheckVersion asks the daemon whether this client is compatible.
// Returns "compatible", "incompatible", or "unknown".
func (c *Client) CheckVersion(ctx context.Context) (string, error) {
	var resp versionResponse
	err := c.tr.DoJSON(ctx, http.MethodPost, "/version", versionRequest{Version: ClientVersion}, &resp)
	if err != nil {
		return "", mapHTTPError("version", err)
	}
	return resp.Result, nil
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./client/ -run 'TestHealth|TestCheckVersion' -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add client/daemon.go client/daemon_test.go
git commit -m "feat(client): daemon Health + CheckVersion"
```

### Task 13: Daemon info, log levels, shutdown, reset (REST)

**Files:**
- Modify: `client/daemon.go`
- Test: `client/daemon_test.go`

- [ ] **Step 1: Write the failing test**

```go
func TestDaemonInfoAndLogLevels(t *testing.T) {
	sock := stub(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/daemon/info":
			w.Write([]byte(`{"api_socket":"/a.sock","docker_socket":"/d.sock"}`))
		case "/daemon/loglevel":
			w.Write([]byte(`{"general":"info","proxy":"info"}`))
		default:
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
	}))
	c, _ := New(context.Background(), WithSocketPath(sock))
	info, err := c.Info(context.Background())
	require.NoError(t, err)
	require.Equal(t, "/d.sock", info.DockerSocket)
	ll, err := c.LogLevels(context.Background())
	require.NoError(t, err)
	require.Equal(t, "info", ll.Proxy)
}

func TestStopAndReset(t *testing.T) {
	var paths []string
	sock := stub(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.Method+" "+r.URL.Path)
		w.WriteHeader(200)
	}))
	c, _ := New(context.Background(), WithSocketPath(sock))
	require.NoError(t, c.StopDaemon(context.Background()))
	require.NoError(t, c.Reset(context.Background()))
	require.Equal(t, []string{"POST /daemon/shutdown", "POST /daemon/reset"}, paths)
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./client/ -run 'TestDaemonInfoAndLogLevels|TestStopAndReset' -v`
Expected: FAIL.

- [ ] **Step 3: Add methods to daemon.go**

```go
// DaemonInfo is the /daemon/info response.
type DaemonInfo struct {
	APISocket    string `json:"api_socket"`
	DockerSocket string `json:"docker_socket"`
}

// Info returns the daemon's socket paths.
func (c *Client) Info(ctx context.Context) (*DaemonInfo, error) {
	var d DaemonInfo
	if err := c.tr.DoJSON(ctx, http.MethodGet, "/daemon/info", nil, &d); err != nil {
		return nil, mapHTTPError("daemon-info", err)
	}
	return &d, nil
}

// LogLevels is the /daemon/loglevel response.
type LogLevels struct {
	General string `json:"general"`
	Proxy   string `json:"proxy"`
}

// LogLevels returns the daemon's per-category log levels.
func (c *Client) LogLevels(ctx context.Context) (*LogLevels, error) {
	var l LogLevels
	if err := c.tr.DoJSON(ctx, http.MethodGet, "/daemon/loglevel", nil, &l); err != nil {
		return nil, mapHTTPError("loglevel", err)
	}
	return &l, nil
}

// SetLogLevel sets a category's level. category: "proxy", "general", or "all".
func (c *Client) SetLogLevel(ctx context.Context, category, level string) error {
	body := map[string]string{"target": category, "level": level}
	if err := c.tr.DoJSON(ctx, http.MethodPost, "/daemon/loglevel/set", body, nil); err != nil {
		return mapHTTPError("set-loglevel", err)
	}
	return nil
}

// StopDaemon shuts the daemon down (REST).
func (c *Client) StopDaemon(ctx context.Context) error {
	if err := c.tr.DoJSON(ctx, http.MethodPost, "/daemon/shutdown", nil, nil); err != nil {
		return mapHTTPError("shutdown", err)
	}
	return nil
}

// Reset resets all sandboxes and daemon state (REST).
func (c *Client) Reset(ctx context.Context) error {
	if err := c.tr.DoJSON(ctx, http.MethodPost, "/daemon/reset", nil, nil); err != nil {
		return mapHTTPError("reset", err)
	}
	return nil
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./client/ -run 'TestDaemonInfoAndLogLevels|TestStopAndReset' -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add client/daemon.go client/daemon_test.go
git commit -m "feat(client): daemon info, log levels, shutdown, reset (REST)"
```

### Task 14: Daemon start / EnsureRunning (shell-out)

**Files:**
- Modify: `client/daemon.go`
- Test: `client/daemon_test.go`

- [ ] **Step 1: Write the failing test**

```go
func TestEnsureRunning_AlreadyHealthy(t *testing.T) {
	sock := stub(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"status":"healthy"}`))
	}))
	// binary path points at a fake that would FAIL if called — proves we don't start.
	bin := filepath.Join(t.TempDir(), "sbx")
	os.WriteFile(bin, []byte("#!/bin/sh\nexit 1\n"), 0o755)
	c, _ := New(context.Background(), WithSocketPath(sock), WithBinaryPath(bin))
	require.NoError(t, c.EnsureRunning(context.Background()))
}
```

Add imports `os`, `path/filepath` to the test file if missing.

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./client/ -run TestEnsureRunning -v`
Expected: FAIL — `c.EnsureRunning undefined`.

- [ ] **Step 3: Add lifecycle methods to daemon.go**

```go
import (
	"time"          // add to daemon.go imports
)

// StartOptions configures daemon start.
type StartOptions struct {
	Policy string // "allow-all" | "balanced" | "deny-all"; empty = daemon default
}

// EnsureRunning returns nil if the daemon is healthy; otherwise it starts it
// (shell-out) and waits up to ~30s for the socket to become healthy.
func (c *Client) EnsureRunning(ctx context.Context) error {
	if _, err := c.Health(ctx); err == nil {
		return nil
	}
	if err := c.StartDaemon(ctx, StartOptions{}); err != nil {
		return err
	}
	return c.waitHealthy(ctx, 30*time.Second)
}

// StartDaemon starts sandboxd via `sbx daemon start --detach` (shell-out).
func (c *Client) StartDaemon(ctx context.Context, opts StartOptions) error {
	r, err := c.runnerOrErr()
	if err != nil {
		return err
	}
	args := []string{"daemon", "start", "--detach"}
	if opts.Policy != "" {
		args = append(args, "--policy", opts.Policy)
	}
	if _, err := r.Capture(ctx, nil, args...); err != nil {
		return err
	}
	return nil
}

func (c *Client) waitHealthy(ctx context.Context, d time.Duration) error {
	deadline := time.Now().Add(d)
	for {
		if _, err := c.Health(ctx); err == nil {
			return nil
		}
		if time.Now().After(deadline) {
			return ErrDaemonNotRunning
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(250 * time.Millisecond):
		}
	}
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./client/ -run TestEnsureRunning -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add client/daemon.go client/daemon_test.go
git commit -m "feat(client): daemon start + EnsureRunning (shell-out)"
```

---

## Phase 6 — sandbox package

### Task 15: Sandbox handle + List/Get (REST)

**Files:**
- Create: `sandbox/sandbox.go`
- Create: `sandbox/list.go`
- Test: `sandbox/list_test.go`

- [ ] **Step 1: Write the failing test**

```go
package sandbox

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

func TestList(t *testing.T) {
	c := stubClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/sandbox", r.URL.Path)
		w.Write([]byte(`[{"name":"a","agent":"claude","status":"SANDBOX_STATUS_RUNNING","workspace":"/w"}]`))
	}))
	sbs, err := List(context.Background(), c)
	require.NoError(t, err)
	require.Len(t, sbs, 1)
	require.Equal(t, "a", sbs[0].Name())
	require.Equal(t, "claude", sbs[0].Agent())
	require.True(t, sbs[0].IsRunning())
}

func TestGet_NotFound(t *testing.T) {
	c := stubClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(404)
		w.Write([]byte(`{"message":"not found"}`))
	}))
	_, err := Get(context.Background(), c, "nope")
	require.ErrorIs(t, err, client.ErrSandboxNotFound)
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./sandbox/ -run 'TestList|TestGet' -v`
Expected: FAIL — undefined symbols.

- [ ] **Step 3: Implement sandbox.go**

```go
// Package sandbox manages sbx sandboxes: provisioning (shell-out) and lifecycle
// (REST), following the docker/go-sdk functional-options idiom.
package sandbox

import (
	"context"
	"net/http"

	"github.com/squall-chua/sbx-go-sdk/client"
	"github.com/squall-chua/sbx-go-sdk/internal/api"
)

// StatusRunning is the daemon's running-status string.
const StatusRunning = "SANDBOX_STATUS_RUNNING"

// Sandbox is a handle to a single sandbox, addressed by name.
type Sandbox struct {
	cli  *client.Client
	info api.SandboxInfo
}

func newSandbox(c *client.Client, info api.SandboxInfo) *Sandbox {
	return &Sandbox{cli: c, info: info}
}

// Name returns the sandbox name (the primary identifier).
func (s *Sandbox) Name() string { return s.info.Name }

// Agent returns the agent the sandbox was created for ("" if unset).
func (s *Sandbox) Agent() string {
	if s.info.Agent == nil {
		return ""
	}
	return *s.info.Agent
}

// State returns the last-known status string.
func (s *Sandbox) State() string { return statusString(s.info) }

// IsRunning reports whether the last-known status is running.
func (s *Sandbox) IsRunning() bool { return statusString(s.info) == StatusRunning }

// Inspect refreshes and returns the sandbox info.
func (s *Sandbox) Inspect(ctx context.Context) (api.SandboxInfo, error) {
	var info api.SandboxInfo
	err := s.cli.Transport().DoJSON(ctx, http.MethodGet, "/sandbox/"+s.info.Name, nil, &info)
	if err != nil {
		return api.SandboxInfo{}, client.MapError("inspect", err)
	}
	s.info = info
	return info, nil
}
```

- [ ] **Step 4: Implement list.go**

```go
package sandbox

import (
	"context"
	"net/http"

	"github.com/squall-chua/sbx-go-sdk/client"
	"github.com/squall-chua/sbx-go-sdk/internal/api"
)

// statusString reads the status field regardless of whether the generated type
// models it as a string or a defined string type.
func statusString(info api.SandboxInfo) string { return string(info.Status) }

// List returns all sandboxes.
func List(ctx context.Context, c *client.Client) ([]*Sandbox, error) {
	var infos []api.SandboxInfo
	if err := c.Transport().DoJSON(ctx, http.MethodGet, "/sandbox", nil, &infos); err != nil {
		return nil, client.MapError("list", err)
	}
	out := make([]*Sandbox, len(infos))
	for i, in := range infos {
		out[i] = newSandbox(c, in)
	}
	return out, nil
}

// Get returns a single sandbox by name.
func Get(ctx context.Context, c *client.Client, name string) (*Sandbox, error) {
	var info api.SandboxInfo
	if err := c.Transport().DoJSON(ctx, http.MethodGet, "/sandbox/"+name, nil, &info); err != nil {
		return nil, client.MapError("get", err)
	}
	return newSandbox(c, info), nil
}
```

- [ ] **Step 5: Export MapError from client**

The sandbox/exec packages need the error mapper. Add to `client/errors.go`:

```go
// MapError converts a transport error to a typed APIError/sentinel. Exported for
// sibling packages (sandbox, exec).
func MapError(op string, err error) error { return mapHTTPError(op, err) }
```

If `api.SandboxInfo.Status` is a defined string type (not plain `string`), `string(info.Status)` works; if dwarfgen emitted it as `string`, change `statusString` to `return info.Status`. Adjust to match the generated type, then re-run.

- [ ] **Step 6: Run test to verify it passes**

Run: `go test ./sandbox/ -run 'TestList|TestGet' -v`
Expected: PASS.

- [ ] **Step 7: Commit**

```bash
git add sandbox/sandbox.go sandbox/list.go sandbox/list_test.go client/errors.go
git commit -m "feat(sandbox): Sandbox handle + List/Get (REST)"
```

### Task 16: Definition + options

**Files:**
- Create: `sandbox/definition.go`
- Create: `sandbox/options.go`
- Test: `sandbox/options_test.go`

- [ ] **Step 1: Write the failing test**

```go
package sandbox

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDefinition_ToCreateArgs(t *testing.T) {
	d := newDefinition(
		WithAgent("claude"),
		WithWorkspace("/abs/ws"),
		WithWorkspace("/abs/docs:ro"),
		WithName("proj"),
		WithCPUs(4),
		WithMemory("8g"),
		WithProfile("balanced"),
		WithClone(),
	)
	args, err := d.toCreateArgs()
	require.NoError(t, err)
	require.Equal(t, []string{
		"create", "claude", "/abs/ws", "/abs/docs:ro",
		"--name", "proj", "--cpus", "4", "--memory", "8g",
		"--profile", "balanced", "--clone",
	}, args)
}

func TestDefinition_RequiresAgentAndWorkspace(t *testing.T) {
	_, err := newDefinition(WithAgent("claude")).toCreateArgs()
	require.Error(t, err)
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./sandbox/ -run TestDefinition -v`
Expected: FAIL.

- [ ] **Step 3: Implement definition.go**

```go
package sandbox

import (
	"errors"
	"strconv"
)

// Definition is the create spec built from options.
type Definition struct {
	agent      string
	workspaces []string // each may carry a ":ro" suffix
	name       string
	cpus       int
	memory     string
	profile    string
	template   string
	clone      bool
}

func newDefinition(opts ...Option) *Definition {
	d := &Definition{}
	for _, o := range opts {
		o(d)
	}
	return d
}

// toCreateArgs builds the `sbx create ...` argument vector. Workspaces must already
// be absolute (resolved by the caller in lifecycle.go).
func (d *Definition) toCreateArgs() ([]string, error) {
	if d.agent == "" {
		return nil, errors.New("sandbox: agent is required (WithAgent)")
	}
	if len(d.workspaces) == 0 {
		return nil, errors.New("sandbox: at least one workspace is required (WithWorkspace)")
	}
	args := []string{"create", d.agent}
	args = append(args, d.workspaces...)
	if d.name != "" {
		args = append(args, "--name", d.name)
	}
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
	return args, nil
}
```

- [ ] **Step 4: Implement options.go**

```go
package sandbox

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
```

- [ ] **Step 5: Run test to verify it passes**

Run: `go test ./sandbox/ -run TestDefinition -v`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add sandbox/definition.go sandbox/options.go sandbox/options_test.go
git commit -m "feat(sandbox): Definition + create options"
```

### Task 17: Name generation (deterministic identity)

**Files:**
- Create: `sandbox/name.go`
- Test: `sandbox/name_test.go`

- [ ] **Step 1: Write the failing test**

```go
package sandbox

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestGenerateName_Default(t *testing.T) {
	existing := map[string]bool{}
	got := generateName("claude", "/home/u/myproj", existing)
	require.Equal(t, "claude-myproj", got)
}

func TestGenerateName_Sanitizes(t *testing.T) {
	got := generateName("claude", "/home/u/My Proj!", map[string]bool{})
	require.Equal(t, "claude-My-Proj", got) // spaces->-, invalid chars dropped
}

func TestGenerateName_CollisionSuffix(t *testing.T) {
	existing := map[string]bool{"claude-myproj": true, "claude-myproj-2": true}
	got := generateName("claude", "/x/myproj", existing)
	require.Equal(t, "claude-myproj-3", got)
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./sandbox/ -run TestGenerateName -v`
Expected: FAIL.

- [ ] **Step 3: Write minimal implementation**

```go
package sandbox

import (
	"path/filepath"
	"strconv"
	"strings"
)

// allowed name charset: letters, digits, '.', '+', '-'.
func sanitize(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9',
			r == '.', r == '+', r == '-':
			b.WriteRune(r)
		case r == ' ' || r == '_' || r == '/':
			b.WriteByte('-')
		}
	}
	return strings.Trim(b.String(), "-")
}

// generateName derives "<agent>-<basename(workspace)>", sanitized and collision-
// resolved against existing names by appending "-N".
func generateName(agent, primaryWorkspace string, existing map[string]bool) string {
	base := sanitize(agent + "-" + filepath.Base(primaryWorkspace))
	if !existing[base] {
		return base
	}
	for i := 2; ; i++ {
		cand := base + "-" + strconv.Itoa(i)
		if !existing[cand] {
			return cand
		}
	}
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./sandbox/ -run TestGenerateName -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add sandbox/name.go sandbox/name_test.go
git commit -m "feat(sandbox): deterministic name generation + collision suffix"
```

### Task 18: Create (shell-out) + Start/Stop/Remove (REST)

**Files:**
- Create: `sandbox/lifecycle.go`
- Test: `sandbox/lifecycle_test.go`

`Create` resolves workspaces to absolute, computes the name (so the SDK owns identity — no output parsing), shells out, then `Get`s to hydrate. To make this testable without a real daemon, `Create` takes the client (whose runner uses the fake `sbx`) and the stub serves `GET /sandbox/{name}`.

- [ ] **Step 1: Write the failing test**

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

func clientWithFakeSbx(t *testing.T, h http.Handler, sbxBody string) *client.Client {
	t.Helper()
	sock := filepath.Join(t.TempDir(), "d.sock")
	l, err := net.Listen("unix", sock)
	require.NoError(t, err)
	srv := &http.Server{Handler: h}
	go srv.Serve(l)
	t.Cleanup(func() { srv.Close() })
	bin := filepath.Join(t.TempDir(), "sbx")
	require.NoError(t, os.WriteFile(bin, []byte("#!/bin/sh\n"+sbxBody), 0o755))
	c, err := client.New(context.Background(), client.WithSocketPath(sock), client.WithBinaryPath(bin))
	require.NoError(t, err)
	return c
}

func TestCreate_OwnsNameAndHydrates(t *testing.T) {
	var sawCreate bool
	c := clientWithFakeSbx(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/sandbox": // List for collision check
			w.Write([]byte(`[]`))
		case "/sandbox/claude-myws":
			w.Write([]byte(`{"name":"claude-myws","agent":"claude","status":"SANDBOX_STATUS_RUNNING"}`))
		default:
			t.Fatalf("unexpected %s", r.URL.Path)
		}
	}), `echo "args: $*"; case "$*" in *"--name claude-myws"*) exit 0;; esac; exit 0`)

	ws := filepath.Join(t.TempDir(), "myws")
	require.NoError(t, os.Mkdir(ws, 0o755))
	sb, err := Create(context.Background(), c, WithAgent("claude"), WithWorkspace(ws))
	require.NoError(t, err)
	require.Equal(t, "claude-myws", sb.Name())
	require.True(t, sb.IsRunning())
	_ = sawCreate
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./sandbox/ -run TestCreate -v`
Expected: FAIL — `Create undefined`.

- [ ] **Step 3: Write minimal implementation**

```go
package sandbox

import (
	"context"
	"net/http"
	"path/filepath"
	"strings"

	"github.com/squall-chua/sbx-go-sdk/client"
)

// Create provisions a sandbox (shell-out `sbx create`) and returns a hydrated
// handle. Workspaces are resolved to absolute; the SDK owns the name so it never
// parses create output.
func Create(ctx context.Context, c *client.Client, opts ...Option) (*Sandbox, error) {
	d := newDefinition(opts...)

	// Resolve workspaces to absolute, preserving any ":ro" suffix.
	for i, ws := range d.workspaces {
		path, ro, _ := strings.Cut(ws, ":")
		abs, err := filepath.Abs(path)
		if err != nil {
			return nil, err
		}
		if ro == "ro" {
			d.workspaces[i] = abs + ":ro"
		} else {
			d.workspaces[i] = abs
		}
	}

	// Determine the name (own identity).
	if d.name == "" {
		existing, err := listNames(ctx, c)
		if err != nil {
			return nil, err
		}
		primary, _, _ := strings.Cut(d.workspaces[0], ":")
		d.name = generateName(d.agent, primary, existing)
	} else {
		existing, err := listNames(ctx, c)
		if err != nil {
			return nil, err
		}
		if existing[d.name] {
			return nil, client.ErrSandboxExists
		}
	}

	args, err := d.toCreateArgs()
	if err != nil {
		return nil, err
	}
	r, err := c.Runner()
	if err != nil {
		return nil, err
	}
	if _, err := r.Capture(ctx, nil, args...); err != nil {
		return nil, err
	}
	return Get(ctx, c, d.name)
}

func listNames(ctx context.Context, c *client.Client) (map[string]bool, error) {
	sbs, err := List(ctx, c)
	if err != nil {
		return nil, err
	}
	m := make(map[string]bool, len(sbs))
	for _, s := range sbs {
		m[s.Name()] = true
	}
	return m, nil
}

// Start starts the sandbox VM (REST).
func (s *Sandbox) Start(ctx context.Context) error {
	return s.post(ctx, "/start")
}

// Stop stops the sandbox VM without removing it (REST).
func (s *Sandbox) Stop(ctx context.Context) error {
	return s.post(ctx, "/stop")
}

// Remove deletes the sandbox (REST DELETE; no confirmation prompt).
func (s *Sandbox) Remove(ctx context.Context) error {
	err := s.cli.Transport().DoJSON(ctx, http.MethodDelete, "/sandbox/"+s.info.Name, nil, nil)
	if err != nil {
		return client.MapError("remove", err)
	}
	return nil
}

func (s *Sandbox) post(ctx context.Context, suffix string) error {
	err := s.cli.Transport().DoJSON(ctx, http.MethodPost, "/sandbox/"+s.info.Name+suffix, nil, nil)
	if err != nil {
		return client.MapError("lifecycle", err)
	}
	return nil
}
```

- [ ] **Step 4: Export Runner from client**

Add to `client/client.go`:

```go
// Runner resolves and returns the sbx-binary runner (for shell-out ops in sibling packages).
func (c *Client) Runner() (*cli.Runner, error) { return c.runnerOrErr() }
```

Change `internal/cli` import in client.go to expose the type if not already imported.

- [ ] **Step 5: Run test to verify it passes**

Run: `go test ./sandbox/ -run 'TestCreate|TestList|TestGet|TestDefinition|TestGenerateName' -v`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add sandbox/lifecycle.go sandbox/lifecycle_test.go client/client.go
git commit -m "feat(sandbox): Create (shell-out) + Start/Stop/Remove (REST)"
```

---

## Phase 7 — exec package

The `exec` methods hang off `*sandbox.Sandbox`. To avoid an import cycle (`sandbox` ← `exec`), the `exec` package depends on `sandbox` and `client`, and defines methods on `*sandbox.Sandbox` is not possible from another package — so exec functions take the sandbox as the first argument: `exec.Run(ctx, sb, cmd, opts...)`. We expose ergonomic wrappers on `*sandbox.Sandbox` in the `sandbox` package that call into a small interface, OR keep exec as package-level functions. **Decision (Task 19):** exec is package-level functions taking `*sandbox.Sandbox`; document `sb.Exec` style is achieved via thin forwarders added in the sandbox package in Plan 2 if desired.

### Task 19: ProcessOptions + exec request body

**Files:**
- Create: `exec/options.go`
- Test: `exec/options_test.go`

- [ ] **Step 1: Write the failing test**

```go
package exec

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestProcessOptions_BuildBody(t *testing.T) {
	body := buildBody([]string{"echo", "hi"},
		WithEnv(map[string]string{"CI": "1"}),
		WithWorkdir("/work"),
		WithUser("dev"),
		WithTTY(),
	)
	b, _ := json.Marshal(body)
	var got map[string]any
	require.NoError(t, json.Unmarshal(b, &got))
	require.Equal(t, []any{"echo", "hi"}, got["cmd"])
	require.Equal(t, "/work", got["workdir"])
	require.Equal(t, "dev", got["user"])
	require.Equal(t, true, got["tty"])
	require.Equal(t, map[string]any{"CI": "1"}, got["env"])
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./exec/ -run TestProcessOptions -v`
Expected: FAIL.

- [ ] **Step 3: Write minimal implementation**

```go
// Package exec runs commands inside sandboxes: capture, interactive attach, and
// detached, over the daemon's hijacking /exec/attach endpoint.
package exec

// processConfig accumulates exec options.
type processConfig struct {
	env        map[string]string
	workdir    string
	user       string
	privileged bool
	tty        bool
	autoStart  bool
}

// ProcessOption configures an exec invocation.
type ProcessOption func(*processConfig)

// WithEnv sets environment variables (cumulative).
func WithEnv(env map[string]string) ProcessOption {
	return func(c *processConfig) {
		if c.env == nil {
			c.env = map[string]string{}
		}
		for k, v := range env {
			c.env[k] = v
		}
	}
}

// WithWorkdir sets the working directory.
func WithWorkdir(d string) ProcessOption { return func(c *processConfig) { c.workdir = d } }

// WithUser sets the user (login name).
func WithUser(u string) ProcessOption { return func(c *processConfig) { c.user = u } }

// WithPrivileged grants extended privileges.
func WithPrivileged() ProcessOption { return func(c *processConfig) { c.privileged = true } }

// WithTTY allocates a pseudo-TTY (raw stream, no stdcopy framing).
func WithTTY() ProcessOption { return func(c *processConfig) { c.tty = true } }

// WithAutoStart transparently starts a stopped sandbox before exec.
func WithAutoStart() ProcessOption { return func(c *processConfig) { c.autoStart = true } }

// execBody is the JSON sent to POST /sandbox/{name}/exec/attach.
type execBody struct {
	Cmd        []string          `json:"cmd"`
	Env        map[string]string `json:"env,omitempty"`
	Workdir    string            `json:"workdir,omitempty"`
	User       string            `json:"user,omitempty"`
	Privileged bool              `json:"privileged,omitempty"`
	TTY        bool              `json:"tty,omitempty"`
}

func buildBody(cmd []string, opts ...ProcessOption) execBody {
	var c processConfig
	for _, o := range opts {
		o(&c)
	}
	return execBody{
		Cmd: cmd, Env: c.env, Workdir: c.workdir,
		User: c.user, Privileged: c.privileged, TTY: c.tty,
	}
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./exec/ -run TestProcessOptions -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add exec/options.go exec/options_test.go
git commit -m "feat(exec): ProcessOptions + exec request body"
```

### Task 20: Exec capture (hijack + stdcopy)

**Files:**
- Create: `exec/exec.go`
- Test: `exec/exec_test.go`

The test stub must speak the 101-upgrade protocol on `/sandbox/{name}/exec/attach`, stream stdcopy frames, then answer `GET /sandbox/{name}/exec/{id}` with an exit code.

- [ ] **Step 1: Write the failing test**

```go
package exec

import (
	"bufio"
	"context"
	"encoding/binary"
	"io"
	"net"
	"net/http"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/squall-chua/sbx-go-sdk/client"
	"github.com/squall-chua/sbx-go-sdk/sandbox"
)

func frame(b byte, s string) []byte {
	h := make([]byte, 8)
	h[0] = b
	binary.BigEndian.PutUint32(h[4:], uint32(len(s)))
	return append(h, []byte(s)...)
}

// attachStub serves the exec protocol on a raw unix listener.
func attachStub(t *testing.T) (*client.Client, string) {
	t.Helper()
	sock := filepath.Join(t.TempDir(), "d.sock")
	l, err := net.Listen("unix", sock)
	require.NoError(t, err)
	go func() {
		for {
			conn, err := l.Accept()
			if err != nil {
				return
			}
			go serveConn(conn)
		}
	}()
	t.Cleanup(func() { l.Close() })
	c, err := client.New(context.Background(), client.WithSocketPath(sock))
	require.NoError(t, err)
	return c, sock
}

func serveConn(conn net.Conn) {
	defer conn.Close()
	br := bufio.NewReader(conn)
	req, err := http.ReadRequest(br)
	if err != nil {
		return
	}
	io.Copy(io.Discard, req.Body)
	switch {
	case req.URL.Path == "/sandbox/s1/exec/attach":
		conn.Write([]byte("HTTP/1.1 101 Switching Protocols\r\n" +
			"Content-Type: application/vnd.docker.raw-stream\r\n" +
			"Sandboxes-Exec-Id: e1\r\nConnection: Upgrade\r\nUpgrade: tcp\r\n\r\n"))
		conn.Write(frame(1, "hello\n"))
	case req.URL.Path == "/sandbox/s1/exec/e1":
		body := `{"exit_code":0,"running":false}`
		conn.Write([]byte("HTTP/1.1 200 OK\r\nContent-Type: application/json\r\n" +
			"Content-Length: " + itoa(len(body)) + "\r\n\r\n" + body))
	}
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b []byte
	for n > 0 {
		b = append([]byte{byte('0' + n%10)}, b...)
		n /= 10
	}
	return string(b)
}

func TestExec_CaptureAndExit(t *testing.T) {
	c, _ := attachStub(t)
	sb := sandbox.NewForTest(c, "s1") // test-only constructor (Task 20 step 3)
	code, r, err := Exec(context.Background(), sb, []string{"echo", "hello"})
	require.NoError(t, err)
	out, _ := io.ReadAll(r)
	require.Equal(t, "hello\n", string(out))
	require.Equal(t, 0, code)
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./exec/ -run TestExec_Capture -v`
Expected: FAIL — `Exec` and `sandbox.NewForTest` undefined.

- [ ] **Step 3: Add a test-only constructor + transport accessor to sandbox**

In `sandbox/sandbox.go`:

```go
// NewForTest builds a Sandbox handle around a client and name without a daemon
// round-trip. For tests only.
func NewForTest(c *client.Client, name string) *Sandbox {
	return &Sandbox{cli: c, info: api.SandboxInfo{Name: name}}
}

// Client returns the underlying client (used by the exec package).
func (s *Sandbox) Client() *client.Client { return s.cli }
```

- [ ] **Step 4: Implement exec.go**

```go
package exec

import (
	"context"
	"encoding/json"
	"io"
	"net/http"

	"github.com/squall-chua/sbx-go-sdk/client"
	"github.com/squall-chua/sbx-go-sdk/internal/stdcopy"
	"github.com/squall-chua/sbx-go-sdk/sandbox"
)

// State is the /sandbox/{name}/exec/{id} response.
type State struct {
	ExitCode int  `json:"exit_code"`
	Running  bool `json:"running"`
}

// Exec runs cmd to completion and returns (exitCode, combinedOutput, error). The
// returned reader is the raw stdcopy stream by default; use WithMultiplexed to
// split it. Requires a running sandbox unless WithAutoStart is set.
func Exec(ctx context.Context, sb *sandbox.Sandbox, cmd []string, opts ...ProcessOption) (int, io.Reader, error) {
	body := buildBody(cmd, opts...)
	body.TTY = false
	raw, err := json.Marshal(body)
	if err != nil {
		return 0, nil, err
	}
	tr := sb.Client().Transport()
	conn, hdr, err := tr.Hijack(ctx, "/sandbox/"+sb.Name()+"/exec/attach", raw)
	if err != nil {
		return 0, nil, client.MapError("exec", err)
	}
	defer conn.Close()
	execID := hdr.Get("Sandboxes-Exec-Id")

	// Demux into an in-memory buffer (capture semantics).
	pr, pw := io.Pipe()
	go func() {
		var sink discardCloser
		_, derr := stdcopy.Demux(pw, &sink, conn)
		pw.CloseWithError(derr)
	}()
	out, _ := io.ReadAll(pr)

	st, err := inspectExec(ctx, sb, execID)
	if err != nil {
		return 0, byteReader(out), err
	}
	return st.ExitCode, byteReader(out), nil
}

func inspectExec(ctx context.Context, sb *sandbox.Sandbox, execID string) (State, error) {
	var st State
	err := sb.Client().Transport().DoJSON(ctx, http.MethodGet, "/sandbox/"+sb.Name()+"/exec/"+execID, nil, &st)
	if err != nil {
		return State{}, client.MapError("inspect-exec", err)
	}
	return st, nil
}

type discardCloser struct{}

func (discardCloser) Write(p []byte) (int, error) { return len(p), nil }

func byteReader(b []byte) io.Reader { return &bytesReader{b: b} }

type bytesReader struct {
	b []byte
	i int
}

func (r *bytesReader) Read(p []byte) (int, error) {
	if r.i >= len(r.b) {
		return 0, io.EOF
	}
	n := copy(p, r.b[r.i:])
	r.i += n
	return n, nil
}
```

Note: for stderr demux-to-caller, the default merges stderr into a discard sink; `WithMultiplexed` (a follow-up option) can route stderr — out of scope for this task, captured in Plan 2.

- [ ] **Step 5: Run test to verify it passes**

Run: `go test ./exec/ -run TestExec_Capture -v`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add exec/exec.go exec/exec_test.go sandbox/sandbox.go
git commit -m "feat(exec): Exec capture over hijack + stdcopy demux"
```

### Task 21: InspectExec + ExecDetached

**Files:**
- Modify: `exec/exec.go`
- Test: `exec/exec_test.go`

- [ ] **Step 1: Write the failing test**

```go
func TestInspectExec(t *testing.T) {
	c, _ := attachStub(t)
	sb := sandbox.NewForTest(c, "s1")
	st, err := InspectExec(context.Background(), sb, "e1")
	require.NoError(t, err)
	require.Equal(t, 0, st.ExitCode)
	require.False(t, st.Running)
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./exec/ -run TestInspectExec -v`
Expected: FAIL — `InspectExec` undefined (it's currently unexported).

- [ ] **Step 3: Export InspectExec + add ExecDetached**

In `exec/exec.go`, rename `inspectExec` usages to call an exported wrapper and add:

```go
// InspectExec returns the state of a previously-started exec.
func InspectExec(ctx context.Context, sb *sandbox.Sandbox, execID string) (State, error) {
	return inspectExec(ctx, sb, execID)
}

// ExecDetached starts cmd in the background and returns its exec id. Poll
// InspectExec for completion. Uses the attach endpoint but does not stream output.
func ExecDetached(ctx context.Context, sb *sandbox.Sandbox, cmd []string, opts ...ProcessOption) (string, error) {
	body := buildBody(cmd, opts...)
	raw, err := json.Marshal(body)
	if err != nil {
		return "", err
	}
	conn, hdr, err := sb.Client().Transport().Hijack(ctx, "/sandbox/"+sb.Name()+"/exec/attach", raw)
	if err != nil {
		return "", client.MapError("exec-detached", err)
	}
	conn.Close() // don't consume the stream; the command keeps running
	return hdr.Get("Sandboxes-Exec-Id"), nil
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./exec/ -v`
Expected: PASS (all exec tests).

- [ ] **Step 5: Commit**

```bash
git add exec/exec.go exec/exec_test.go
git commit -m "feat(exec): InspectExec + ExecDetached"
```

### Task 22: ExecInteractive + AttachSession

**Files:**
- Create: `exec/attach.go`
- Test: `exec/attach_test.go`

- [ ] **Step 1: Write the failing test**

```go
package exec

import (
	"context"
	"io"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/squall-chua/sbx-go-sdk/sandbox"
)

func TestExecInteractive_StreamsAndWaits(t *testing.T) {
	c, _ := attachStub(t)
	sb := sandbox.NewForTest(c, "s1")
	sess, err := ExecInteractive(context.Background(), sb, []string{"echo", "hi"}, WithTTY())
	require.NoError(t, err)
	defer sess.Close()
	out, _ := io.ReadAll(sess.Stdout())
	require.Contains(t, string(out), "hello") // stub streams a stdcopy "hello\n" frame
	code, err := sess.Wait(context.Background())
	require.NoError(t, err)
	require.Equal(t, 0, code)
}
```

Note: the stub streams a stdcopy frame even for TTY; the test asserts the bytes arrive. (Real TTY raw-framing is verified in the integration suite.)

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./exec/ -run TestExecInteractive -v`
Expected: FAIL — `ExecInteractive` undefined.

- [ ] **Step 3: Write minimal implementation**

```go
package exec

import (
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"

	"github.com/squall-chua/sbx-go-sdk/client"
	"github.com/squall-chua/sbx-go-sdk/internal/stdcopy"
	"github.com/squall-chua/sbx-go-sdk/sandbox"
)

// AttachSession is a live bidirectional stream to an exec'd process.
type AttachSession struct {
	sb     *sandbox.Sandbox
	conn   net.Conn
	execID string
	tty    bool
	stdout *io.PipeReader
}

// ExecInteractive starts cmd with a hijacked stream and returns an AttachSession.
// TTY mode yields a raw stream; otherwise stdout/stderr are stdcopy-demuxed (stdout here).
func ExecInteractive(ctx context.Context, sb *sandbox.Sandbox, cmd []string, opts ...ProcessOption) (*AttachSession, error) {
	body := buildBody(cmd, opts...)
	raw, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	conn, hdr, err := sb.Client().Transport().Hijack(ctx, "/sandbox/"+sb.Name()+"/exec/attach", raw)
	if err != nil {
		return nil, client.MapError("exec-interactive", err)
	}
	s := &AttachSession{sb: sb, conn: conn, execID: hdr.Get("Sandboxes-Exec-Id"), tty: body.TTY}
	pr, pw := io.Pipe()
	s.stdout = pr
	go func() {
		if s.tty {
			_, e := io.Copy(pw, conn)
			pw.CloseWithError(e)
			return
		}
		var discard discardCloser
		_, e := stdcopy.Demux(pw, &discard, conn)
		pw.CloseWithError(e)
	}()
	return s, nil
}

// Stdout returns the (demuxed for non-TTY) stdout stream.
func (s *AttachSession) Stdout() io.Reader { return s.stdout }

// Stdin writes to the process stdin.
func (s *AttachSession) Stdin() io.Writer { return s.conn }

// Resize sets the TTY window size.
func (s *AttachSession) Resize(ctx context.Context, cols, rows int) error {
	body := map[string]int{"cols": cols, "rows": rows}
	err := s.sb.Client().Transport().DoJSON(ctx, http.MethodPost,
		"/sandbox/"+s.sb.Name()+"/exec/"+s.execID+"/resize", body, nil)
	if err != nil {
		return client.MapError("resize", err)
	}
	return nil
}

// Wait drains the stream and returns the exit code.
func (s *AttachSession) Wait(ctx context.Context) (int, error) {
	io.Copy(io.Discard, s.stdout)
	st, err := inspectExec(ctx, s.sb, s.execID)
	if err != nil {
		return 0, err
	}
	return st.ExitCode, nil
}

// Close releases the hijacked connection.
func (s *AttachSession) Close() error { return s.conn.Close() }
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./exec/ -v`
Expected: PASS (all exec tests).

- [ ] **Step 5: Commit**

```bash
git add exec/attach.go exec/attach_test.go
git commit -m "feat(exec): ExecInteractive + AttachSession (stream/resize/wait)"
```

---

## Phase 8 — integration smoke test + wrap-up

### Task 23: End-to-end integration smoke test

**Files:**
- Create: `internal/integration/smoke_test.go`

- [ ] **Step 1: Write the build-tagged smoke test**

```go
//go:build integration

package integration

import (
	"context"
	"io"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/squall-chua/sbx-go-sdk/client"
	"github.com/squall-chua/sbx-go-sdk/exec"
	"github.com/squall-chua/sbx-go-sdk/sandbox"
)

// Requires a real sbx on PATH and a reachable daemon. Creates and removes a sandbox.
func TestSmoke_CreateExecRemove(t *testing.T) {
	ctx := context.Background()
	c, err := client.New(ctx, client.WithAutoStart())
	require.NoError(t, err)

	sb, err := sandbox.Create(ctx, c, sandbox.WithAgent("shell"), sandbox.WithWorkspace(t.TempDir()))
	require.NoError(t, err)
	t.Cleanup(func() { sb.Remove(ctx) })

	code, r, err := exec.Exec(ctx, sb, []string{"echo", "sdk-smoke"})
	require.NoError(t, err)
	out, _ := io.ReadAll(r)
	require.Equal(t, 0, code)
	require.Contains(t, string(out), "sdk-smoke")
}
```

- [ ] **Step 2: Run it against the live environment**

Run: `go test -tags integration ./internal/integration/ -run TestSmoke -v`
Expected: PASS (creates a `shell` sandbox, execs echo, removes it). If it fails, the failure pinpoints which layer drifted.

- [ ] **Step 3: Commit**

```bash
git add internal/integration/
git commit -m "test(integration): end-to-end create/exec/remove smoke test"
```

### Task 24: Module-wide build, vet, and unit test gate

**Files:** none (verification + README stub)

- [ ] **Step 1: Run the full unit suite + vet**

Run: `go build ./... && go vet ./... && go test ./...`
Expected: all PASS (integration-tagged tests are skipped without the tag).

- [ ] **Step 2: Add a minimal README**

Create `README.md`:

```markdown
# sbx-go-sdk

A Go SDK to automate Docker Sandboxes (`sbx`). See
[docs/superpowers/specs/2026-06-10-sbx-go-sdk-design.md](docs/superpowers/specs/2026-06-10-sbx-go-sdk-design.md).

```go
ctx := context.Background()
c, _ := client.New(ctx, client.WithAutoStart())
sb, _ := sandbox.Create(ctx, c, sandbox.WithAgent("claude"), sandbox.WithWorkspace("."))
defer sb.Remove(ctx)
code, out, _ := exec.Exec(ctx, sb, []string{"claude", "-p", "summarise the repo"})
```

Plan 1 covers: daemon lifecycle, sandbox create/list/inspect/start/stop/remove, exec
(capture/interactive/detached). Plan 2 adds cp/ports/template/policy/secret.
```

- [ ] **Step 3: Commit**

```bash
git add README.md
git commit -m "docs: add README with quickstart"
```

---

## Self-Review Notes (for the implementer)

- **Import-cycle guard:** `client` must not import `sandbox`/`exec`. `sandbox` imports `client` + `internal/*`. `exec` imports `sandbox` + `client` + `internal/*`. Keep this direction.
- **Generated-type coupling:** Tasks 15–18 assume `api.SandboxInfo` fields `Name`, `Agent (*string)`, `Status`. After Task 8 generates the real struct, reconcile field names/types (e.g. if `Status` is a defined type, `statusString` casts it). The integration drift test (Task 9) is the safety net.
- **stdcopy deviation from spec:** spec §7 says reuse `moby/.../stdcopy`; this plan uses `internal/stdcopy` (identical wire format) to avoid the `moby/moby` dependency. Documented here intentionally.
- **Exec API shape:** package-level `exec.Exec(ctx, sb, …)` rather than `sb.Exec(…)` to avoid the import cycle; ergonomic `sb.Exec` forwarders can be added in the sandbox package in Plan 2 once the dependency direction is fixed (sandbox would call an exec-runner interface).
- **`cli.Inherit`/`Stdio` (Task 7)** is built and tested here but its consumer, `sandbox.Run` (interactive agent attach, create-if-missing → terminal-inherit → exit code), is deferred to Plan 2. It's built now because it's a small, self-contained primitive and the dependency is certain. If you prefer strict YAGNI, skip Task 7 and add it alongside `sandbox.Run` in Plan 2.
- **Deferred daemon endpoint:** `Diagnostics(ctx)` (`GET /daemon/diagnostics`) from spec §5 is not in Plan 1 (not core to the automation loop) — add in Plan 2.

## Plan 2 preview (not in this plan)

- `sandbox.Run` (package one-shot + handle method) + `sb.Exec`/`sb.ExecInteractive` forwarders — wire `cli.Inherit`.
- cp/files (`GetFile`/`PutFile` tar; `CopyTo`/`CopyFrom` + `CopyTarTo`/`CopyTarFrom`).
- ports (`/sandbox/{name}/ports`: list/publish/unpublish).
- template (`/docker/images*` REST list/inspect/remove/load + shell-out `template save`).
- policy (shell-out `sbx policy` allow/deny/ls/reset/set-default/profiles; `policy.Log` REST `/network/log`).
- secret (shell-out `sbx secret set-custom/ls/rm`).
- `client.Diagnostics`, `exec.WithMultiplexed` (split stdout/stderr writers).

Each reuses `internal/transport` + `internal/cli` + `internal/stdcopy` built here.
