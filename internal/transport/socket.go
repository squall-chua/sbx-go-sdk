package transport

import (
	"os"
	"path/filepath"
)

// EnvSocket is the env var sandboxlib.SocketPath reads to override the socket path.
const EnvSocket = "DOCKER_SANDBOXES_API"

// ResolveSocketPath returns the daemon socket path. Precedence:
// explicit arg > $DOCKER_SANDBOXES_API > $XDG_STATE_HOME/.../sandboxd.sock
// (default XDG_STATE_HOME is ~/.local/state).
func ResolveSocketPath(explicit string) (string, error) {
	if explicit != "" {
		return explicit, nil
	}
	if v := os.Getenv(EnvSocket); v != "" {
		return v, nil
	}
	state := os.Getenv("XDG_STATE_HOME")
	if state == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		state = filepath.Join(home, ".local", "state")
	}
	return filepath.Join(state, "sandboxes", "sandboxes", "sandboxd", "sandboxd.sock"), nil
}
