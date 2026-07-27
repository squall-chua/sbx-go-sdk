package secret

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/squall-chua/sbx-go-sdk/client"
	"github.com/stretchr/testify/require"
)

// stdinRecordingClient returns a client whose fake sbx records its args to
// argFile and its stdin to stdinFile.
func stdinRecordingClient(t *testing.T, argFile, stdinFile string) *client.Client {
	t.Helper()
	bin := filepath.Join(t.TempDir(), "sbx")
	script := "#!/bin/sh\n" +
		"printf '%s\\n' \"$*\" >> " + argFile + "\n" +
		"cat > " + stdinFile + "\n" +
		"exit 0\n"
	require.NoError(t, os.WriteFile(bin, []byte(script), 0o755))
	c, err := client.New(context.Background(), client.WithBinaryPath(bin))
	require.NoError(t, err)
	return c
}

func TestSetToken_GlobalScope(t *testing.T) {
	dir := t.TempDir()
	argFile := filepath.Join(dir, "args.txt")
	c := stdinRecordingClient(t, argFile, filepath.Join(dir, "stdin.txt"))

	require.NoError(t, SetToken(context.Background(), c, "", "anthropic", "sk-test"))

	args, err := os.ReadFile(argFile)
	require.NoError(t, err)
	require.Contains(t, string(args), "secret set")
	require.Contains(t, string(args), "-g")
	require.Contains(t, string(args), "anthropic")
	require.NotContains(t, string(args), "sk-test",
		"the token must never appear in the argument vector")

	stdin, err := os.ReadFile(filepath.Join(dir, "stdin.txt"))
	require.NoError(t, err)
	require.Equal(t, "sk-test", strings.TrimRight(string(stdin), "\n"))
}

func TestSetToken_SandboxScopeAndOverwrite(t *testing.T) {
	dir := t.TempDir()
	argFile := filepath.Join(dir, "args.txt")
	c := stdinRecordingClient(t, argFile, filepath.Join(dir, "stdin.txt"))

	require.NoError(t, SetToken(context.Background(), c, "my-sandbox", "openai", "sk-x", WithOverwrite()))

	args, err := os.ReadFile(argFile)
	require.NoError(t, err)
	require.Contains(t, string(args), "my-sandbox")
	require.Contains(t, string(args), "--force")
	require.NotContains(t, string(args), "-g")
}

func TestSetToken_RejectsEmptyInput(t *testing.T) {
	dir := t.TempDir()
	c := stdinRecordingClient(t, filepath.Join(dir, "a.txt"), filepath.Join(dir, "s.txt"))
	ctx := context.Background()
	require.Error(t, SetToken(ctx, c, "", "", "sk-x"), "empty service must be rejected")
	require.Error(t, SetToken(ctx, c, "", "anthropic", ""), "empty token must be rejected")
}

func TestSetRegistry_PasswordGoesToStdinNotArgv(t *testing.T) {
	dir := t.TempDir()
	argFile := filepath.Join(dir, "args.txt")
	stdinFile := filepath.Join(dir, "stdin.txt")
	c := stdinRecordingClient(t, argFile, stdinFile)

	err := SetRegistry(context.Background(), c, "", RegistryCredential{
		Host: "ghcr.io", Username: "me", Password: "ghp_secret",
	})
	require.NoError(t, err)

	args, err := os.ReadFile(argFile)
	require.NoError(t, err)
	require.Contains(t, string(args), "--registry ghcr.io")
	require.Contains(t, string(args), "--password-stdin")
	require.Contains(t, string(args), "--username me")
	require.NotContains(t, string(args), "ghp_secret",
		"the password must never appear in the argument vector")

	stdin, err := os.ReadFile(stdinFile)
	require.NoError(t, err)
	require.Equal(t, "ghp_secret", strings.TrimRight(string(stdin), "\n"))
}

func TestSetRegistry_OmitsUsernameWhenEmpty(t *testing.T) {
	dir := t.TempDir()
	argFile := filepath.Join(dir, "args.txt")
	c := stdinRecordingClient(t, argFile, filepath.Join(dir, "stdin.txt"))

	require.NoError(t, SetRegistry(context.Background(), c, "", RegistryCredential{
		Host: "ghcr.io", Password: "tok",
	}))

	args, err := os.ReadFile(argFile)
	require.NoError(t, err)
	require.NotContains(t, string(args), "--username")
}

func TestSetRegistry_RejectsEmptyHostOrPassword(t *testing.T) {
	dir := t.TempDir()
	c := stdinRecordingClient(t, filepath.Join(dir, "a.txt"), filepath.Join(dir, "s.txt"))
	ctx := context.Background()
	require.Error(t, SetRegistry(ctx, c, "", RegistryCredential{Password: "p"}))
	require.Error(t, SetRegistry(ctx, c, "", RegistryCredential{Host: "ghcr.io"}))
}
