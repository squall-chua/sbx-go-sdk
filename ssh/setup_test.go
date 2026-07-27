package ssh

import (
	"context"
	"os"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSetup_UsesDocumentedSetupSSHPath(t *testing.T) {
	c, argFile := newFakeSbx(t, 0, "", "")

	require.NoError(t, Setup(context.Background(), c))

	args, err := os.ReadFile(argFile)
	require.NoError(t, err)
	require.Contains(t, string(args), "setup ssh\n",
		"sbx ssh setup is an undocumented hidden alias; sbx setup ssh is the documented path")
	require.NotContains(t, string(args), "ssh setup")
}

func TestSetup_AliasIsStillPassed(t *testing.T) {
	c, argFile := newFakeSbx(t, 0, "", "")

	require.NoError(t, Setup(context.Background(), c, WithAlias("work")))

	args, err := os.ReadFile(argFile)
	require.NoError(t, err)
	require.Contains(t, string(args), "setup ssh --alias work")
}
