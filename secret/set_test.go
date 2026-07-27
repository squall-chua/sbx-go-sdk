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
// argFile and its stdin to stdinFile, and answers a `secret ls` invocation
// with lsOutput — the fixture SetToken, SetRegistry and Import list before
// invoking the CLI, to check for an existing entry. Pass "" for lsOutput when
// the scope should appear empty.
func stdinRecordingClient(t *testing.T, argFile, stdinFile, lsOutput string) *client.Client {
	t.Helper()
	dir := t.TempDir()
	lsFile := filepath.Join(dir, "ls_output.txt")
	require.NoError(t, os.WriteFile(lsFile, []byte(lsOutput), 0o644))
	bin := filepath.Join(dir, "sbx")
	script := "#!/bin/sh\n" +
		"printf '%s\\n' \"$*\" >> " + argFile + "\n" +
		"cat > " + stdinFile + "\n" +
		"if [ \"$1\" = secret ] && [ \"$2\" = ls ]; then cat " + lsFile + "; fi\n" +
		"exit 0\n"
	require.NoError(t, os.WriteFile(bin, []byte(script), 0o755))
	c, err := client.New(context.Background(), client.WithBinaryPath(bin))
	require.NoError(t, err)
	return c
}

func TestSetToken_GlobalScope(t *testing.T) {
	dir := t.TempDir()
	argFile := filepath.Join(dir, "args.txt")
	c := stdinRecordingClient(t, argFile, filepath.Join(dir, "stdin.txt"), "")

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
	require.Equal(t, "sk-test\n", string(stdin))
}

func TestSetToken_SandboxScopeAndOverwrite(t *testing.T) {
	dir := t.TempDir()
	argFile := filepath.Join(dir, "args.txt")
	c := stdinRecordingClient(t, argFile, filepath.Join(dir, "stdin.txt"), "")

	require.NoError(t, SetToken(context.Background(), c, "my-sandbox", "openai", "sk-x", WithOverwrite()))

	args, err := os.ReadFile(argFile)
	require.NoError(t, err)
	require.Contains(t, string(args), "my-sandbox")
	require.Contains(t, string(args), "--force")
	require.NotContains(t, string(args), "-g")
}

func TestSetToken_RejectsEmptyInput(t *testing.T) {
	dir := t.TempDir()
	c := stdinRecordingClient(t, filepath.Join(dir, "a.txt"), filepath.Join(dir, "s.txt"), "")
	ctx := context.Background()
	require.Error(t, SetToken(ctx, c, "", "", "sk-x"), "empty service must be rejected")
	require.Error(t, SetToken(ctx, c, "", "anthropic", ""), "empty token must be rejected")
}

func TestSetRegistry_PasswordGoesToStdinNotArgv(t *testing.T) {
	dir := t.TempDir()
	argFile := filepath.Join(dir, "args.txt")
	stdinFile := filepath.Join(dir, "stdin.txt")
	c := stdinRecordingClient(t, argFile, stdinFile, "")

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
	require.Equal(t, "ghp_secret\n", string(stdin))
}

func TestSetRegistry_OmitsUsernameWhenEmpty(t *testing.T) {
	dir := t.TempDir()
	argFile := filepath.Join(dir, "args.txt")
	c := stdinRecordingClient(t, argFile, filepath.Join(dir, "stdin.txt"), "")

	require.NoError(t, SetRegistry(context.Background(), c, "", RegistryCredential{
		Host: "ghcr.io", Password: "tok",
	}))

	args, err := os.ReadFile(argFile)
	require.NoError(t, err)
	require.NotContains(t, string(args), "--username")
}

func TestSetRegistry_RejectsEmptyHostOrPassword(t *testing.T) {
	dir := t.TempDir()
	c := stdinRecordingClient(t, filepath.Join(dir, "a.txt"), filepath.Join(dir, "s.txt"), "")
	ctx := context.Background()
	require.Error(t, SetRegistry(ctx, c, "", RegistryCredential{Password: "p"}))
	require.Error(t, SetRegistry(ctx, c, "", RegistryCredential{Host: "ghcr.io"}))
}

const secretLsServiceOpenAI = "SCOPE       TYPE     NAME    SECRET\n" +
	"(global)    service  openai  testte**\n"

const secretLsServiceGithub = "SCOPE       TYPE     NAME    SECRET\n" +
	"(global)    service  github  ghp_te**\n"

const secretLsRegistryGHCR = "SCOPE       TYPE      NAME     SECRET\n" +
	"(global)    registry  ghcr.io  ghp_te**\n"

func TestSetToken_ExistingSecretWithoutOverwriteIsRejected(t *testing.T) {
	dir := t.TempDir()
	argFile := filepath.Join(dir, "args.txt")
	c := stdinRecordingClient(t, argFile, filepath.Join(dir, "stdin.txt"), secretLsServiceOpenAI)

	err := SetToken(context.Background(), c, "", "openai", "sk-test")
	require.Error(t, err)
	require.Contains(t, err.Error(), "WithOverwrite")

	args, err := os.ReadFile(argFile)
	require.NoError(t, err)
	for _, line := range strings.Split(string(args), "\n") {
		require.NotContains(t, line, "secret set",
			"the CLI must never be invoked for secret set when the pre-flight check rejects")
	}
}

func TestSetToken_ExistingSecretWithOverwriteProceeds(t *testing.T) {
	dir := t.TempDir()
	argFile := filepath.Join(dir, "args.txt")
	c := stdinRecordingClient(t, argFile, filepath.Join(dir, "stdin.txt"), secretLsServiceOpenAI)

	require.NoError(t, SetToken(context.Background(), c, "", "openai", "sk-test", WithOverwrite()))

	args, err := os.ReadFile(argFile)
	require.NoError(t, err)
	require.Contains(t, string(args), "secret set")
	require.Contains(t, string(args), "--force")
}

func TestSetToken_DifferentServiceIsNotBlocked(t *testing.T) {
	dir := t.TempDir()
	argFile := filepath.Join(dir, "args.txt")
	c := stdinRecordingClient(t, argFile, filepath.Join(dir, "stdin.txt"), secretLsServiceGithub)

	require.NoError(t, SetToken(context.Background(), c, "", "anthropic", "sk-test"))

	args, err := os.ReadFile(argFile)
	require.NoError(t, err)
	require.Contains(t, string(args), "secret set")
}

func TestSetToken_SameServiceInDifferentScopeIsNotBlocked(t *testing.T) {
	// `sbx secret ls` with no scope lists every scope, not global-only (only
	// `-g` asks the CLI for global-only), so this fixture's "other-sandbox" row
	// must not leak into a global-scope existence check.
	const fixture = "SCOPE           TYPE     NAME    SECRET\n" +
		"other-sandbox   service  openai  testte**\n"
	dir := t.TempDir()
	argFile := filepath.Join(dir, "args.txt")
	c := stdinRecordingClient(t, argFile, filepath.Join(dir, "stdin.txt"), fixture)

	require.NoError(t, SetToken(context.Background(), c, "", "openai", "sk-test"))

	args, err := os.ReadFile(argFile)
	require.NoError(t, err)
	require.Contains(t, string(args), "secret set")
}

func TestSetRegistry_ExistingSecretWithoutOverwriteIsRejected(t *testing.T) {
	dir := t.TempDir()
	argFile := filepath.Join(dir, "args.txt")
	c := stdinRecordingClient(t, argFile, filepath.Join(dir, "stdin.txt"), secretLsRegistryGHCR)

	err := SetRegistry(context.Background(), c, "", RegistryCredential{Host: "ghcr.io", Password: "tok"})
	require.Error(t, err)
	require.Contains(t, err.Error(), "WithOverwrite")

	args, err := os.ReadFile(argFile)
	require.NoError(t, err)
	for _, line := range strings.Split(string(args), "\n") {
		require.NotContains(t, line, "secret set",
			"the CLI must never be invoked for secret set when the pre-flight check rejects")
	}
}

func TestSetRegistry_ExistingSecretWithOverwriteProceeds(t *testing.T) {
	dir := t.TempDir()
	argFile := filepath.Join(dir, "args.txt")
	c := stdinRecordingClient(t, argFile, filepath.Join(dir, "stdin.txt"), secretLsRegistryGHCR)

	require.NoError(t, SetRegistry(context.Background(), c, "", RegistryCredential{
		Host: "ghcr.io", Password: "tok",
	}, WithOverwrite()))

	args, err := os.ReadFile(argFile)
	require.NoError(t, err)
	require.Contains(t, string(args), "secret set")
	require.Contains(t, string(args), "--force")
}
