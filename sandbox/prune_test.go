package sandbox

import (
	"context"
	"encoding/json"
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

// clientPruning fakes both halves of a real prune: the sbx script prints
// dryRun for a `--dry-run` call and nothing otherwise, and the daemon's
// GET /sandbox reports remaining as what is left afterwards.
func clientPruning(t *testing.T, argFile, dryRun string, remaining ...string) *client.Client {
	t.Helper()
	sock := filepath.Join(t.TempDir(), "d.sock")
	l, err := net.Listen("unix", sock)
	require.NoError(t, err)
	mux := http.NewServeMux()
	mux.HandleFunc("GET /sandbox", func(w http.ResponseWriter, _ *http.Request) {
		infos := make([]map[string]string, 0, len(remaining))
		for _, n := range remaining {
			infos = append(infos, map[string]string{"name": n, "status": "running"})
		}
		require.NoError(t, json.NewEncoder(w).Encode(infos))
	})
	srv := &http.Server{Handler: mux}
	go srv.Serve(l)
	t.Cleanup(func() { srv.Close() })
	bin := filepath.Join(t.TempDir(), "sbx")
	script := "#!/bin/sh\n" +
		"printf '%s\\n' \"$*\" >> " + argFile + "\n" +
		"case \"$*\" in *--dry-run*) cat <<'SBXOUT'\n" + dryRun + "\nSBXOUT\n;; esac\n"
	require.NoError(t, os.WriteFile(bin, []byte(script), 0o755))
	c, err := client.New(context.Background(), client.WithSocketPath(sock), client.WithBinaryPath(bin))
	require.NoError(t, err)
	return c
}

// A real prune reports the candidates the daemon no longer lists, not what the
// CLI printed: "older" survived (say, it was restarted mid-prune), so it is not
// reported as removed.
func TestPrune_RemovesAndReportsNames(t *testing.T) {
	argFile := filepath.Join(t.TempDir(), "args.txt")
	c := clientPruning(t, argFile, pruneDryRunOutput, "older", "bystander")

	got, err := Prune(context.Background(), c)
	require.NoError(t, err)
	require.Equal(t, []string{"probe-env"}, got)

	args, _ := os.ReadFile(argFile)
	lines := strings.Split(strings.TrimSpace(string(args)), "\n")
	require.Equal(t, []string{"prune --dry-run --json", "prune --force"}, lines,
		"without --force the CLI refuses outright on a non-interactive stdin; it refuses --json on a real prune")
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
	c := clientPruning(t, argFile, pruneDryRunOutput)

	_, err := Prune(context.Background(), c, WithStoppedLongerThan("24h"))
	require.NoError(t, err)

	args, _ := os.ReadFile(argFile)
	require.Equal(t, "prune --dry-run --json --filter since=24h\nprune --force --filter since=24h\n", string(args),
		"the dry run and the real prune must select the same sandboxes")
}

// Nothing to prune is the ordinary case, not a failure.
func TestPrune_NothingToPruneIsEmptyNotAnError(t *testing.T) {
	for _, tc := range []struct {
		out  string
		opts []PruneOption
	}{
		{`{"would_remove": [], "skipped_unknown_stop": []}`, nil},
		{`{"would_remove": [], "skipped_unknown_stop": []}`, []PruneOption{WithDryRun()}},
	} {
		argFile := filepath.Join(t.TempDir(), "args.txt")
		c := clientPruning(t, argFile, tc.out)
		got, err := Prune(context.Background(), c, tc.opts...)
		require.NoError(t, err)
		require.Empty(t, got)
		args, _ := os.ReadFile(argFile)
		require.NotContains(t, string(args), "--force", "nothing to prune means no real prune is run")
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
