//go:build integration

package integration

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/squall-chua/sbx-go-sdk/client"
	"github.com/squall-chua/sbx-go-sdk/settings"
	"github.com/stretchr/testify/require"
)

// TestContract_SettingsListGet is a read-only drift detector for the `sbx settings
// … --json` shape. It asserts structure, not exact default values (which drift
// across sbx versions). It never mutates settings.
func TestContract_SettingsListGet(t *testing.T) {
	ctx := context.Background()
	c, err := client.New(ctx)
	require.NoError(t, err)

	all, err := settings.List(ctx, c)
	require.NoError(t, err)
	require.NotEmpty(t, all)
	for _, s := range all {
		require.NotEmpty(t, s.Key)
		require.NotEmpty(t, s.Type)
		require.NotEmpty(t, s.Source)
	}

	// ssh.autoCreate is a known bool-typed setting; assert structure, not the value.
	p, err := settings.Get(ctx, c, "ssh.autoCreate")
	require.NoError(t, err)
	require.Equal(t, "ssh.autoCreate", p.Key)
	var autoCreate bool
	require.NoError(t, json.Unmarshal(p.Value, &autoCreate))
}
