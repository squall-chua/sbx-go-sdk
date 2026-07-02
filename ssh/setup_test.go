package ssh

import (
	"context"
	"os"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSetupArgs(t *testing.T) {
	c, argFile := newFakeSbx(t, 0, "", "")
	ctx := context.Background()

	require.NoError(t, Setup(ctx, c))
	require.NoError(t, Setup(ctx, c, WithAlias("work"), WithRegenerate()))

	args, _ := os.ReadFile(argFile)
	lines := string(args)
	require.Contains(t, lines, "ssh setup\n") // no options -> bare
	require.Contains(t, lines, "ssh setup --alias work --regenerate")
}
