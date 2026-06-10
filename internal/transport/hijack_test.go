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
