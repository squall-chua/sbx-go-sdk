package sandbox

import (
	"context"
	"net"
	"net/http"
	"path/filepath"
	"testing"

	"github.com/squall-chua/sbx-go-sdk/client"
	"github.com/stretchr/testify/require"
)

func stubClient(t *testing.T, h http.Handler) *client.Client {
	t.Helper()
	sock := filepath.Join(t.TempDir(), "d.sock")
	l, err := net.Listen("unix", sock)
	require.NoError(t, err)
	srv := &http.Server{Handler: h}
	go srv.Serve(l)
	t.Cleanup(func() { srv.Close() })
	c, err := client.New(context.Background(), client.WithSocketPath(sock))
	require.NoError(t, err)
	return c
}

func TestList(t *testing.T) {
	c := stubClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/sandbox", r.URL.Path)
		w.Write([]byte(`[{"name":"a","agent":"claude","status":"SANDBOX_STATUS_RUNNING","workspace":"/w"}]`))
	}))
	sbs, err := List(context.Background(), c)
	require.NoError(t, err)
	require.Len(t, sbs, 1)
	require.Equal(t, "a", sbs[0].Name())
	require.Equal(t, "claude", sbs[0].Agent())
	require.True(t, sbs[0].IsRunning())
}

func TestGet_NotFound(t *testing.T) {
	c := stubClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(404)
		w.Write([]byte(`{"message":"not found"}`))
	}))
	_, err := Get(context.Background(), c, "nope")
	require.ErrorIs(t, err, client.ErrSandboxNotFound)
}
