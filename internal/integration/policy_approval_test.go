//go:build integration

package integration

import (
	"context"
	"errors"
	"testing"

	"github.com/squall-chua/sbx-go-sdk/client"
	"github.com/squall-chua/sbx-go-sdk/policy"
	"github.com/stretchr/testify/require"
)

// TestPolicy_Approvals reads the pending list and answers an ID that is not
// pending. It never answers a real approval: an ungoverned host has none.
func TestPolicy_Approvals(t *testing.T) {
	ctx := context.Background()
	c, err := client.New(ctx, client.WithAutoStart())
	require.NoError(t, err)

	_, err = policy.ListApprovals(ctx, c)
	require.NoError(t, err, "approval ls --json must stay a JSON array")

	for name, err := range map[string]error{
		"respond": policy.RespondApproval(ctx, c, "sdk-no-such-approval", "allow"),
		"inspect": func() error { _, err := policy.InspectApprovalRaw(ctx, c, "sdk-no-such-approval"); return err }(),
	} {
		var cliErr *client.CLIError
		require.True(t, errors.As(err, &cliErr), "%s: %v", name, err)
		require.Contains(t, cliErr.Stderr, "is not pending", name)
	}
}
