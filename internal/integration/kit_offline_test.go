//go:build integration

package integration

import (
	"context"
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
