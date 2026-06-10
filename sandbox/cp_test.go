package sandbox

import (
	"context"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"github.com/squall-chua/sbx-go-sdk/client"
	"github.com/stretchr/testify/require"
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
