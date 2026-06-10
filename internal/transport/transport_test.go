package transport

import (
	"context"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

// startStub serves handler on a temp unix socket and returns its path.
func startStub(t *testing.T, handler http.Handler) string {
	t.Helper()
	dir := t.TempDir()
	sock := filepath.Join(dir, "d.sock")
	l, err := net.Listen("unix", sock)
	require.NoError(t, err)
	srv := &http.Server{Handler: handler}
	go srv.Serve(l)
	t.Cleanup(func() { srv.Close(); os.Remove(sock) })
	return sock
}

func TestTransport_GetJSON(t *testing.T) {
	sock := startStub(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/health", r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"status":"healthy"}`))
	}))
	tr := New(sock)
	var out struct {
		Status string `json:"status"`
	}
	err := tr.DoJSON(context.Background(), http.MethodGet, "/health", nil, &out)
	require.NoError(t, err)
	require.Equal(t, "healthy", out.Status)
}
