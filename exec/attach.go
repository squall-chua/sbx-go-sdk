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
