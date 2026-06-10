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

// Exec runs cmd to completion and returns (exitCode, output, error). The returned
// reader carries the demultiplexed stdout stream; stderr is discarded in this
// release (splitting stdout/stderr is planned). The sandbox must already be running.
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
