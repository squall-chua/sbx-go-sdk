package sandbox

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDefinition_ToRunArgs(t *testing.T) {
	d := newDefinition(
		WithAgent("claude"),
		WithWorkspace("/abs/ws"),
		WithCPUs(4),
		WithAgentArgs("--model", "opus"),
	)
	args, err := d.toRunArgs()
	require.NoError(t, err)
	require.Equal(t, []string{
		"run", "claude", "/abs/ws", "--cpus", "4", "--", "--model", "opus",
	}, args)
}

func TestDefinition_ToRunArgs_NoAgentArgs(t *testing.T) {
	d := newDefinition(WithAgent("shell"), WithWorkspace("/w"))
	args, err := d.toRunArgs()
	require.NoError(t, err)
	require.Equal(t, []string{"run", "shell", "/w"}, args)
}
