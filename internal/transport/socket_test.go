package transport

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestResolveSocketPath_EnvOverride(t *testing.T) {
	t.Setenv("DOCKER_SANDBOXES_API", "/custom/sandboxd.sock")
	got, err := ResolveSocketPath("")
	require.NoError(t, err)
	require.Equal(t, "/custom/sandboxd.sock", got)
}

func TestResolveSocketPath_ExplicitWins(t *testing.T) {
	t.Setenv("DOCKER_SANDBOXES_API", "/env.sock")
	got, err := ResolveSocketPath("/explicit.sock")
	require.NoError(t, err)
	require.Equal(t, "/explicit.sock", got)
}

func TestResolveSocketPath_DefaultXDG(t *testing.T) {
	t.Setenv("DOCKER_SANDBOXES_API", "")
	t.Setenv("XDG_STATE_HOME", "/home/u/.local/state")
	got, err := ResolveSocketPath("")
	require.NoError(t, err)
	want := filepath.Join("/home/u/.local/state", "sandboxes", "sandboxes", "sandboxd", "sandboxd.sock")
	require.Equal(t, want, got)
}
