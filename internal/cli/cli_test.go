package cli

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

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

// withShortWaitDelay shortens the package-level waitDelay for the duration of
// a test and restores it afterward. Tests in this file must not run in
// parallel because they mutate this shared var.
func withShortWaitDelay(t *testing.T, d time.Duration) {
	t.Helper()
	old := waitDelay
	waitDelay = d
	t.Cleanup(func() { waitDelay = old })
}

func TestCaptureStdin_IgnoredInterruptIsBoundedByWaitDelay(t *testing.T) {
	withShortWaitDelay(t, 200*time.Millisecond)
	bin := fakeSbx(t, `trap '' INT; sleep 30`)
	r, err := NewRunner(bin)
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	start := time.Now()
	_, err = r.Capture(ctx, nil, "kit", "push")
	elapsed := time.Since(start)

	require.Error(t, err)
	require.Less(t, elapsed, 5*time.Second, "Capture should be bounded by waitDelay, not the child's 30s sleep")
}

func TestCaptureStdin_GrandchildHoldingPipeIsBoundedByWaitDelay(t *testing.T) {
	withShortWaitDelay(t, 200*time.Millisecond)
	bin := fakeSbx(t, `(sleep 30) & echo started; exit 0`)
	r, err := NewRunner(bin)
	require.NoError(t, err)

	start := time.Now()
	out, err := r.Capture(context.Background(), nil)
	elapsed := time.Since(start)

	require.Less(t, elapsed, 5*time.Second, "Capture should be bounded by waitDelay, not the grandchild's 30s sleep")
	// Documented trade-off: the direct child exited 0, but WaitDelay firing
	// while a grandchild still holds the output pipe makes Wait return
	// exec.ErrWaitDelay, which CaptureStdin surfaces as a *cli.Error with
	// ExitCode -1 rather than success.
	var cliErr *Error
	require.ErrorAs(t, err, &cliErr)
	require.Equal(t, -1, cliErr.ExitCode)
	require.Contains(t, out, "started")
}

func TestCaptureStdin_WellBehavedChildNotPenalisedByWaitDelay(t *testing.T) {
	withShortWaitDelay(t, 10*time.Second)
	// A direct `sleep 30` wouldn't work here: Signal() targets only the shell
	// PID, not sleep's, so the shell can't act on the interrupt until wait()
	// on sleep returns. Loop on a short sleep instead so the shell rechecks
	// for the pending signal often and the trap fires promptly.
	bin := fakeSbx(t, `trap 'exit 130' INT; while true; do sleep 0.1; done`)
	r, err := NewRunner(bin)
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	start := time.Now()
	_, err = r.Capture(ctx, nil, "kit", "push")
	elapsed := time.Since(start)

	require.Error(t, err)
	require.Less(t, elapsed, 5*time.Second, "a well-behaved child should exit promptly on the interrupt, well under the full waitDelay")
}
