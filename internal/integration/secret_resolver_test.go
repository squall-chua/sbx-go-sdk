//go:build integration

package integration

import (
	"context"
	"strings"
	"testing"

	"github.com/squall-chua/sbx-go-sdk/client"
	"github.com/squall-chua/sbx-go-sdk/secret"
	"github.com/stretchr/testify/require"
)

// The --command resolver is the only one verifiable here: --ref needs a
// 1Password `op` binary or AWS credentials, neither of which this host has.
// A custom secret is used rather than a service one so the target host can be
// a name that resolves nowhere.
func TestSmoke_SecretCommandResolver(t *testing.T) {
	ctx := context.Background()
	c, err := client.New(ctx, client.WithAutoStart())
	require.NoError(t, err)

	const env = "SBX_SDK_RESOLVER_PROBE"
	// probe.invalid exists only for this test, so removing every custom secret
	// on it cannot touch a real one.
	cleanup := func() { _ = secret.RemoveCustom(ctx, c, "", "probe.invalid") }
	cleanup()
	t.Cleanup(cleanup)

	// The command's output must not appear anywhere in its own text, or the
	// assertion below cannot tell the two apart. "expr 21 * 2" prints 42.
	require.NoError(t, secret.SetCustom(ctx, c, "",
		secret.CustomSecret{Host: "probe.invalid", Env: env},
		secret.FromCommand("expr 21 \\* 2"),
		secret.WithRefresh("30m"),
	))

	list, err := secret.List(ctx, c, "")
	require.NoError(t, err)

	var found *secret.Custom
	for i := range list.Custom {
		if list.Custom[i].Env == env {
			found = &list.Custom[i]
		}
	}
	require.NotNil(t, found, "the resolver-backed secret must list")

	// The SECRET column reports the source and its refresh policy, not a mask,
	// because there is no stored value to mask.
	require.Contains(t, found.ValueMasked, "command:")
	require.Contains(t, found.ValueMasked, "30m")

	// What the resolver *returns* is never stored or displayed. What it is
	// *spelled as* is displayed in full, which is why FromCommand's doc says
	// never to embed a secret in the command text.
	require.NotContains(t, found.ValueMasked, "42",
		"the resolved value is never stored or displayed")
	require.Contains(t, found.ValueMasked, "expr",
		"the command text itself is displayed verbatim")
}

// A resolver that cannot succeed is refused at store time, so a broken secret
// is never written. That check is exactly what WithoutVerify turns off.
func TestSmoke_SecretResolverIsVerifiedAtStoreTime(t *testing.T) {
	ctx := context.Background()
	c, err := client.New(ctx, client.WithAutoStart())
	require.NoError(t, err)

	err = secret.SetCustom(ctx, c, "",
		secret.CustomSecret{Host: "probe.invalid", Env: "SBX_SDK_BROKEN_PROBE"},
		secret.FromCommand("exit 3"),
	)
	require.Error(t, err)
	require.True(t, strings.Contains(err.Error(), "verify command failed"),
		"the CLI reports the failed check verbatim: %v", err)

	list, err := secret.List(ctx, c, "")
	require.NoError(t, err)
	for _, cs := range list.Custom {
		require.NotEqual(t, "SBX_SDK_BROKEN_PROBE", cs.Env, "a refused resolver stores nothing")
	}
}
