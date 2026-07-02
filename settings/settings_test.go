package settings

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/squall-chua/sbx-go-sdk/client"
	"github.com/stretchr/testify/require"
)

func TestSettingBool(t *testing.T) {
	s := Setting{Key: "feature.x", Value: json.RawMessage(`false`), Type: "bool"}
	b, err := s.Bool()
	require.NoError(t, err)
	require.False(t, b)

	_, err = Setting{Key: "ssh.port", Value: json.RawMessage(`2222`)}.Bool()
	require.Error(t, err) // not a bool
}

func TestSettingText(t *testing.T) {
	require.Equal(t, "shell", Setting{Value: json.RawMessage(`"shell"`)}.Text())                 // string -> unquoted
	require.Equal(t, "2222", Setting{Value: json.RawMessage(`2222`)}.Text())                     // number -> as-is
	require.Equal(t, `["docker.io/"]`, Setting{Value: json.RawMessage(`["docker.io/"]`)}.Text()) // array -> raw JSON
}

// newFakeSbx builds a Client whose sbx binary is a shell script: it records its
// args (space-joined) to the returned file, prints stdout, prints stderr, and
// exits with the given code. A stub unix socket satisfies client.New.
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

func TestList(t *testing.T) {
	out := `[{"key":"ssh.port","value":2222,"type":"int","source":"default","description":"port"},
	         {"key":"feature.ssh","value":{"enabled":false},"type":"json","source":"default","description":"flag"}]`
	c, argFile := newFakeSbx(t, 0, out, "")
	ss, err := List(context.Background(), c)
	require.NoError(t, err)
	require.Len(t, ss, 2)
	require.Equal(t, "ssh.port", ss[0].Key)
	require.Equal(t, "int", ss[0].Type)
	args, _ := os.ReadFile(argFile)
	require.Contains(t, string(args), "settings list --json")
}

func TestGet(t *testing.T) {
	c, argFile := newFakeSbx(t, 0, `{"key":"ssh.port","value":2222,"type":"int","source":"default","description":"port"}`, "")
	s, err := Get(context.Background(), c, "ssh.port")
	require.NoError(t, err)
	require.Equal(t, "ssh.port", s.Key)
	require.Equal(t, "2222", s.Text())
	args, _ := os.ReadFile(argFile)
	require.Contains(t, string(args), "settings get --json ssh.port")
}

func TestGetErrorPropagatesCLIError(t *testing.T) {
	c, _ := newFakeSbx(t, 1, "", "key not defined")
	_, err := Get(context.Background(), c, "nope.key")
	require.Error(t, err)
	var ce *client.CLIError
	require.ErrorAs(t, err, &ce) // raw shell-out error, no sentinel
	require.Equal(t, 1, ce.ExitCode)
}

func TestListGetWrapMalformedJSON(t *testing.T) {
	ctx := context.Background()

	c, _ := newFakeSbx(t, 0, "{ not json", "")
	_, err := List(ctx, c)
	require.ErrorIs(t, err, client.ErrUnexpectedFormat)

	c, _ = newFakeSbx(t, 0, "{ not json", "")
	_, err = Get(ctx, c, "ssh.port")
	require.ErrorIs(t, err, client.ErrUnexpectedFormat)
}

func TestSetUnsetArgs(t *testing.T) {
	c, argFile := newFakeSbx(t, 0, "", "")
	ctx := context.Background()
	require.NoError(t, Set(ctx, c, "feature.ssh", "true"))
	require.NoError(t, Set(ctx, c, "kit.allowedSources", `["docker.io/"]`))
	require.NoError(t, Unset(ctx, c, "feature.ssh"))
	args, _ := os.ReadFile(argFile)
	lines := string(args)
	require.Contains(t, lines, "settings set feature.ssh true")
	require.Contains(t, lines, `settings set kit.allowedSources ["docker.io/"]`)
	require.Contains(t, lines, "settings unset feature.ssh")
}
