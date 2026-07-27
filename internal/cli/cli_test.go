package cli

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// fakeSbx writes a fake `sbx` script that echoes its args and exits with `code`.
func fakeSbx(t *testing.T, body string) string {
	t.Helper()
	dir := t.TempDir()
	p := filepath.Join(dir, "sbx")
	require.NoError(t, os.WriteFile(p, []byte("#!/bin/sh\n"+body), 0o755))
	return p
}

func TestRunner_Capture_Success(t *testing.T) {
	bin := fakeSbx(t, `echo "created $3"; exit 0`)
	r, err := NewRunner(bin)
	require.NoError(t, err)
	out, err := r.Capture(context.Background(), nil, "create", "shell", "myws", "--name", "n1")
	require.NoError(t, err)
	require.Contains(t, out, "created myws")
}

func TestRunner_Capture_NonZeroIsCLIError(t *testing.T) {
	bin := fakeSbx(t, `echo "boom" 1>&2; exit 3`)
	r, _ := NewRunner(bin)
	_, err := r.Capture(context.Background(), nil, "create", "shell", ".")
	require.Error(t, err)
	var ce *Error
	require.ErrorAs(t, err, &ce)
	require.Equal(t, 3, ce.ExitCode)
	require.Contains(t, ce.Stderr, "boom")
}

func TestNewRunner_MissingBinary(t *testing.T) {
	_, err := NewRunner("/no/such/sbx")
	require.ErrorIs(t, err, ErrBinaryNotFound)
}

func TestRunner_Inherit_ReturnsExitCode(t *testing.T) {
	bin := fakeSbx(t, `exit 7`)
	r, _ := NewRunner(bin)
	code, err := r.Inherit(context.Background(), Stdio{}, nil, "run", "shell")
	require.NoError(t, err) // non-zero exit is reported via code, not err
	require.Equal(t, 7, code)
}

func TestCaptureStdin_FeedsChildStdin(t *testing.T) {
	bin := filepath.Join(t.TempDir(), "sbx")
	// The fake binary echoes whatever it reads on stdin, prefixed, so the test
	// can prove the value arrived via stdin and not via the argument vector.
	script := "#!/bin/sh\nprintf 'stdin:'\ncat\n"
	require.NoError(t, os.WriteFile(bin, []byte(script), 0o755))

	r, err := NewRunner(bin)
	require.NoError(t, err)

	out, err := r.CaptureStdin(context.Background(), strings.NewReader("s3cr3t"), nil, "secret", "set")
	require.NoError(t, err)
	require.Equal(t, "stdin:s3cr3t", out)
}

func TestCaptureStdin_NilStdinIsEmpty(t *testing.T) {
	bin := filepath.Join(t.TempDir(), "sbx")
	script := "#!/bin/sh\nprintf 'stdin:'\ncat\n"
	require.NoError(t, os.WriteFile(bin, []byte(script), 0o755))

	r, err := NewRunner(bin)
	require.NoError(t, err)

	out, err := r.CaptureStdin(context.Background(), nil, nil, "x")
	require.NoError(t, err)
	require.Equal(t, "stdin:", out)
}

func TestCaptureStdin_NonZeroExitReturnsError(t *testing.T) {
	bin := filepath.Join(t.TempDir(), "sbx")
	script := "#!/bin/sh\necho boom >&2\nexit 3\n"
	require.NoError(t, os.WriteFile(bin, []byte(script), 0o755))

	r, err := NewRunner(bin)
	require.NoError(t, err)

	_, err = r.CaptureStdin(context.Background(), strings.NewReader("v"), nil, "x")
	var cliErr *Error
	require.ErrorAs(t, err, &cliErr)
	require.Equal(t, 3, cliErr.ExitCode)
	require.Contains(t, cliErr.Stderr, "boom")
}
