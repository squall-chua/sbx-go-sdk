package cli

import (
	"context"
	"os"
	"path/filepath"
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
