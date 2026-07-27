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

// kitFakeClient returns a client whose fake sbx binary records its args.
func kitFakeClient(t *testing.T, argFile string) *client.Client {
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

// The daemon records the kit list verbatim and re-resolves it on every later
// add, resolving a relative path against its own working directory rather
// than the caller's. A relative path therefore records one that does not
// exist and breaks every subsequent add on that sandbox.
func TestAbsLocal_ExistingRelativePathBecomesAbsolute(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.Mkdir(filepath.Join(dir, "mykit"), 0o755))
	t.Chdir(dir)

	got := absLocal("./mykit")

	require.True(t, filepath.IsAbs(got), "want an absolute path, got %q", got)
	require.Equal(t, filepath.Join(dir, "mykit"), filepath.Clean(got))
}

func TestAbsLocal_NonPathReferencesPassThroughUntouched(t *testing.T) {
	t.Chdir(t.TempDir())

	for _, ref := range []string{
		"ghcr.io/myorg/mcp-postgres:1.0",
		"git+https://github.com/org/kits.git#dir=mcp-postgres",
		"./does-not-exist",
	} {
		require.Equal(t, ref, absLocal(ref))
	}
}

func TestAddKit_PassesSandboxNameThenReference(t *testing.T) {
	argFile := filepath.Join(t.TempDir(), "args.txt")
	c := kitFakeClient(t, argFile)
	sb := NewForTest(c, "my-sandbox")

	require.NoError(t, sb.AddKit(context.Background(), "ghcr.io/org/kit:1.0"))

	args, err := os.ReadFile(argFile)
	require.NoError(t, err)
	require.Contains(t, string(args), "kit add my-sandbox ghcr.io/org/kit:1.0")
}

func TestAddKit_AbsolutisesALocalDirectory(t *testing.T) {
	argFile := filepath.Join(t.TempDir(), "args.txt")
	c := kitFakeClient(t, argFile)
	sb := NewForTest(c, "my-sandbox")

	dir := t.TempDir()
	require.NoError(t, os.Mkdir(filepath.Join(dir, "mykit"), 0o755))
	t.Chdir(dir)

	require.NoError(t, sb.AddKit(context.Background(), "./mykit"))

	args, err := os.ReadFile(argFile)
	require.NoError(t, err)
	require.NotContains(t, string(args), "./mykit",
		"a relative path reaches the daemon and poisons the recorded kit list")
	require.Contains(t, string(args), filepath.Join(dir, "mykit"))
}
