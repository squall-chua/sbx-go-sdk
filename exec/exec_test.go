package exec

import (
	"bufio"
	"bytes"
	"context"
	"encoding/binary"
	"io"
	"net"
	"net/http"
	"path/filepath"
	"testing"

	"github.com/squall-chua/sbx-go-sdk/client"
	"github.com/squall-chua/sbx-go-sdk/sandbox"
	"github.com/stretchr/testify/require"
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
		conn.Write(frame(2, "err\n"))
	case req.URL.Path == "/sandbox/s1/exec/e1":
		body := `{"exit_code":0,"running":false}`
		conn.Write([]byte("HTTP/1.1 200 OK\r\nContent-Type: application/json\r\n" +
			"Content-Length: " + itoa(len(body)) + "\r\n\r\n" + body))
	case req.URL.Path == "/sandbox/s1/exec/missing":
		conn.Write([]byte("HTTP/1.1 404 Not Found\r\nContent-Type: application/json\r\n" +
			"Content-Length: 27\r\n\r\n{\"message\":\"exec not found\"}"))
	case req.URL.Path == "/sandbox/s1":
		body := `{"name":"s1","status":"SANDBOX_STATUS_RUNNING"}`
		conn.Write([]byte("HTTP/1.1 200 OK\r\nContent-Type: application/json\r\n" +
			"Content-Length: " + itoa(len(body)) + "\r\n\r\n" + body))
	case req.URL.Path == "/sandbox/stopped":
		body := `{"name":"stopped","status":"SANDBOX_STATUS_STOPPED"}`
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
	sb := sandbox.NewForTest(c, "s1") // test-only constructor
	code, r, err := Exec(context.Background(), sb, []string{"echo", "hello"})
	require.NoError(t, err)
	out, _ := io.ReadAll(r)
	require.Equal(t, "hello\n", string(out))
	require.Equal(t, 0, code)
}

func TestInspectExec(t *testing.T) {
	c, _ := attachStub(t)
	sb := sandbox.NewForTest(c, "s1")
	st, err := InspectExec(context.Background(), sb, "e1")
	require.NoError(t, err)
	require.Equal(t, 0, st.ExitCode)
	require.False(t, st.Running)
}

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
