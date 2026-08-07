package client

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

// Captured verbatim from `sbx setup </dev/null` at sbx v0.38.0.
const setupOutput = `sbx setup — detected configuration

PREREQUISITES
  host           sbx, Docker                                   Ready

SECRETS
  agent secrets  none found on host                            0 found

SKILLS
  skills         54 skill(s), 2 folders                        11 conflict(s)

GOVERNANCE
  Local policy   balanced                                      Set

MCP
  mcp            chrome-devtools                               1 found

(non-interactive: showing detected configuration only)`

func TestParseSetupReport(t *testing.T) {
	got, err := parseSetupReport(setupOutput)
	require.NoError(t, err)
	require.Len(t, got.Items, 5)

	require.Equal(t, SetupItem{
		Section: "PREREQUISITES", Name: "host", Detail: "sbx, Docker", Status: "Ready",
	}, got.Items[0])
	require.Equal(t, SetupItem{
		Section: "SKILLS", Name: "skills", Detail: "54 skill(s), 2 folders", Status: "11 conflict(s)",
	}, got.Items[2])

	// The title line and the trailing "(non-interactive: …)" note are not rows.
	for _, it := range got.Items {
		require.NotEmpty(t, it.Section)
		require.NotContains(t, it.Name, "non-interactive")
	}

	mcp := got.Section("MCP")
	require.Len(t, mcp, 1)
	require.Equal(t, "chrome-devtools", mcp[0].Detail)
	require.Empty(t, got.Section("NOPE"))
}

// A two-column row must not lose its detail into Status.
func TestParseSetupReport_TwoColumnRow(t *testing.T) {
	got, err := parseSetupReport("SECRETS\n  agent secrets  none found on host\n")
	require.NoError(t, err)
	require.Len(t, got.Items, 1)
	require.Equal(t, "none found on host", got.Items[0].Detail)
	require.Empty(t, got.Items[0].Status)
}

func TestParseSetupReport_ReportsFormatDrift(t *testing.T) {
	_, err := parseSetupReport("sbx setup — detected configuration\n\n(nothing detected)")
	require.ErrorIs(t, err, ErrUnexpectedFormat)
}

// A terminal on stdin is what turns `sbx setup` into a wizard that writes to the
// host, so the child must get an empty stdin, never the caller's.
func TestDetectSetup_GivesTheChildEmptyStdin(t *testing.T) {
	dir := t.TempDir()
	argFile, stdinFile := filepath.Join(dir, "args.txt"), filepath.Join(dir, "stdin.txt")
	bin := filepath.Join(dir, "sbx")
	script := "#!/bin/sh\nprintf '%s\\n' \"$*\" >> " + argFile + "\ncat > " + stdinFile + "\ncat <<'EOF'\n" + setupOutput + "\nEOF\n"
	require.NoError(t, os.WriteFile(bin, []byte(script), 0o755))

	c, err := New(context.Background(), WithBinaryPath(bin))
	require.NoError(t, err)

	rep, err := c.DetectSetup(context.Background())
	require.NoError(t, err)
	require.Len(t, rep.Items, 5)

	args, err := os.ReadFile(argFile)
	require.NoError(t, err)
	require.Contains(t, string(args), "setup")

	stdin, err := os.ReadFile(stdinFile)
	require.NoError(t, err)
	require.Empty(t, stdin, "the child must not inherit a terminal on stdin")
}
