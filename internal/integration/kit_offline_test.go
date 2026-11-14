//go:build integration

package integration

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/squall-chua/sbx-go-sdk/client"
	"github.com/squall-chua/sbx-go-sdk/kit"
	"github.com/stretchr/testify/require"
)

// With the daemon up, a v2 kit loads the same with or without --offline, so
// only a v3 source kit tells the flag apart: it loads normally but is refused
// offline, since that load path has no kit builder. Neither call creates the
// builder sandbox.
func TestSmoke_KitOffline(t *testing.T) {
	ctx := context.Background()
	c, err := client.New(ctx, client.WithAutoStart())
	require.NoError(t, err)

	v3 := fixtureKitV3(t)
	_, err = kit.InspectWith(ctx, c, v3)
	require.NoError(t, err)

	_, err = kit.InspectWith(ctx, c, v3, kit.WithOffline())
	require.ErrorContains(t, err, "no kit builder configured")

	err = kit.ValidateWith(ctx, c, v3, kit.WithOffline())
	require.ErrorIs(t, err, client.ErrKitRejected)
	require.ErrorContains(t, err, "no kit builder configured")

	// A v2 kit has no build step, so it loads offline.
	v2 := fixtureKit(t)
	require.NoError(t, kit.ValidateWith(ctx, c, v2, kit.WithOffline()))
	info, err := kit.InspectWith(ctx, c, v2, kit.WithOffline(), kit.WithResolved())
	require.NoError(t, err)
	require.Equal(t, "fixture-kit", info.Name)
}

// gemini is one of the few agents still embedded at sbx v0.51.0, so it is a
// parent --resolved can merge without network. claude and shell are refused
// ("no longer a built-in agent").
func TestSmoke_KitResolved(t *testing.T) {
	ctx := context.Background()
	c, err := client.New(ctx, client.WithAutoStart())
	require.NoError(t, err)

	dir := filepath.Join(t.TempDir(), "child")
	require.NoError(t, os.MkdirAll(dir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "spec.yaml"), []byte(
		"schemaVersion: \"2\"\nkind: sandbox\nname: child\nversion: 0.1.0\nextends: gemini\n"+
			"environment:\n  variables:\n    FROM_CHILD: \"1\"\n"), 0o644))

	plain, err := kit.InspectWith(ctx, c, dir)
	require.NoError(t, err)
	require.Equal(t, "gemini", plain.Extends)
	require.Empty(t, plain.Sandbox, "unresolved, the child declares no sandbox block")

	res, err := kit.InspectWith(ctx, c, dir, kit.WithResolved())
	require.NoError(t, err)
	require.Empty(t, res.Extends, "resolving consumes extends:")
	require.Contains(t, string(res.Sandbox), "gemini", "the parent's sandbox block is merged in")
	require.Contains(t, string(res.Environment), "FROM_CHILD", "the child's own keys survive")
}
