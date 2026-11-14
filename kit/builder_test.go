package kit

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestBuilder_Argv(t *testing.T) {
	argFile := filepath.Join(t.TempDir(), "args.txt")
	c := fakeClient(t, argFile, "BUILDER-OUT", "", 0)
	ctx := context.Background()

	out, err := BuilderStatusRaw(ctx, c)
	require.NoError(t, err)
	require.Contains(t, out, "BUILDER-OUT")

	out, err = BuilderHistoryRaw(ctx, c, "ls", "--format", "json")
	require.NoError(t, err)
	require.Contains(t, out, "BUILDER-OUT")

	require.NoError(t, RemoveBuilder(ctx, c))

	data, _ := os.ReadFile(argFile)
	require.Equal(t,
		"kit builder status\n"+
			"kit builder history ls --format json\n"+
			"kit builder rm --force\n", string(data))
}

// With no subcommand the CLI would only print its help.
func TestBuilderHistoryRaw_RequiresASubcommand(t *testing.T) {
	argFile := filepath.Join(t.TempDir(), "args.txt")
	c := fakeClient(t, argFile, "", "", 0)
	_, err := BuilderHistoryRaw(context.Background(), c)
	require.Error(t, err)
	_, statErr := os.Stat(argFile)
	require.True(t, os.IsNotExist(statErr), "the CLI must not run")
}

func TestBuilder_NonZeroExitIsAnError(t *testing.T) {
	c := fakeClient(t, "", "", `ERROR: no record found for ref "nope"`, 1)
	_, err := BuilderHistoryRaw(context.Background(), c, "inspect", "nope")
	require.ErrorContains(t, err, "no record found")
}
