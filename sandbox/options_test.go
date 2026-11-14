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

// --deny-network and --static-mcp both arrived in sbx v0.38.0 and both repeat
// rather than taking a list.
func TestWithDenyNetworkAndStaticMCP_EmitRepeatedFlagsOnCreateAndRun(t *testing.T) {
	d := newDefinition(
		WithAgent("shell"),
		WithWorkspace("/ws"),
		WithDenyNetwork("evil.example.com", "*.tracker.io"),
		WithStaticMCP("notion", "atlassian"),
	)

	for name, build := range map[string]func() ([]string, error){
		"create": d.toCreateArgs,
		"run":    d.toRunArgs,
	} {
		args, err := build()
		require.NoError(t, err, name)
		require.Subset(t, args, []string{"--deny-network", "evil.example.com"}, name)
		require.Subset(t, args, []string{"--deny-network", "*.tracker.io"}, name)
		require.Subset(t, args, []string{"--static-mcp", "notion"}, name)
		require.Subset(t, args, []string{"--static-mcp", "atlassian"}, name)
	}
}

func TestWithDenyNetworkAndStaticMCP_AbsentWhenUnset(t *testing.T) {
	d := newDefinition(WithAgent("shell"), WithWorkspace("/ws"))
	args, err := d.toCreateArgs()
	require.NoError(t, err)
	require.NotContains(t, args, "--deny-network")
	require.NotContains(t, args, "--static-mcp")
	require.NotContains(t, args, "--skills")
}

// --no-share-skills is deprecated at sbx v0.51.0; the SDK emits --skills off.
func TestWithoutSharedSkills(t *testing.T) {
	d := newDefinition(WithAgent("shell"), WithWorkspace("/ws"), WithoutSharedSkills())

	createArgs, err := d.toCreateArgs()
	require.NoError(t, err)
	require.Subset(t, createArgs, []string{"--skills", "off"})
	require.NotContains(t, createArgs, "--no-share-skills")

	runArgs, err := d.toRunArgs()
	require.NoError(t, err)
	require.Subset(t, runArgs, []string{"--skills", "off"})
	require.NotContains(t, runArgs, "--no-share-skills")
}

func TestCreateTimeFlags_OnCreateAndRun(t *testing.T) {
	wd, err := os.Getwd()
	require.NoError(t, err)
	d := newDefinition(WithAgent("shell"), WithWorkspace("/ws"),
		WithSkills("readwrite"), WithPull("missing"),
		WithMount("/abs/data:/data", "rel/cache:/cache"))
	createArgs, err := d.toCreateArgs()
	require.NoError(t, err)
	runArgs, err := d.toRunArgs()
	require.NoError(t, err)
	for name, args := range map[string][]string{"create": createArgs, "run": runArgs} {
		require.Subset(t, args, []string{"--skills", "readwrite"}, name)
		require.Subset(t, args, []string{"--pull", "missing"}, name)
		require.Contains(t, args, "/abs/data:/data", name)
		require.Contains(t, args, filepath.Join(wd, "rel/cache")+":/cache", name)
		require.Equal(t, 2, countFlag(args, "--mount"), name)
	}
}

// The last skills option wins, whichever spelling set it.
func TestWithSkills_LastWins(t *testing.T) {
	d := newDefinition(WithAgent("shell"), WithWorkspace("/ws"), WithoutSharedSkills(), WithSkills("readonly"))
	args, err := d.toCreateArgs()
	require.NoError(t, err)
	require.Subset(t, args, []string{"--skills", "readonly"})
	require.Equal(t, 1, countFlag(args, "--skills"))
}

func TestRunOnlyFlags(t *testing.T) {
	d := newDefinition(WithAgent("shell"), WithWorkspace("/ws"), WithRemoveOnExit(), WithWorkdir("sub"),
		WithAgentArgs("-c", "true"))
	runArgs, err := d.toRunArgs()
	require.NoError(t, err)
	require.Equal(t, []string{"run", "shell", "/ws", "--rm", "-w", "sub", "--", "-c", "true"}, runArgs)

	createArgs, err := d.toCreateArgs()
	require.NoError(t, err)
	require.NotContains(t, createArgs, "--rm")
	require.NotContains(t, createArgs, "-w")
}

func countFlag(args []string, flag string) int {
	n := 0
	for _, a := range args {
		if a == flag {
			n++
		}
	}
	return n
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

// --env-file must be emitted before -e, because the CLI resolves them in flag
// order and documents that --env wins over any file.
func TestWithEnv_EmitsFilesBeforeVarsOnCreateAndRun(t *testing.T) {
	d := newDefinition(
		WithAgent("shell"),
		WithWorkspace("/ws"),
		WithEnv("DIRECT=one"),
		WithEnvFile("/tmp/a.env", "/tmp/b.env"),
		WithEnv("INHERITED"),
	)

	for name, build := range map[string]func() ([]string, error){
		"create": d.toCreateArgs,
		"run":    d.toRunArgs,
	} {
		args, err := build()
		require.NoError(t, err, name)
		require.Subset(t, args, []string{"-e", "DIRECT=one"}, name)
		require.Subset(t, args, []string{"-e", "INHERITED"}, name)
		require.Subset(t, args, []string{"--env-file", "/tmp/a.env"}, name)
		require.Subset(t, args, []string{"--env-file", "/tmp/b.env"}, name)

		joined := strings.Join(args, " ")
		require.Less(t, strings.Index(joined, "--env-file"), strings.Index(joined, "-e "),
			"%s: every --env-file must precede the first -e", name)
		require.Less(t, strings.Index(joined, "/tmp/a.env"), strings.Index(joined, "/tmp/b.env"),
			"%s: file order is preserved, a later file wins", name)
	}
}

func TestWithEnv_AbsentWhenUnset(t *testing.T) {
	d := newDefinition(WithAgent("shell"), WithWorkspace("/ws"))
	args, err := d.toCreateArgs()
	require.NoError(t, err)
	require.NotContains(t, args, "-e")
	require.NotContains(t, args, "--env-file")
}
