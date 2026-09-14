//go:build integration

package integration

import (
	"context"
	"testing"

	"github.com/squall-chua/sbx-go-sdk/client"
	"github.com/squall-chua/sbx-go-sdk/policy"
	"github.com/squall-chua/sbx-go-sdk/secret"
	"github.com/stretchr/testify/require"
)

// TestContract_ListFormat guards the output shapes that policy.List and
// secret.List decode (secret.List reads `secret ls --json` since sbx v0.42.0).
// If the CLI renames a column or a JSON key, strict validation surfaces
// client.ErrUnexpectedFormat and this test fails so a maintainer can re-sync
// the parser — the format sibling of TestContract_VersionAlignment.
func TestContract_ListFormat(t *testing.T) {
	ctx := context.Background()
	c, err := client.New(ctx, client.WithAutoStart())
	require.NoError(t, err)

	_, perr := policy.List(ctx, c, "")
	require.NotErrorIs(t, perr, client.ErrUnexpectedFormat, "sbx policy ls table format drifted")
	require.NoError(t, perr)

	_, serr := secret.List(ctx, c, "")
	require.NotErrorIs(t, serr, client.ErrUnexpectedFormat, "sbx secret ls --json format drifted")
	require.NoError(t, serr)
}
