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

// Captured verbatim from `sbx prune --dry-run` at sbx v0.39.0.
const pruneDryRunOutput = `Would remove 2 stopped sandbox(es):
SANDBOX     AGENT   STOPPED                  WORKSPACE
probe-env   shell   Less than a second ago   /tmp
older       claude  3 days ago               /home/me/proj`

// Captured verbatim from `sbx prune -f` at sbx v0.39.0.
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
}

func TestPrune_DryRunReadsTheCandidateTable(t *testing.T) {
	argFile := filepath.Join(t.TempDir(), "args.txt")
	c := clientPrinting(t, argFile, pruneDryRunOutput)

	got, err := Prune(context.Background(), c, WithDryRun())
	require.NoError(t, err)
	require.Equal(t, []string{"probe-env", "older"}, got)

	args, _ := os.ReadFile(argFile)
	require.Contains(t, string(args), "--dry-run")
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
	for _, opts := range [][]PruneOption{nil, {WithDryRun()}} {
		c := clientPrinting(t, filepath.Join(t.TempDir(), "args.txt"), "No stopped sandboxes to prune.")
		got, err := Prune(context.Background(), c, opts...)
		require.NoError(t, err)
		require.Empty(t, got)
	}
}

// A dry run whose table lost its header must fail loudly. Reporting "nothing
// to prune" for an unreadable table is the silent-wrong-answer failure that
// mcp.List shipped with at v0.39.0.
func TestPrune_DryRunWithoutAHeaderIsUnexpectedFormat(t *testing.T) {
	c := clientPrinting(t, filepath.Join(t.TempDir(), "args.txt"),
		"Would remove 1 stopped sandbox(es):\n  probe-env   shell   3 days ago   /tmp")

	_, err := Prune(context.Background(), c, WithDryRun())
	require.ErrorIs(t, err, client.ErrUnexpectedFormat)
	require.True(t, strings.Contains(err.Error(), "prune"))
}
