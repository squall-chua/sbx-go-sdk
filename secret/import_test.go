package secret

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestImport_NamedServiceOmitsAllFlag(t *testing.T) {
	dir := t.TempDir()
	argFile := filepath.Join(dir, "args.txt")
	c := stdinRecordingClient(t, argFile, filepath.Join(dir, "stdin.txt"))

	require.NoError(t, Import(context.Background(), c, "openai"))

	args, err := os.ReadFile(argFile)
	require.NoError(t, err)
	require.Contains(t, string(args), "secret import")
	require.Contains(t, string(args), "openai")
	require.NotContains(t, string(args), "--all",
		"a named-service import must not sweep every detected variable")
}

func TestImport_DryRun(t *testing.T) {
	dir := t.TempDir()
	argFile := filepath.Join(dir, "args.txt")
	c := stdinRecordingClient(t, argFile, filepath.Join(dir, "stdin.txt"))

	require.NoError(t, Import(context.Background(), c, "openai", WithDryRun()))

	args, err := os.ReadFile(argFile)
	require.NoError(t, err)
	require.Contains(t, string(args), "--dry-run")
}

func TestImport_EmptyServiceIsRejected(t *testing.T) {
	dir := t.TempDir()
	c := stdinRecordingClient(t, filepath.Join(dir, "a.txt"), filepath.Join(dir, "s.txt"))
	require.Error(t, Import(context.Background(), c, ""),
		"use ImportAll to import every detected variable")
}

func TestImportAll_PassesAllFlag(t *testing.T) {
	dir := t.TempDir()
	argFile := filepath.Join(dir, "args.txt")
	c := stdinRecordingClient(t, argFile, filepath.Join(dir, "stdin.txt"))

	require.NoError(t, ImportAll(context.Background(), c))

	args, err := os.ReadFile(argFile)
	require.NoError(t, err)
	require.Contains(t, string(args), "secret import")
	require.Contains(t, string(args), "--all")
}

func TestImportAll_DryRunAndOverwrite(t *testing.T) {
	dir := t.TempDir()
	argFile := filepath.Join(dir, "args.txt")
	c := stdinRecordingClient(t, argFile, filepath.Join(dir, "stdin.txt"))

	require.NoError(t, ImportAll(context.Background(), c, WithDryRun(), WithOverwriteExisting()))

	args, err := os.ReadFile(argFile)
	require.NoError(t, err)
	require.Contains(t, string(args), "--dry-run")
	require.Contains(t, string(args), "--force")
	require.Contains(t, string(args), "--all")
}
