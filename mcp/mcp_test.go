package mcp

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/squall-chua/sbx-go-sdk/client"
	"github.com/stretchr/testify/require"
)

// stubClient runs a fake sbx that records its argument vector into argFile and
// prints out on stdout.
func stubClient(t *testing.T, argFile, out string) *client.Client {
	t.Helper()
	bin := filepath.Join(t.TempDir(), "sbx")
	script := "#!/bin/sh\nprintf '%s\\n' \"$*\" >> " + argFile + "\ncat <<'SBXEOF'\n" + out + "\nSBXEOF\nexit 0\n"
	require.NoError(t, os.WriteFile(bin, []byte(script), 0o755))
	c, err := client.New(context.Background(), client.WithBinaryPath(bin))
	require.NoError(t, err)
	return c
}

func recordedArgs(t *testing.T, argFile string) string {
	t.Helper()
	b, err := os.ReadFile(argFile)
	require.NoError(t, err)
	return string(b)
}

// Captured verbatim from `sbx mcp ls` at sbx v0.38.0 with one local and one
// remote server registered.
const lsOutput = `NAME                 TYPE     URL/COMMAND
sdkprobe             local    echo hi
sdkprobe2            remote   https://mcp.deepwiki.com/mcp`

func TestList(t *testing.T) {
	argFile := filepath.Join(t.TempDir(), "args.txt")
	c := stubClient(t, argFile, lsOutput)

	got, err := List(context.Background(), c)
	require.NoError(t, err)
	require.Equal(t, []Server{
		{Name: "sdkprobe", Type: "local", Target: "echo hi"},
		{Name: "sdkprobe2", Type: "remote", Target: "https://mcp.deepwiki.com/mcp"},
	}, got)
	require.Contains(t, recordedArgs(t, argFile), "mcp ls")
}

// With nothing registered the CLI prints prose, not a table.
func TestList_EmptyIsNotAnError(t *testing.T) {
	argFile := filepath.Join(t.TempDir(), "args.txt")
	c := stubClient(t, argFile, "No MCP servers registered")

	got, err := List(context.Background(), c)
	require.NoError(t, err)
	require.Empty(t, got)
}

// Captured verbatim from `sbx mcp inspect` at sbx v0.38.0. A remote server's
// URL contains a colon, which must not confuse the label split.
func TestInspect_Remote(t *testing.T) {
	argFile := filepath.Join(t.TempDir(), "args.txt")
	c := stubClient(t, argFile, "Name:      sdkprobe2\nType:      remote\nURL:       https://mcp.deepwiki.com/mcp\nTransport: streamable-http")

	d, err := Inspect(context.Background(), c, "sdkprobe2")
	require.NoError(t, err)
	require.Equal(t, "sdkprobe2", d.Name)
	require.Equal(t, "remote", d.Type)
	require.Equal(t, "https://mcp.deepwiki.com/mcp", d.URL)
	require.Equal(t, "streamable-http", d.Transport)
	require.False(t, d.RequiresOAuth)
	require.Contains(t, recordedArgs(t, argFile), "mcp inspect sdkprobe2")
}

func TestInspect_Local(t *testing.T) {
	argFile := filepath.Join(t.TempDir(), "args.txt")
	c := stubClient(t, argFile, "Name:      sdkprobe\nType:      local\nCommand:   echo hi\nResolved:  /usr/bin/echo")

	d, err := Inspect(context.Background(), c, "sdkprobe")
	require.NoError(t, err)
	require.Equal(t, "echo hi", d.Command)
	require.Equal(t, "/usr/bin/echo", d.Resolved)
	require.Equal(t, "local", d.Fields["Type"])
}

func TestInspect_OAuthRequired(t *testing.T) {
	argFile := filepath.Join(t.TempDir(), "args.txt")
	c := stubClient(t, argFile, "Name:      notion\nType:      remote\nOAuth:     required")

	d, err := Inspect(context.Background(), c, "notion")
	require.NoError(t, err)
	require.True(t, d.RequiresOAuth)
}

func TestAddRemote_Flags(t *testing.T) {
	argFile := filepath.Join(t.TempDir(), "args.txt")
	c := stubClient(t, argFile, "")

	require.NoError(t, AddRemote(context.Background(), c, "acme", "https://mcp.acme.com/mcp",
		WithClientID("my-client"),
		WithOAuthAuthorizationServer("./acme-as.json"),
		WithScopes("read", "write"),
		WithSkipAuth(),
		WithSkipSSRFCheck()))

	args := recordedArgs(t, argFile)
	require.Contains(t, args, "mcp add acme --url https://mcp.acme.com/mcp")
	require.Contains(t, args, "--client-id my-client")
	require.Contains(t, args, "--oauth-authorization-server ./acme-as.json")
	require.Contains(t, args, "--scope read --scope write")
	require.Contains(t, args, "--skip_auth")
	require.Contains(t, args, "--skip-ssrf-check")
}

// The CLI takes --args as one comma-separated list, per its own help examples.
func TestAddLocal_JoinsArgsWithCommas(t *testing.T) {
	argFile := filepath.Join(t.TempDir(), "args.txt")
	c := stubClient(t, argFile, "")

	require.NoError(t, AddLocal(context.Background(), c, "postgres", "docker",
		[]string{"run", "-i", "--rm", "mcp/postgres"}, WithWorkingDir("/srv/data")))

	args := recordedArgs(t, argFile)
	require.Contains(t, args, "mcp add postgres --command docker")
	require.Contains(t, args, "--args run,-i,--rm,mcp/postgres")
	require.Contains(t, args, "--dir /srv/data")
}

func TestRemoveAndLoad(t *testing.T) {
	argFile := filepath.Join(t.TempDir(), "args.txt")
	c := stubClient(t, argFile, "")

	require.NoError(t, Remove(context.Background(), c, "notion"))
	require.NoError(t, Load(context.Background(), c, "notion", "my-sbx"))

	args := recordedArgs(t, argFile)
	require.Contains(t, args, "mcp rm notion")
	require.Contains(t, args, "mcp load notion --sandbox my-sbx")
}

// Captured verbatim at sbx v0.38.0 from a server registered with --skip_auth.
func TestAuthStatus(t *testing.T) {
	argFile := filepath.Join(t.TempDir(), "args.txt")
	c := stubClient(t, argFile, `[{"server_name":"sdkoauth","status":"unauthorized"}]`)

	got, err := AuthStatus(context.Background(), c)
	require.NoError(t, err)
	require.Equal(t, []AuthResult{{ServerName: "sdkoauth", Status: "unauthorized"}}, got)
	require.False(t, got[0].Authorized())
	require.Contains(t, recordedArgs(t, argFile), "mcp auth status --all --format json")
}

// Hosts with only non-OAuth servers get an empty array, not an error.
func TestAuthStatus_EmptyIsNotAnError(t *testing.T) {
	argFile := filepath.Join(t.TempDir(), "args.txt")
	c := stubClient(t, argFile, "[]")

	got, err := AuthStatus(context.Background(), c)
	require.NoError(t, err)
	require.Empty(t, got)
}

func TestAuthStatus_RejectsNonJSON(t *testing.T) {
	argFile := filepath.Join(t.TempDir(), "args.txt")
	c := stubClient(t, argFile, "No registered MCP servers require OAuth")

	_, err := AuthStatus(context.Background(), c)
	require.ErrorIs(t, err, client.ErrUnexpectedFormat)
}

func TestAuthRemove(t *testing.T) {
	argFile := filepath.Join(t.TempDir(), "args.txt")
	c := stubClient(t, argFile, `[{"server_name":"notion","status":"unauthorized"}]`)

	got, err := AuthRemove(context.Background(), c, "notion")
	require.NoError(t, err)
	require.Len(t, got, 1)
	require.Equal(t, "notion", got[0].ServerName)
	require.Contains(t, recordedArgs(t, argFile), "mcp auth rm notion --format json")
}

func TestEmptyNamesRejectedBeforeShellOut(t *testing.T) {
	argFile := filepath.Join(t.TempDir(), "args.txt")
	c := stubClient(t, argFile, "")
	ctx := context.Background()

	require.Error(t, AddRemote(ctx, c, "", "https://x/mcp"))
	require.Error(t, AddRemote(ctx, c, "x", ""))
	require.Error(t, AddLocal(ctx, c, "", "echo", nil))
	require.Error(t, AddLocal(ctx, c, "x", "", nil))
	require.Error(t, Remove(ctx, c, ""))
	require.Error(t, Load(ctx, c, "x", ""))
	_, err := AuthRemove(ctx, c, "")
	require.Error(t, err)
	_, err = Inspect(ctx, c, "")
	require.Error(t, err)

	require.NoFileExists(t, argFile, "no call should have reached the CLI")
}
