package sandbox

import (
	"context"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/squall-chua/sbx-go-sdk/client"
	"github.com/stretchr/testify/require"
)

// clientPrinting returns a client whose fake sbx records its args to argFile
// and prints stdout.
func clientPrinting(t *testing.T, argFile, stdout string) *client.Client {
	t.Helper()
	sock := filepath.Join(t.TempDir(), "d.sock")
	l, err := net.Listen("unix", sock)
	require.NoError(t, err)
	srv := &http.Server{Handler: http.NewServeMux()}
	go srv.Serve(l)
	t.Cleanup(func() { srv.Close() })
	bin := filepath.Join(t.TempDir(), "sbx")
	script := "#!/bin/sh\n" +
		"printf '%s\\n' \"$*\" >> " + argFile + "\n" +
		"cat <<'SBXOUT'\n" + stdout + "\nSBXOUT\n"
	require.NoError(t, os.WriteFile(bin, []byte(script), 0o755))
	c, err := client.New(context.Background(), client.WithSocketPath(sock), client.WithBinaryPath(bin))
	require.NoError(t, err)
	return c
}

// Shaped like `sbx prune --dry-run --json` at sbx v0.42.1, with a second
// candidate and a skipped name added.
const pruneDryRunOutput = `{
  "would_remove": [
    {
      "name": "probe-env",
      "agent": "shell",
      "stopped_at": "2026-09-14T05:10:35Z",
      "workspaces": [
        "/tmp"
      ]
    },
    {
      "name": "older",
      "agent": "claude",
      "stopped_at": "2026-09-11T05:10:35Z",
      "workspaces": [
        "/home/me/proj"
      ]
    }
  ],
  "skipped_unknown_stop": ["no-stop-time"]
}`

// Captured verbatim from `sbx prune -f` at sbx v0.39.0; unchanged at v0.42.1.
const pruneRemovedOutput = `Deleting sandbox probe-env...
Sandbox 'probe-env' removed
Deleting sandbox older...
Sandbox 'older' removed`

func TestPrune_RemovesAndReportsNames(t *testing.T) {
	argFile := filepath.Join(t.TempDir(), "args.txt")
	c := clientPrinting(t, argFile, pruneRemovedOutput)

	got, err := Prune(context.Background(), c)
	require.NoError(t, err)
	require.Equal(t, []string{"probe-env", "older"}, got)

	args, _ := os.ReadFile(argFile)
	require.Contains(t, string(args), "prune --force",
		"without --force the CLI refuses outright on a non-interactive stdin")
	require.NotContains(t, string(args), "--dry-run")
	require.NotContains(t, string(args), "--json", "the CLI refuses --json on a real prune")
}

func TestPrune_DryRunReadsTheCandidateList(t *testing.T) {
	argFile := filepath.Join(t.TempDir(), "args.txt")
	c := clientPrinting(t, argFile, pruneDryRunOutput)

	got, err := Prune(context.Background(), c, WithDryRun())
	require.NoError(t, err)
	require.Equal(t, []string{"probe-env", "older"}, got)

	args, _ := os.ReadFile(argFile)
	require.Contains(t, string(args), "prune --dry-run --json")
	require.NotContains(t, string(args), "--force",
		"a dry run must never carry the flag that also removes in-use sandboxes")
}

func TestPrune_SinceFilter(t *testing.T) {
	argFile := filepath.Join(t.TempDir(), "args.txt")
	c := clientPrinting(t, argFile, pruneRemovedOutput)

	_, err := Prune(context.Background(), c, WithStoppedLongerThan("24h"))
	require.NoError(t, err)

	args, _ := os.ReadFile(argFile)
	require.Contains(t, string(args), "--filter since=24h")
}

// Nothing to prune is the ordinary case, not a failure.
func TestPrune_NothingToPruneIsEmptyNotAnError(t *testing.T) {
	for _, tc := range []struct {
		out  string
		opts []PruneOption
	}{
		{"No stopped sandboxes to prune.", nil},
		{`{"would_remove": [], "skipped_unknown_stop": []}`, []PruneOption{WithDryRun()}},
	} {
		c := clientPrinting(t, filepath.Join(t.TempDir(), "args.txt"), tc.out)
		got, err := Prune(context.Background(), c, tc.opts...)
		require.NoError(t, err)
		require.Empty(t, got)
	}
}

// A dry run whose output is not the expected JSON must fail loudly. Reporting
// "nothing to prune" for unreadable output is the silent-wrong-answer failure
// that mcp.List shipped with at v0.39.0.
func TestPrune_DryRunUnreadableIsUnexpectedFormat(t *testing.T) {
	for _, out := range []string{
		"Would remove 1 stopped sandbox(es):\n  probe-env   shell   3 days ago   /tmp",
		`{"skipped_unknown_stop": []}`,
	} {
		c := clientPrinting(t, filepath.Join(t.TempDir(), "args.txt"), out)
		_, err := Prune(context.Background(), c, WithDryRun())
		require.ErrorIs(t, err, client.ErrUnexpectedFormat, out)
		require.True(t, strings.Contains(err.Error(), "prune"))
	}
}
