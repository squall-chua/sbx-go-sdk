package policy

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/squall-chua/sbx-go-sdk/client"
	"github.com/stretchr/testify/require"
)

func TestListApprovals_KeepsEntriesRaw(t *testing.T) {
	argFile := filepath.Join(t.TempDir(), "args.txt")
	c := fakeClient(t, argFile, `[{"id":"a1","x":1},{"id":"a2"}]`)
	got, err := ListApprovals(context.Background(), c)
	require.NoError(t, err)
	require.Len(t, got, 2)
	require.JSONEq(t, `{"id":"a1","x":1}`, string(got[0]))
	data, _ := os.ReadFile(argFile)
	require.Equal(t, "policy approval ls --json", strings.TrimSpace(string(data)))
}

// sbx v0.51.0 prints [] with nothing pending.
func TestListApprovals_EmptyIsEmptyNotError(t *testing.T) {
	got, err := ListApprovals(context.Background(), fakeClient(t, "", `[]`))
	require.NoError(t, err)
	require.NotNil(t, got)
	require.Empty(t, got)
}

func TestListApprovals_NonArrayIsUnexpectedFormat(t *testing.T) {
	for _, out := range []string{`null`, `{"approvals":[]}`, `No pending approvals.`} {
		_, err := ListApprovals(context.Background(), fakeClient(t, "", out))
		require.ErrorIs(t, err, client.ErrUnexpectedFormat, out)
	}
}

func TestInspectApprovalRawAndRespond_Argv(t *testing.T) {
	argFile := filepath.Join(t.TempDir(), "args.txt")
	c := fakeClient(t, argFile, "detail text")
	ctx := context.Background()
	out, err := InspectApprovalRaw(ctx, c, "a1")
	require.NoError(t, err)
	require.Contains(t, out, "detail text")
	require.NoError(t, RespondApproval(ctx, c, "a1", "allow-once"))
	data, _ := os.ReadFile(argFile)
	require.Equal(t, []string{
		"policy approval inspect a1",
		"policy approval respond a1 --option allow-once",
	}, strings.Split(strings.TrimSpace(string(data)), "\n"))
}

func TestApprovals_EmptyArgsRejectedLocally(t *testing.T) {
	argFile := filepath.Join(t.TempDir(), "args.txt")
	c := recordingClient(t, argFile)
	ctx := context.Background()
	_, err := InspectApprovalRaw(ctx, c, "")
	require.Error(t, err)
	require.Error(t, RespondApproval(ctx, c, "", "o"))
	require.Error(t, RespondApproval(ctx, c, "a1", ""))
	_, statErr := os.Stat(argFile)
	require.True(t, os.IsNotExist(statErr), "the CLI must not run")
}
