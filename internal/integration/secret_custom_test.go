//go:build integration

package integration

import (
	"context"
	"testing"

	"github.com/squall-chua/sbx-go-sdk/client"
	"github.com/squall-chua/sbx-go-sdk/secret"
	"github.com/stretchr/testify/require"
)

// SetCustom sends a literal value on stdin (--value-stdin). The masked value
// in `secret ls` has the stored value's length and first six characters, so
// it shows the value arrived intact: the trailing spaces that --value trimmed
// are kept. A second write that repeats the placeholder rotates the value in
// place.
func TestSmoke_SecretCustomValueStdin(t *testing.T) {
	ctx := context.Background()
	c, err := client.New(ctx, client.WithAutoStart())
	require.NoError(t, err)

	const host, env = "custom-stdin.probe.invalid", "SBX_SDK_STDIN_PROBE"
	cleanup := func() { _ = secret.RemoveCustom(ctx, c, "", host) }
	cleanup()
	t.Cleanup(cleanup)

	find := func() *secret.Custom {
		t.Helper()
		list, err := secret.List(ctx, c, "")
		require.NoError(t, err)
		for i := range list.Custom {
			if list.Custom[i].Env == env {
				return &list.Custom[i]
			}
		}
		return nil
	}

	const value = "sdkval-made-up  " // 16 bytes; made up for this test
	require.NoError(t, secret.SetCustom(ctx, c, "", secret.CustomSecret{Host: host, Env: env, Value: value}))
	got := find()
	require.NotNil(t, got)
	require.Equal(t, "sdkval**********", got.ValueMasked, "length and prefix must match the value sent")

	require.NoError(t, secret.SetCustom(ctx, c, "",
		secret.CustomSecret{Host: host, Env: env, Value: "sdkrot-2", Placeholder: got.Placeholder}))
	rotated := find()
	require.NotNil(t, rotated)
	require.Equal(t, got.Placeholder, rotated.Placeholder)
	require.Equal(t, "sdkrot**", rotated.ValueMasked)
}
