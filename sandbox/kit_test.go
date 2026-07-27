package sandbox

import (
	"context"
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"github.com/squall-chua/sbx-go-sdk/client"
	"github.com/stretchr/testify/require"
)

// The daemon records the kit list verbatim and re-resolves it on every later
// add, resolving a relative path against its own working directory rather
// than the caller's. A relative path therefore records one that does not
// exist and breaks every subsequent add on that sandbox.
func TestAbsLocal_ExistingRelativePathBecomesAbsolute(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.Mkdir(filepath.Join(dir, "mykit"), 0o755))
	t.Chdir(dir)

	got := absLocal("./mykit")

	require.True(t, filepath.IsAbs(got), "want an absolute path, got %q", got)
	require.Equal(t, filepath.Join(dir, "mykit"), filepath.Clean(got))
}

func TestAbsLocal_NonPathReferencesPassThroughUntouched(t *testing.T) {
	t.Chdir(t.TempDir())

	for _, ref := range []string{
		"ghcr.io/myorg/mcp-postgres:1.0",
		"git+https://github.com/org/kits.git#dir=mcp-postgres",
		"./does-not-exist",
	} {
		require.Equal(t, ref, absLocal(ref))
	}
}

func TestAddKit_PassesSandboxNameThenReference(t *testing.T) {
	argFile := filepath.Join(t.TempDir(), "args.txt")
	c := clientWithRecordingSbx(t, argFile)
	sb := NewForTest(c, "my-sandbox")

	require.NoError(t, sb.AddKit(context.Background(), "ghcr.io/org/kit:1.0"))

	args, err := os.ReadFile(argFile)
	require.NoError(t, err)
	require.Contains(t, string(args), "kit add my-sandbox ghcr.io/org/kit:1.0")
}

func TestAddKit_AbsolutisesALocalDirectory(t *testing.T) {
	argFile := filepath.Join(t.TempDir(), "args.txt")
	c := clientWithRecordingSbx(t, argFile)
	sb := NewForTest(c, "my-sandbox")

	dir := t.TempDir()
	require.NoError(t, os.Mkdir(filepath.Join(dir, "mykit"), 0o755))
	t.Chdir(dir)

	require.NoError(t, sb.AddKit(context.Background(), "./mykit"))

	args, err := os.ReadFile(argFile)
	require.NoError(t, err)
	require.NotContains(t, string(args), "./mykit",
		"a relative path reaches the daemon and poisons the recorded kit list")
	require.Contains(t, string(args), filepath.Join(dir, "mykit"))
}

// The kit list is a container label, not a SandboxInfo field: sandboxapi's
// SandboxInfo has no kit field in DWARF. The label value is a JSON string
// array. Verified 2026-07-27 against sandboxd 0.24.0.
func TestKits_DecodesTheLabelJSONArray(t *testing.T) {
	c := stubClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"id":"x","name":"my-sandbox","status":"running","workspace":"/ws",
			"labels":{"com.docker.sandbox.kits":"[\"/abs/a\",\"/abs/b\"]"}}`))
	}))
	sb := NewForTest(c, "my-sandbox")

	kits, err := sb.Kits(context.Background())

	require.NoError(t, err)
	require.Equal(t, []string{"/abs/a", "/abs/b"}, kits)
}

func TestKits_MissingLabelIsEmptyNotAnError(t *testing.T) {
	c := stubClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"id":"x","name":"my-sandbox","status":"running","workspace":"/ws",
			"labels":{"com.docker.sandbox.agent":"shell"}}`))
	}))
	sb := NewForTest(c, "my-sandbox")

	kits, err := sb.Kits(context.Background())

	require.NoError(t, err)
	require.Empty(t, kits)
}

func TestKits_MalformedLabelIsUnexpectedFormat(t *testing.T) {
	c := stubClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"id":"x","name":"my-sandbox","status":"running","workspace":"/ws",
			"labels":{"com.docker.sandbox.kits":"not json"}}`))
	}))
	sb := NewForTest(c, "my-sandbox")

	_, err := sb.Kits(context.Background())

	require.ErrorIs(t, err, client.ErrUnexpectedFormat)
}
