//go:build integration

package integration

import (
	"context"
	"strings"
	"testing"

	"github.com/squall-chua/sbx-go-sdk/client"
	"github.com/squall-chua/sbx-go-sdk/kit"
	"github.com/stretchr/testify/require"
)

// Creates the shared sbx-kit-builder sandbox through a history call and
// removes it again. Skips when a builder already exists, so it never throws
// away a build cache it did not make.
func TestSmoke_KitBuilderLifecycle(t *testing.T) {
	ctx := context.Background()
	c, err := client.New(ctx, client.WithAutoStart())
	require.NoError(t, err)

	const absent = "not created"
	st, err := kit.BuilderStatusRaw(ctx, c)
	require.NoError(t, err)
	if !strings.Contains(st, absent) {
		t.Skip("a kit builder already exists; not removing someone else's build cache")
	}
	t.Cleanup(func() { _ = kit.RemoveBuilder(context.Background(), c) })

	out, err := kit.BuilderHistoryRaw(ctx, c, "ls")
	require.NoError(t, err)
	require.Contains(t, out, "BUILD ID", "buildx history ls prints its table header")

	st, err = kit.BuilderStatusRaw(ctx, c)
	require.NoError(t, err)
	require.Contains(t, st, "sbx-kit-builder (running)", "a history call creates the builder")

	require.NoError(t, kit.RemoveBuilder(ctx, c))
	st, err = kit.BuilderStatusRaw(ctx, c)
	require.NoError(t, err)
	require.Contains(t, st, absent)
}
