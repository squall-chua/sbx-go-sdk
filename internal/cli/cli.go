// Package cli drives the `sbx` binary for orchestration-heavy operations that
// have no daemon REST path (sandbox create, daemon start, etc.).
package cli

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
)

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
	cmd := exec.CommandContext(ctx, r.bin, args...)
	cmd.Env = append(os.Environ(), extraEnv...)
	cmd.Cancel = func() error { return cmd.Process.Signal(os.Interrupt) }
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
