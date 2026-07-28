// Package cli drives the `sbx` binary for orchestration-heavy operations that
// have no daemon REST path (sandbox create, daemon start, etc.).
package cli

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"time"
)

// waitDelay bounds how long Run waits after the context is done, or after
// the process exits, before killing the child and closing its pipes. Cancel
// sends an interrupt, which a child may ignore, and the SDK's captured
// stdout/stderr are pipes a surviving grandchild can hold open — without
// this, either case blocks Run forever. Generous on purpose: it must not cut
// short a legitimate teardown, only bound a hang. Tests shorten it.
var waitDelay = 10 * time.Second

// ErrBinaryNotFound is returned when the sbx binary cannot be located.
var ErrBinaryNotFound = errors.New("sbx binary not found")

// Error is a non-zero exit from an sbx shell-out.
type Error struct {
	Args     []string
	ExitCode int
	Stderr   string
}

func (e *Error) Error() string {
	return fmt.Sprintf("sbx %v failed (exit %d): %s", e.Args, e.ExitCode, e.Stderr)
}

// Runner runs the resolved sbx binary.
type Runner struct{ bin string }

// NewRunner resolves the binary: if path is set it must exist; otherwise PATH
// is searched for "sbx".
func NewRunner(path string) (*Runner, error) {
	if path != "" {
		if _, err := os.Stat(path); err != nil {
			return nil, fmt.Errorf("%w: %s", ErrBinaryNotFound, path)
		}
		return &Runner{bin: path}, nil
	}
	p, err := exec.LookPath("sbx")
	if err != nil {
		return nil, ErrBinaryNotFound
	}
	return &Runner{bin: p}, nil
}

// Bin returns the resolved binary path.
func (r *Runner) Bin() string { return r.bin }

// Capture runs `sbx args...` with extra env (KEY=VALUE), inheriting os.Environ,
// and returns combined stdout. Non-zero exit yields *Error.
func (r *Runner) Capture(ctx context.Context, extraEnv []string, args ...string) (string, error) {
	return r.CaptureStdin(ctx, nil, extraEnv, args...)
}

// CaptureStdin runs `sbx args...` like Capture, but wires stdin to the given
// reader so flags such as `secret set --password-stdin` can be fed a value
// without exposing it in the argument vector. A nil stdin reads as empty.
//
// On cancellation the child is sent an interrupt and given waitDelay to exit
// before being killed. If the child exits successfully but a grandchild
// still holds the stdout/stderr pipes open past waitDelay, Wait returns
// exec.ErrWaitDelay rather than nil — that surfaces here as a non-nil error,
// specifically *Error with ExitCode -1, even though the command itself
// succeeded.
func (r *Runner) CaptureStdin(ctx context.Context, stdin io.Reader, extraEnv []string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, r.bin, args...)
	cmd.Env = append(os.Environ(), extraEnv...)
	if stdin == nil {
		stdin = bytes.NewReader(nil)
	}
	cmd.Stdin = stdin
	cmd.Cancel = func() error { return cmd.Process.Signal(os.Interrupt) }
	cmd.WaitDelay = waitDelay
	var out, errb bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errb
	err := cmd.Run()
	if err != nil {
		var ee *exec.ExitError
		if errors.As(err, &ee) {
			return out.String(), &Error{Args: args, ExitCode: ee.ExitCode(), Stderr: errb.String()}
		}
		return out.String(), &Error{Args: args, ExitCode: -1, Stderr: err.Error()}
	}
	return out.String(), nil
}

// Stdio overrides the child's stdio; zero values inherit os.Stdin/out/err.
type Stdio struct {
	In  io.Reader
	Out io.Writer
	Err io.Writer
}

// Inherit runs `sbx args...` wired to the given (or inherited) terminal stdio and
// returns the child's exit code. A non-zero exit is returned as (code, nil); only
// failures to start/wait are returned as a non-nil error.
//
// On cancellation the child is sent an interrupt and given waitDelay to exit
// before being killed, bounding a child that ignores the interrupt. Unlike
// CaptureStdin, stdio here is passed through as real *os.File descriptors
// rather than pipes, so there is no copy goroutine for a grandchild to stall;
// waitDelay only backstops the ignored-interrupt case.
func (r *Runner) Inherit(ctx context.Context, s Stdio, extraEnv []string, args ...string) (int, error) {
	cmd := exec.CommandContext(ctx, r.bin, args...)
	cmd.Env = append(os.Environ(), extraEnv...)
	cmd.Stdin = orDefault[io.Reader](s.In, os.Stdin)
	cmd.Stdout = orDefault[io.Writer](s.Out, os.Stdout)
	cmd.Stderr = orDefault[io.Writer](s.Err, os.Stderr)
	cmd.Cancel = func() error { return cmd.Process.Signal(os.Interrupt) }
	cmd.WaitDelay = waitDelay
	if err := cmd.Run(); err != nil {
		var ee *exec.ExitError
		if errors.As(err, &ee) {
			return ee.ExitCode(), nil
		}
		return -1, err
	}
	return 0, nil
}

func orDefault[T comparable](v, def T) T {
	var zero T
	if v == zero {
		return def
	}
	return v
}
