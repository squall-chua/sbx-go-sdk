//go:build integration

package integration

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/squall-chua/sbx-go-sdk/client"
	"github.com/squall-chua/sbx-go-sdk/kit"
	"github.com/squall-chua/sbx-go-sdk/sandbox"
	"github.com/stretchr/testify/require"
)

// fixtureKit copies testdata/fixture-kit into a fresh temp directory and
// returns that directory. Copying keeps every test free to chdir without
// disturbing the repo.
func fixtureKit(t *testing.T) string {
	t.Helper()
	src := filepath.Join("testdata", "fixture-kit", "spec.yaml")
	body, err := os.ReadFile(src)
	require.NoError(t, err)

	dir := filepath.Join(t.TempDir(), "fixture-kit")
	require.NoError(t, os.MkdirAll(dir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "spec.yaml"), body, 0o644))
	return dir
}

// Requires a real sbx on PATH. Needs no daemon, no sandbox and no network.
func TestSmoke_KitValidateInspectPack(t *testing.T) {
	ctx := context.Background()
	c, err := client.New(ctx, client.WithAutoStart())
	require.NoError(t, err)

	dir := fixtureKit(t)

	require.NoError(t, kit.Validate(ctx, c, dir))

	info, err := kit.Inspect(ctx, c, dir)
	require.NoError(t, err)
	require.Equal(t, "fixture-kit", info.Manifest.Name)
	require.Equal(t, "mixin", info.Manifest.Kind)
	require.Equal(t, "2", info.Manifest.SchemaVersion)
	require.NotEmpty(t, info.Caps, "caps.network should survive as raw JSON")

	out := filepath.Join(t.TempDir(), "fixture-kit.zip")
	require.NoError(t, kit.Pack(ctx, c, dir, out))
	st, err := os.Stat(out)
	require.NoError(t, err)
	require.Greater(t, st.Size(), int64(0))
}

// A malformed kit must come back as ErrKitRejected, and a missing path must
// not — `sbx kit validate` exits 1 for both, and only the "INVALID:" prefix
// separates them.
func TestSmoke_KitValidateRejectionIsClassified(t *testing.T) {
	ctx := context.Background()
	c, err := client.New(ctx, client.WithAutoStart())
	require.NoError(t, err)

	bad := filepath.Join(t.TempDir(), "bad-kit")
	require.NoError(t, os.MkdirAll(bad, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(bad, "spec.yaml"),
		[]byte("schemaVersion: \"2\"\nkind: mixin\nname: bad\nversion: 0.1.0\nnope: true\n"), 0o644))

	err = kit.Validate(ctx, c, bad)
	require.ErrorIs(t, err, client.ErrKitRejected)

	err = kit.Validate(ctx, c, filepath.Join(t.TempDir(), "no-such-directory"))
	require.Error(t, err)
	require.NotErrorIs(t, err, client.ErrKitRejected)
}

// AddKit must record an ABSOLUTE path. A relative one is resolved by the
// daemon against its own working directory, silently recording a path that
// does not exist and breaking every later add on that sandbox. A unit test
// can only prove the SDK passed an absolute path; only this proves the
// daemon recorded the right one.
//
// Creates and removes its own sandbox. Recreates a container, so it is slow.
func TestSmoke_AddKitRecordsAbsolutePath(t *testing.T) {
	ctx := context.Background()
	c, err := client.New(ctx, client.WithAutoStart())
	require.NoError(t, err)

	sb, err := sandbox.Create(ctx, c,
		sandbox.WithAgent("shell"),
		sandbox.WithWorkspace(t.TempDir()),
	)
	require.NoError(t, err)
	t.Cleanup(func() { sb.Remove(ctx) })

	dir := fixtureKit(t)
	t.Chdir(filepath.Dir(dir))

	// Deliberately relative. This is the call that used to poison the list.
	require.NoError(t, sb.AddKit(ctx, "./fixture-kit"))

	// absLocal resolves via filepath.Abs, which uses the resolved working
	// directory, not necessarily t.TempDir()'s own path — on a host where the
	// temp directory sits behind a symlink (e.g. macOS's /tmp -> /private/tmp)
	// the two differ. Resolve dir the same way before comparing.
	wantDir, err := filepath.EvalSymlinks(dir)
	require.NoError(t, err)

	kits, err := sb.Kits(ctx)
	require.NoError(t, err)
	require.Contains(t, kits, wantDir,
		"the daemon recorded a path resolved against its own working directory")
}
