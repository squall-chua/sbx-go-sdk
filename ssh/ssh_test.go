package ssh

import (
	"context"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/squall-chua/sbx-go-sdk/client"
	"github.com/stretchr/testify/require"
)

func newFakeSbx(t *testing.T, exit int, stdout, stderr string) (*client.Client, string) {
	t.Helper()
	dir := t.TempDir()
	argFile := filepath.Join(dir, "args.txt")
	sock := filepath.Join(dir, "d.sock")
	l, err := net.Listen("unix", sock)
	require.NoError(t, err)
	srv := &http.Server{Handler: http.NewServeMux()}
	go srv.Serve(l)
	t.Cleanup(func() { srv.Close() })
	bin := filepath.Join(dir, "sbx")
	script := "#!/bin/sh\n" +
		"printf '%s\\n' \"$*\" >> " + argFile + "\n" +
		"cat <<'STDOUT_EOF'\n" + stdout + "\nSTDOUT_EOF\n" +
		"cat >&2 <<'STDERR_EOF'\n" + stderr + "\nSTDERR_EOF\n" +
		"exit " + strconv.Itoa(exit) + "\n"
	require.NoError(t, os.WriteFile(bin, []byte(script), 0o755))
	c, err := client.New(context.Background(), client.WithSocketPath(sock), client.WithBinaryPath(bin))
	require.NoError(t, err)
	return c, argFile
}

func TestTargetArgsCommand(t *testing.T) {
	tg := Target{User: "mybox", Host: "127.0.0.1", Port: 2222}
	require.Equal(t, []string{"mybox@127.0.0.1", "-p", "2222"}, tg.Args())
	require.Equal(t, "ssh mybox@127.0.0.1 -p 2222", tg.Command())
}

func TestPortAndTargetFor(t *testing.T) {
	c, argFile := newFakeSbx(t, 0, `{"key":"ssh.port","value":2222,"type":"int","source":"default","description":"port"}`, "")
	ctx := context.Background()

	p, err := Port(ctx, c)
	require.NoError(t, err)
	require.Equal(t, 2222, p)

	tg, err := TargetFor(ctx, c, "mybox")
	require.NoError(t, err)
	require.Equal(t, Target{User: "mybox", Host: "127.0.0.1", Port: 2222}, tg)

	args, _ := os.ReadFile(argFile)
	require.Contains(t, string(args), "settings get --json ssh.port")
}

func TestEnableDisableArgs(t *testing.T) {
	c, argFile := newFakeSbx(t, 0, "", "")
	ctx := context.Background()
	require.NoError(t, Enable(ctx, c))
	require.NoError(t, Disable(ctx, c))
	args, _ := os.ReadFile(argFile)
	lines := string(args)
	require.Contains(t, lines, "settings set feature.ssh true")
	require.Contains(t, lines, "settings set feature.ssh false")
}

func TestPortEnabledWrapMalformedValue(t *testing.T) {
	ctx := context.Background()

	c, _ := newFakeSbx(t, 0, `{"key":"ssh.port","value":"notanint","type":"int","source":"default","description":""}`, "")
	_, err := Port(ctx, c)
	require.ErrorIs(t, err, client.ErrUnexpectedFormat)

	c, _ = newFakeSbx(t, 0, `{"key":"feature.ssh","value":"notanobject","type":"json","source":"default","description":""}`, "")
	_, err = Enabled(ctx, c)
	require.ErrorIs(t, err, client.ErrUnexpectedFormat)
}

func TestEnabledParsesFeatureFlag(t *testing.T) {
	c, _ := newFakeSbx(t, 0, `{"key":"feature.ssh","value":{"enabled":true,"variant":""},"type":"json","source":"override","description":"flag"}`, "")
	on, err := Enabled(context.Background(), c)
	require.NoError(t, err)
	require.True(t, on)
}
