//go:build integration

package integration

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/squall-chua/sbx-go-sdk/client"
	"github.com/squall-chua/sbx-go-sdk/exec"
	"github.com/squall-chua/sbx-go-sdk/sandbox"
	"github.com/squall-chua/sbx-go-sdk/skillstore"
	"github.com/stretchr/testify/require"
)

// WithEnv is the SDK's only route to the daemon's SandboxCreateRequest
// .Environment field, so this checks all three forms the CLI accepts reach the
// sandbox: an explicit KEY=VALUE, a bare KEY inherited from this process, and
// a file.
func TestSmoke_CreateWithEnv(t *testing.T) {
	ctx := context.Background()
	c, err := client.New(ctx, client.WithAutoStart())
	require.NoError(t, err)

	t.Setenv("SBX_SDK_INHERITED", "from-the-caller")

	envFile := filepath.Join(t.TempDir(), "probe.env")
	require.NoError(t, os.WriteFile(envFile, []byte("SBX_SDK_FROM_FILE=from-the-file\n"), 0o600))

	sb, err := sandbox.Create(ctx, c,
		sandbox.WithAgent("shell"),
		sandbox.WithWorkspace(t.TempDir()),
		sandbox.WithEnv("SBX_SDK_DIRECT=from-the-flag", "SBX_SDK_INHERITED"),
		sandbox.WithEnvFile(envFile),
	)
	require.NoError(t, err)
	t.Cleanup(func() { sb.Remove(ctx) })

	code, r, err := exec.Exec(ctx, sb, []string{"env"}, exec.WithAutoStart())
	require.NoError(t, err)
	out, _ := io.ReadAll(r)
	require.Equal(t, 0, code)

	require.Contains(t, string(out), "SBX_SDK_DIRECT=from-the-flag")
	require.Contains(t, string(out), "SBX_SDK_INHERITED=from-the-caller",
		"a bare KEY takes its value from the calling process")
	require.Contains(t, string(out), "SBX_SDK_FROM_FILE=from-the-file")
}

// Prune only ever touches stopped sandboxes, so this stops one and leaves a
// second running as the control: the dry run must name the first and not the
// second, and the real prune must remove exactly the first.
func TestSmoke_Prune(t *testing.T) {
	ctx := context.Background()
	c, err := client.New(ctx, client.WithAutoStart())
	require.NoError(t, err)

	stopped, err := sandbox.Create(ctx, c, sandbox.WithAgent("shell"), sandbox.WithWorkspace(t.TempDir()))
	require.NoError(t, err)
	t.Cleanup(func() { stopped.Remove(ctx) })

	running, err := sandbox.Create(ctx, c, sandbox.WithAgent("shell"), sandbox.WithWorkspace(t.TempDir()))
	require.NoError(t, err)
	t.Cleanup(func() { running.Remove(ctx) })

	require.NoError(t, stopped.Start(ctx))
	require.NoError(t, running.Start(ctx))
	require.NoError(t, stopped.Stop(ctx))

	// A window far longer than this test has been alive excludes every
	// candidate, which proves the filter reaches the CLI.
	none, err := sandbox.Prune(ctx, c, sandbox.WithDryRun(), sandbox.WithStoppedLongerThan("24h"))
	require.NoError(t, err)
	require.NotContains(t, none, stopped.Name(), "stopped seconds ago, not 24h ago")

	candidates, err := sandbox.Prune(ctx, c, sandbox.WithDryRun())
	require.NoError(t, err)
	require.Contains(t, candidates, stopped.Name())
	require.NotContains(t, candidates, running.Name(), "a running sandbox is never a candidate")

	removed, err := sandbox.Prune(ctx, c)
	require.NoError(t, err)
	require.Contains(t, removed, stopped.Name())
	require.NotContains(t, removed, running.Name())

	after, err := sandbox.List(ctx, c)
	require.NoError(t, err)
	var names []string
	for _, s := range after {
		names = append(names, s.Name())
	}
	require.NotContains(t, names, stopped.Name())
	require.Contains(t, names, running.Name())
}

// skills ls and daemon status --json both read real host state, so this only
// checks the SDK reads them the way the CLI writes them.
func TestSmoke_SkillsListAndDaemonLogPath(t *testing.T) {
	ctx := context.Background()
	c, err := client.New(ctx, client.WithAutoStart())
	require.NoError(t, err)

	store, err := skillstore.List(ctx, c)
	require.NoError(t, err)
	require.True(t, filepath.IsAbs(store.Path), "the store path is absolute: %q", store.Path)
	for _, name := range store.Skills {
		require.NotEmpty(t, name)
		require.NotContains(t, name, " ", "a skill is one folder name per line")
	}

	logs, err := c.DaemonLogPath(ctx)
	require.NoError(t, err)
	require.True(t, filepath.IsAbs(logs), "the log path is absolute: %q", logs)

	st, err := c.DaemonStatus(ctx)
	require.NoError(t, err)
	require.True(t, st.Running)
	require.Equal(t, filepath.Dir(st.Socket), filepath.Dir(logs),
		"the daemon keeps its socket and its log in one state directory")
}
