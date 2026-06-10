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
