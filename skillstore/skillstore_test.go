package skillstore

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/squall-chua/sbx-go-sdk/client"
	"github.com/stretchr/testify/require"
)

func recordingClient(t *testing.T, argFile string) *client.Client {
	t.Helper()
	bin := filepath.Join(t.TempDir(), "sbx")
	script := "#!/bin/sh\nprintf '%s\\n' \"$*\" >> " + argFile + "\nexit 0\n"
	require.NoError(t, os.WriteFile(bin, []byte(script), 0o755))
	c, err := client.New(context.Background(), client.WithBinaryPath(bin))
	require.NoError(t, err)
	return c
}

func TestImport_AlwaysForcesToStayNonInteractive(t *testing.T) {
	argFile := filepath.Join(t.TempDir(), "args.txt")
	c := recordingClient(t, argFile)

	require.NoError(t, Import(context.Background(), c))

	args, err := os.ReadFile(argFile)
	require.NoError(t, err)
	require.Contains(t, string(args), "skills import")
	require.Contains(t, string(args), "--force",
		"without --force the CLI prompts before overwriting and the call would hang")
}

func TestImport_DryRun(t *testing.T) {
	argFile := filepath.Join(t.TempDir(), "args.txt")
	c := recordingClient(t, argFile)

	require.NoError(t, Import(context.Background(), c, WithDryRun()))

	args, err := os.ReadFile(argFile)
	require.NoError(t, err)
	require.Contains(t, string(args), "--dry-run")
}

// printingClient prints stdout instead of staying silent, so List has
// something to parse.
func printingClient(t *testing.T, stdout string) *client.Client {
	t.Helper()
	bin := filepath.Join(t.TempDir(), "sbx")
	script := "#!/bin/sh\ncat <<'SBXOUT'\n" + stdout + "\nSBXOUT\n"
	require.NoError(t, os.WriteFile(bin, []byte(script), 0o755))
	c, err := client.New(context.Background(), client.WithBinaryPath(bin))
	require.NoError(t, err)
	return c
}

// Captured verbatim from `sbx skills ls` at sbx v0.39.0.
const lsOutput = `Skills store: /home/me/.local/state/sandboxes/sandboxes/agent-skills
code-review
diagnosing-bugs
oldman`

func TestList_SplitsThePathFromTheSkills(t *testing.T) {
	st, err := List(context.Background(), printingClient(t, lsOutput))
	require.NoError(t, err)
	require.Equal(t, "/home/me/.local/state/sandboxes/sandboxes/agent-skills", st.Path)
	require.Equal(t, []string{"code-review", "diagnosing-bugs", "oldman"}, st.Skills)
}

// The store directory exists whether or not anything was imported into it.
func TestList_EmptyStoreStillReportsItsPath(t *testing.T) {
	st, err := List(context.Background(), printingClient(t, "Skills store: /var/lib/skills"))
	require.NoError(t, err)
	require.Equal(t, "/var/lib/skills", st.Path)
	require.Empty(t, st.Skills)
}

// Without the path line there is no way to tell a skill name from a stray
// message, so the whole parse is refused rather than guessed at.
func TestList_MissingPathLineIsUnexpectedFormat(t *testing.T) {
	_, err := List(context.Background(), printingClient(t, "code-review\ndiagnosing-bugs"))
	require.ErrorIs(t, err, client.ErrUnexpectedFormat)
}
