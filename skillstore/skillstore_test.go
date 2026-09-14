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

// printingClient records its args to argFile and prints stdout, so List has
// something to parse.
func printingClient(t *testing.T, argFile, stdout string) *client.Client {
	t.Helper()
	bin := filepath.Join(t.TempDir(), "sbx")
	script := "#!/bin/sh\nprintf '%s\\n' \"$*\" >> " + argFile + "\ncat <<'SBXOUT'\n" + stdout + "\nSBXOUT\n"
	require.NoError(t, os.WriteFile(bin, []byte(script), 0o755))
	c, err := client.New(context.Background(), client.WithBinaryPath(bin))
	require.NoError(t, err)
	return c
}

// Captured from `sbx skills ls --json` at sbx v0.42.1, trimmed to three skills.
const lsOutput = `{
  "store": "/home/me/.local/state/sandboxes/sandboxes/agent-skills",
  "skills": [
    "code-review",
    "diagnosing-bugs",
    "oldman"
  ]
}`

func TestList_ReadsThePathAndTheSkills(t *testing.T) {
	argFile := filepath.Join(t.TempDir(), "args.txt")
	st, err := List(context.Background(), printingClient(t, argFile, lsOutput))
	require.NoError(t, err)
	require.Equal(t, "/home/me/.local/state/sandboxes/sandboxes/agent-skills", st.Path)
	require.Equal(t, []string{"code-review", "diagnosing-bugs", "oldman"}, st.Skills)

	args, err := os.ReadFile(argFile)
	require.NoError(t, err)
	require.Contains(t, string(args), "skills ls --json")
}

// The store directory exists whether or not anything was imported into it.
func TestList_EmptyStoreStillReportsItsPath(t *testing.T) {
	for _, out := range []string{`{"store":"/var/lib/skills","skills":[]}`, `{"store":"/var/lib/skills","skills":null}`} {
		st, err := List(context.Background(), printingClient(t, filepath.Join(t.TempDir(), "args.txt"), out))
		require.NoError(t, err)
		require.Equal(t, "/var/lib/skills", st.Path)
		require.Empty(t, st.Skills)
	}
}

// Without the store path the output is not what List understands, so it is
// refused rather than read as an empty store.
func TestList_MissingPathIsUnexpectedFormat(t *testing.T) {
	for _, out := range []string{`{"skills":["code-review"]}`, "Skills store: /var/lib/skills\ncode-review"} {
		_, err := List(context.Background(), printingClient(t, filepath.Join(t.TempDir(), "args.txt"), out))
		require.ErrorIs(t, err, client.ErrUnexpectedFormat, out)
	}
}
