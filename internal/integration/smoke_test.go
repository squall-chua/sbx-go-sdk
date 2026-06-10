//go:build integration

package integration

import (
	"context"
	"io"
	"testing"

	"github.com/squall-chua/sbx-go-sdk/client"
	"github.com/squall-chua/sbx-go-sdk/exec"
	"github.com/squall-chua/sbx-go-sdk/sandbox"
	"github.com/stretchr/testify/require"
)

// Requires a real sbx on PATH and a reachable daemon. Creates and removes a sandbox.
func TestSmoke_CreateExecRemove(t *testing.T) {
	ctx := context.Background()
	c, err := client.New(ctx, client.WithAutoStart())
	require.NoError(t, err)

	sb, err := sandbox.Create(ctx, c, sandbox.WithAgent("shell"), sandbox.WithWorkspace(t.TempDir()))
	require.NoError(t, err)
	t.Cleanup(func() { sb.Remove(ctx) })

	code, r, err := exec.Exec(ctx, sb, []string{"echo", "sdk-smoke"})
	require.NoError(t, err)
	out, _ := io.ReadAll(r)
	require.Equal(t, 0, code)
	require.Contains(t, string(out), "sdk-smoke")
}
