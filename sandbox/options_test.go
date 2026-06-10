package sandbox

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDefinition_ToCreateArgs(t *testing.T) {
	d := newDefinition(
		WithAgent("claude"),
		WithWorkspace("/abs/ws"),
		WithWorkspace("/abs/docs:ro"),
		WithName("proj"),
		WithCPUs(4),
		WithMemory("8g"),
		WithProfile("balanced"),
		WithClone(),
	)
	args, err := d.toCreateArgs()
	require.NoError(t, err)
	require.Equal(t, []string{
		"create", "claude", "/abs/ws", "/abs/docs:ro",
		"--name", "proj", "--cpus", "4", "--memory", "8g",
		"--profile", "balanced", "--clone",
	}, args)
}

func TestDefinition_RequiresAgentAndWorkspace(t *testing.T) {
	_, err := newDefinition(WithAgent("claude")).toCreateArgs()
	require.Error(t, err)
}
