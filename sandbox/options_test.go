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

func TestWithPublish_EmitsRepeatedFlagOnCreateAndRun(t *testing.T) {
	d := newDefinition(
		WithAgent("shell"),
		WithWorkspace("/ws"),
		WithPublish("8080", "127.0.0.1:3000:3000/tcp"),
	)

	createArgs, err := d.toCreateArgs()
	require.NoError(t, err)
	require.Subset(t, createArgs, []string{"-p", "8080"})
	require.Subset(t, createArgs, []string{"-p", "127.0.0.1:3000:3000/tcp"})

	runArgs, err := d.toRunArgs()
	require.NoError(t, err)
	require.Subset(t, runArgs, []string{"-p", "8080"})
	require.Subset(t, runArgs, []string{"-p", "127.0.0.1:3000:3000/tcp"})
}

func TestWithPublish_AbsentWhenUnset(t *testing.T) {
	d := newDefinition(WithAgent("shell"), WithWorkspace("/ws"))
	args, err := d.toCreateArgs()
	require.NoError(t, err)
	require.NotContains(t, args, "-p")
}
