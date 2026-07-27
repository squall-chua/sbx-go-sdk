package sandbox

import (
	"os"
	"path/filepath"
	"strings"
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

func TestWithKit_EmitsRepeatedFlagOnCreate(t *testing.T) {
	d := newDefinition(
		WithAgent("shell"),
		WithWorkspace("/ws"),
		WithKit("ghcr.io/org/a:1.0"),
		WithKit("ghcr.io/org/b:1.0", "ghcr.io/org/c:1.0"),
	)

	args, err := d.toCreateArgs()
	require.NoError(t, err)

	joined := strings.Join(args, " ")
	require.Contains(t, joined, "--kit ghcr.io/org/a:1.0")
	require.Contains(t, joined, "--kit ghcr.io/org/b:1.0")
	require.Contains(t, joined, "--kit ghcr.io/org/c:1.0")
}

func TestWithKit_AbsentWhenUnset(t *testing.T) {
	d := newDefinition(WithAgent("shell"), WithWorkspace("/ws"))
	args, err := d.toCreateArgs()
	require.NoError(t, err)
	require.NotContains(t, args, "--kit")
}

// create --kit records the kit list the same way kit add does, so a relative
// path is resolved against the daemon's working directory and recorded wrong.
func TestWithKit_AbsolutisesALocalDirectory(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.Mkdir(filepath.Join(dir, "mykit"), 0o755))
	t.Chdir(dir)

	d := newDefinition(WithAgent("shell"), WithWorkspace("/ws"), WithKit("./mykit"))
	args, err := d.toCreateArgs()
	require.NoError(t, err)

	joined := strings.Join(args, " ")
	require.NotContains(t, joined, "--kit ./mykit")
	require.Contains(t, joined, "--kit "+filepath.Join(dir, "mykit"))
}

func TestWithKit_EmitsRepeatedFlagOnRun(t *testing.T) {
	d := newDefinition(
		WithAgent("shell"),
		WithWorkspace("/ws"),
		WithKit("ghcr.io/org/a:1.0"),
		WithKit("ghcr.io/org/b:1.0", "ghcr.io/org/c:1.0"),
	)

	args, err := d.toRunArgs()
	require.NoError(t, err)

	joined := strings.Join(args, " ")
	require.Contains(t, joined, "--kit ghcr.io/org/a:1.0")
	require.Contains(t, joined, "--kit ghcr.io/org/b:1.0")
	require.Contains(t, joined, "--kit ghcr.io/org/c:1.0")
}
