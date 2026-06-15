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

// TestContract_ListFormat guards the sbx table layout that policy.List and
// secret.List parse. If the CLI renames/reorders columns, strict header
// validation surfaces client.ErrUnexpectedFormat and this test fails so a
// maintainer can re-sync the parser — the table-format sibling of
// TestContract_VersionAlignment.
func TestContract_ListFormat(t *testing.T) {
	ctx := context.Background()
	c, err := client.New(ctx, client.WithAutoStart())
	require.NoError(t, err)

	_, perr := policy.List(ctx, c, "")
	require.NotErrorIs(t, perr, client.ErrUnexpectedFormat, "sbx policy ls table format drifted")
	require.NoError(t, perr)

	_, serr := secret.List(ctx, c, "")
	require.NotErrorIs(t, serr, client.ErrUnexpectedFormat, "sbx secret ls table format drifted")
	require.NoError(t, serr)
}
