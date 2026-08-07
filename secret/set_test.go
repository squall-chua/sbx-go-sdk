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
	// Global is the CLI default as of sbx v0.38.0, so no scope argument at all.
	require.Contains(t, string(args), "secret set anthropic")
	require.NotContains(t, string(args), "-g")
	require.NotContains(t, string(args), "--sandbox")
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
	require.Contains(t, string(args), "--sandbox my-sandbox")
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

// A bare `secret set --registry` stores a host-only entry as of sbx v0.38.0,
// where it used to mean global. Global scope must therefore spell itself
// --all-sandboxes so the SDK keeps storing an entry injected into every sandbox.
func TestSetRegistry_GlobalScopeMeansAllSandboxes(t *testing.T) {
	dir := t.TempDir()
	argFile := filepath.Join(dir, "args.txt")
	c := stdinRecordingClient(t, argFile, filepath.Join(dir, "stdin.txt"), "")

	require.NoError(t, SetRegistry(context.Background(), c, "", RegistryCredential{
		Host: "ghcr.io", Password: "tok",
	}))

	args, err := os.ReadFile(argFile)
	require.NoError(t, err)
	require.Contains(t, string(args), "--all-sandboxes")
}

func TestSetRegistry_HostOnlyOmitsAllSandboxes(t *testing.T) {
	dir := t.TempDir()
	argFile := filepath.Join(dir, "args.txt")
	c := stdinRecordingClient(t, argFile, filepath.Join(dir, "stdin.txt"), "")

	require.NoError(t, SetRegistry(context.Background(), c, "", RegistryCredential{
		Host: "ghcr.io", Password: "tok",
	}, WithHostOnly()))

	args, err := os.ReadFile(argFile)
	require.NoError(t, err)
	require.NotContains(t, string(args), "--all-sandboxes")
	require.NotContains(t, string(args), "--sandbox")
}

// A sandbox scope wins over WithHostOnly: the two are mutually exclusive in the
// CLI, and an explicit sandbox is the more specific request.
func TestSetRegistry_SandboxScopeIgnoresHostOnly(t *testing.T) {
	dir := t.TempDir()
	argFile := filepath.Join(dir, "args.txt")
	c := stdinRecordingClient(t, argFile, filepath.Join(dir, "stdin.txt"), "")

	require.NoError(t, SetRegistry(context.Background(), c, "my-sandbox", RegistryCredential{
		Host: "ghcr.io", Password: "tok",
	}, WithHostOnly()))

	args, err := os.ReadFile(argFile)
	require.NoError(t, err)
	require.Contains(t, string(args), "--sandbox my-sandbox")
	require.NotContains(t, string(args), "--all-sandboxes")
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

func TestSetToken_ExistingSecretInSameSandboxScopeIsRejected(t *testing.T) {
	// The blocking tests above all use global scope; this pins the direction
	// where a miss would re-open the original bug: a sandbox-scoped row must
	// block a sandbox-scoped SetToken in that same scope. It also pins that
	// List passes the scope through to `ls` — the fake's recorded args are
	// the only thing distinguishing "ls of my-sandbox" from "ls of everything".
	const fixture = "SCOPE        TYPE     NAME    SECRET\n" +
		"my-sandbox   service  openai  testte**\n"
	dir := t.TempDir()
	argFile := filepath.Join(dir, "args.txt")
	c := stdinRecordingClient(t, argFile, filepath.Join(dir, "stdin.txt"), fixture)

	err := SetToken(context.Background(), c, "my-sandbox", "openai", "sk-test")
	require.Error(t, err)
	require.Contains(t, err.Error(), "WithOverwrite")

	args, err := os.ReadFile(argFile)
	require.NoError(t, err)
	require.Contains(t, string(args), "secret ls --sandbox my-sandbox",
		"List must pass the sandbox scope through to the ls lookup, not just query the default")
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
