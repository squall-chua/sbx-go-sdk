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
