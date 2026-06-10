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
	sb := sandbox.NewForTest(c, "s1") // test-only constructor
	code, r, err := Exec(context.Background(), sb, []string{"echo", "hello"})
	require.NoError(t, err)
	out, _ := io.ReadAll(r)
	require.Equal(t, "hello\n", string(out))
	require.Equal(t, 0, code)
}
