package exec

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"path/filepath"
	"strings"
	"testing"

	"github.com/squall-chua/sbx-go-sdk/client"
	"github.com/squall-chua/sbx-go-sdk/sandbox"
	"github.com/stretchr/testify/require"
)

// logsStub records the exec command sent to /exec/attach (over cmdCh) and streams
// a single stdout frame, answering the inspect precondition as running.
func logsStub(t *testing.T) (*client.Client, <-chan []string) {
	t.Helper()
	cmdCh := make(chan []string, 1)
	sock := filepath.Join(t.TempDir(), "d.sock")
	l, err := net.Listen("unix", sock)
	require.NoError(t, err)
	go func() {
		for {
			conn, err := l.Accept()
			if err != nil {
				return
			}
			go func(conn net.Conn) {
				defer conn.Close()
				req, err := http.ReadRequest(bufio.NewReader(conn))
				if err != nil {
					return
				}
				body, _ := io.ReadAll(req.Body)
				if strings.HasSuffix(req.URL.Path, "/exec/attach") {
					var eb execBody
					json.Unmarshal(body, &eb)
					select {
					case cmdCh <- eb.Cmd:
					default:
					}
					conn.Write([]byte("HTTP/1.1 101 Switching Protocols\r\n" +
						"Content-Type: application/vnd.docker.raw-stream\r\n" +
						"Sandboxes-Exec-Id: e1\r\nConnection: Upgrade\r\nUpgrade: tcp\r\n\r\n"))
					conn.Write(frame(1, "hello\n"))
					return
				}
				writeJSON(conn, `{"name":"s1","status":"running"}`)
			}(conn)
		}
	}()
	t.Cleanup(func() { l.Close() })
	c, err := client.New(context.Background(), client.WithSocketPath(sock))
	require.NoError(t, err)
	return c, cmdCh
}

func TestLogs_FollowsFileAndStreams(t *testing.T) {
	c, cmdCh := logsStub(t)
	sb := sandbox.NewForTest(c, "s1")
	sess, err := Logs(context.Background(), sb, "/var/log/app.log")
	require.NoError(t, err)
	defer sess.Close()

	out, _ := io.ReadAll(sess.Stdout())
	require.Equal(t, "hello\n", string(out))
	require.Equal(t, []string{"tail", "-F", "/var/log/app.log"}, <-cmdCh)
}
