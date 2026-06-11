package exec

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"github.com/squall-chua/sbx-go-sdk/client"
	"github.com/squall-chua/sbx-go-sdk/internal/stdcopy"
	"github.com/squall-chua/sbx-go-sdk/internal/transport"
	"github.com/squall-chua/sbx-go-sdk/sandbox"
)

// State is the /sandbox/{name}/exec/{id} response.
type State struct {
	ExitCode int  `json:"exit_code"`
	Running  bool `json:"running"`
}

// Exec runs cmd to completion and returns (exitCode, output, error). The returned
// reader carries the demultiplexed stdout stream; stderr is discarded unless
// WithMultiplexed is used to route both streams to caller-supplied writers, in
// which case the returned reader is empty. The sandbox must be running, or pass
// WithAutoStart to start a stopped one (otherwise ErrSandboxNotRunning is returned).
func Exec(ctx context.Context, sb *sandbox.Sandbox, cmd []string, opts ...ProcessOption) (int, io.Reader, error) {
	cfg := parseConfig(opts...)
	if err := ensureRunnable(ctx, sb, cfg); err != nil {
		return 0, nil, err
	}
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
	var out []byte
	if cfg.muxOut != nil || cfg.muxErr != nil {
		// Route demuxed streams straight to the caller's writers.
		_, derr := stdcopy.Demux(orDiscard(cfg.muxOut), orDiscard(cfg.muxErr), conn)
		if derr != nil {
			return 0, byteReader(nil), client.MapError("exec", derr)
		}
	} else {
		// Demux into an in-memory buffer (capture semantics).
		pr, pw := io.Pipe()
		go func() {
			var sink discardCloser
			_, derr := stdcopy.Demux(pw, &sink, conn)
			pw.CloseWithError(derr)
		}()
		var rerr error
		out, rerr = io.ReadAll(pr)
		if rerr != nil {
			return 0, byteReader(out), client.MapError("exec", rerr)
		}
	}

	st, err := inspectExec(ctx, sb, execID)
	if err != nil {
		return 0, byteReader(out), err
	}
	return st.ExitCode, byteReader(out), nil
}

// parseConfig applies opts to a processConfig to read back option values.
func parseConfig(opts ...ProcessOption) processConfig {
	var c processConfig
	for _, o := range opts {
		o(&c)
	}
	return c
}

// ensureRunnable enforces the exec precondition: the sandbox must be running. If
// WithAutoStart is set it starts a stopped sandbox first; otherwise a stopped
// sandbox yields ErrSandboxNotRunning.
func ensureRunnable(ctx context.Context, sb *sandbox.Sandbox, cfg processConfig) error {
	info, err := sb.Inspect(ctx)
	if err != nil {
		return err
	}
	if info.Status == sandbox.StatusRunning {
		return nil
	}
	if !cfg.autoStart {
		return client.ErrSandboxNotRunning
	}
	return sb.Start(ctx)
}

// orDiscard returns w, or an always-succeeding discard writer if w is nil.
func orDiscard(w io.Writer) io.Writer {
	if w == nil {
		return discardCloser{}
	}
	return w
}

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
		ae := &client.APIError{Op: "inspect-exec", Status: 404, Message: client.ParseMessage(se.Body)}
		return errors.Join(client.ErrExecNotFound, ae)
	}
	return client.MapError("inspect-exec", err)
}

// InspectExec returns the state of a previously-started exec.
func InspectExec(ctx context.Context, sb *sandbox.Sandbox, execID string) (State, error) {
	return inspectExec(ctx, sb, execID)
}

// ExecDetached starts cmd in the background and returns its exec id. Poll
// InspectExec for completion. Uses the attach endpoint but does not stream output.
func ExecDetached(ctx context.Context, sb *sandbox.Sandbox, cmd []string, opts ...ProcessOption) (string, error) {
	cfg := parseConfig(opts...)
	if err := ensureRunnable(ctx, sb, cfg); err != nil {
		return "", err
	}
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
