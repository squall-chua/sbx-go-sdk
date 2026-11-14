package secret

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// A resolver replaces the literal, so --value must be gone and the source
// flags must be there.
func TestSetCustom_FromCommandReplacesTheValueFlag(t *testing.T) {
	argFile := filepath.Join(t.TempDir(), "args.txt")
	c := recordingClient(t, argFile)

	require.NoError(t, SetCustom(context.Background(), c, "",
		CustomSecret{Host: "api.example.com", Env: "API_KEY"},
		FromCommand("vault read -field=key secret/ai"),
		WithRefresh("30m"),
	))

	args, _ := os.ReadFile(argFile)
	require.Contains(t, string(args), "--command vault read -field=key secret/ai")
	require.Contains(t, string(args), "--refresh 30m")
	require.NotContains(t, string(args), "--value",
		"a resolver has no literal to pass")
}

func TestSetToken_FromRefSkipsTheStdinPath(t *testing.T) {
	argFile := filepath.Join(t.TempDir(), "args.txt")
	c := recordingClient(t, argFile)

	require.NoError(t, SetToken(context.Background(), c, "", "anthropic", "",
		FromRef("op://vault/item/field"),
		WithoutVerify(),
		WithResolverErrors(),
	))

	args, _ := os.ReadFile(argFile)
	require.Contains(t, string(args), "--ref op://vault/item/field")
	require.Contains(t, string(args), "--no-verify")
	require.Contains(t, string(args), "--show-error")
}

// The value source is exactly one of three things, so every other combination
// must be refused before the CLI is invoked.
func TestSetSourceValidation(t *testing.T) {
	argFile := filepath.Join(t.TempDir(), "args.txt")
	c := recordingClient(t, argFile)
	ctx := context.Background()

	for name, call := range map[string]func() error{
		"token: no value at all": func() error {
			return SetToken(ctx, c, "", "anthropic", "")
		},
		"token: literal and resolver": func() error {
			return SetToken(ctx, c, "", "anthropic", "sk-1", FromRef("op://v/i/f"))
		},
		"token: two resolvers": func() error {
			return SetToken(ctx, c, "", "anthropic", "", FromRef("op://v/i/f"), FromCommand("echo x"))
		},
		"custom: no value at all": func() error {
			return SetCustom(ctx, c, "", CustomSecret{Host: "h", Env: "E"})
		},
		"custom: literal and resolver": func() error {
			return SetCustom(ctx, c, "", CustomSecret{Host: "h", Env: "E", Value: "v"}, FromCommand("echo x"))
		},
	} {
		require.Error(t, call(), name)
	}

	// Nothing above should have reached the CLI.
	if data, err := os.ReadFile(argFile); err == nil {
		require.NotContains(t, string(data), "set-custom", "validation runs before the shell-out")
	}
}

// Refresh only means something with a resolver behind it.
func TestWithRefresh_IgnoredForALiteral(t *testing.T) {
	argFile := filepath.Join(t.TempDir(), "args.txt")
	c := recordingClient(t, argFile)

	require.NoError(t, SetCustom(context.Background(), c, "",
		CustomSecret{Host: "h", Env: "E", Value: "sk-1"}, WithRefresh("30m")))

	args, _ := os.ReadFile(argFile)
	require.Contains(t, string(args), "--value-stdin")
	require.NotContains(t, string(args), "--refresh")
}

// SetCustom's literal value goes to stdin exactly, with no newline: the CLI
// keeps trailing spaces read from stdin, which --value trimmed.
func TestSetCustom_ValueGoesToStdinVerbatim(t *testing.T) {
	dir := t.TempDir()
	argFile, stdinFile := filepath.Join(dir, "args.txt"), filepath.Join(dir, "stdin.txt")
	c := stdinRecordingClient(t, argFile, stdinFile, "")

	require.NoError(t, SetCustom(context.Background(), c, "sb",
		CustomSecret{Host: "api.example.com", Env: "API_KEY", Value: "sk-123  ", Placeholder: "ph"}))

	args, err := os.ReadFile(argFile)
	require.NoError(t, err)
	require.Equal(t, "secret set-custom --sandbox sb --host api.example.com --env API_KEY --value-stdin --placeholder ph\n", string(args))
	stdin, err := os.ReadFile(stdinFile)
	require.NoError(t, err)
	require.Equal(t, "sk-123  ", string(stdin))
}

// A resolver has no literal, so nothing is written to stdin.
func TestSetCustom_ResolverSendsNoStdin(t *testing.T) {
	dir := t.TempDir()
	argFile, stdinFile := filepath.Join(dir, "args.txt"), filepath.Join(dir, "stdin.txt")
	c := stdinRecordingClient(t, argFile, stdinFile, "")

	require.NoError(t, SetCustom(context.Background(), c, "",
		CustomSecret{Host: "api.example.com", Env: "API_KEY"}, FromRef("op://v/i/f")))
	stdin, err := os.ReadFile(stdinFile)
	require.NoError(t, err)
	require.Empty(t, stdin)
}

// The literal path must keep writing the secret to stdin rather than argv.
func TestSetToken_LiteralStillNeverEntersTheArgumentVector(t *testing.T) {
	argFile := filepath.Join(t.TempDir(), "args.txt")
	c := recordingClient(t, argFile)

	require.NoError(t, SetToken(context.Background(), c, "", "anthropic", "sk-super-secret"))

	args, _ := os.ReadFile(argFile)
	require.False(t, strings.Contains(string(args), "sk-super-secret"),
		"the token must reach the child on stdin, never in argv")
}
