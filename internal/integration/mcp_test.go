//go:build integration

package integration

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/squall-chua/sbx-go-sdk/client"
	"github.com/squall-chua/sbx-go-sdk/mcp"
	"github.com/squall-chua/sbx-go-sdk/sandbox"
	"github.com/stretchr/testify/require"
)

// Round-trips the mcp package against the live daemon with a throwaway local
// stdio server: nothing leaves the host, and the server is removed at the end.
func TestSmoke_MCPRegisterInspectRemove(t *testing.T) {
	ctx := context.Background()
	c, err := client.New(ctx, client.WithAutoStart())
	require.NoError(t, err)

	const name = "sbx-go-sdk-smoke"
	t.Cleanup(func() { _ = mcp.Remove(ctx, c, name) })

	require.NoError(t, mcp.AddLocal(ctx, c, name, "echo", []string{"hi"}))

	servers, err := mcp.List(ctx, c)
	require.NoError(t, err)
	var found *mcp.Server
	for i := range servers {
		if servers[i].Name == name {
			found = &servers[i]
		}
	}
	require.NotNil(t, found, "the registered server must appear in mcp ls")
	require.Equal(t, "local", found.Type)
	require.Equal(t, "stdio", found.Transport)
	require.NotEmpty(t, found.Status)

	d, err := mcp.Inspect(ctx, c, name)
	require.NoError(t, err)
	require.Equal(t, name, d.Name)
	require.Equal(t, "local", d.Type)
	require.Equal(t, "echo hi", d.Command)
	require.NotEmpty(t, d.Resolved, "a local server reports the resolved executable")
	require.False(t, d.RequiresOAuth)

	// A local stdio server needs no OAuth, so it must not show up in auth status.
	// (Registering an OAuth server would need a network round trip to a real MCP
	// provider, which does not belong in this suite.)
	auth, err := mcp.AuthStatus(ctx, c)
	require.NoError(t, err)
	for _, a := range auth {
		require.NotEqual(t, name, a.ServerName, "a non-OAuth server has no auth state")
	}

	require.NoError(t, mcp.Remove(ctx, c, name))
	after, err := mcp.List(ctx, c)
	require.NoError(t, err)
	for _, s := range after {
		require.NotEqual(t, name, s.Name, "removed server must be gone")
	}
}

// The gateway-mode route is the one piece of MCP the daemon serves over REST.
func TestSmoke_MCPGatewayMode(t *testing.T) {
	ctx := context.Background()
	c, err := client.New(ctx, client.WithAutoStart())
	require.NoError(t, err)

	m, err := c.MCPGatewayMode(ctx)
	require.NoError(t, err)
	require.Contains(t, []string{"local", "saas"}, m.Decision)
	require.NotEmpty(t, m.Reason)
}

// Loads a stub stdio server (testdata/mcp-stub.sh) into a running sandbox and
// unloads it again.
func TestSmoke_MCPLoadUnload(t *testing.T) {
	ctx := context.Background()
	c, err := client.New(ctx, client.WithAutoStart())
	require.NoError(t, err)

	stub, err := filepath.Abs("testdata/mcp-stub.sh")
	require.NoError(t, err)
	const name = "sbx-go-sdk-stub"
	t.Cleanup(func() { _ = mcp.Remove(ctx, c, name) })
	require.NoError(t, mcp.AddLocal(ctx, c, name, "sh", []string{stub}))

	sb, err := sandbox.Create(ctx, c, sandbox.WithAgent("shell"), sandbox.WithWorkspace(t.TempDir()))
	require.NoError(t, err)
	t.Cleanup(func() { sb.Remove(ctx, sandbox.WithForce()) })

	require.NoError(t, mcp.Load(ctx, c, name, sb.Name()))
	require.NoError(t, mcp.Unload(ctx, c, name, sb.Name()))

	err = mcp.Unload(ctx, c, name, sb.Name())
	var cliErr *client.CLIError
	require.True(t, errors.As(err, &cliErr), "a second unload must fail: %v", err)
	require.Contains(t, cliErr.Stderr, "is not loaded")
}

// RemoveRegistration drops the registration; a second call fails, since the
// name is then unregistered.
func TestSmoke_MCPRemoveRegistration(t *testing.T) {
	ctx := context.Background()
	c, err := client.New(ctx, client.WithAutoStart())
	require.NoError(t, err)

	const name = "sbx-go-sdk-regonly"
	t.Cleanup(func() { _ = mcp.Remove(ctx, c, name) })
	require.NoError(t, mcp.AddLocal(ctx, c, name, "echo", []string{"hi"}))

	require.NoError(t, mcp.RemoveRegistration(ctx, c, name))
	servers, err := mcp.List(ctx, c)
	require.NoError(t, err)
	for _, s := range servers {
		require.NotEqual(t, name, s.Name, "the registration must be gone")
	}

	err = mcp.RemoveRegistration(ctx, c, name)
	var cliErr *client.CLIError
	require.True(t, errors.As(err, &cliErr), "%v", err)
	require.Contains(t, cliErr.Stderr, "not found")
}
