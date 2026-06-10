package template

import (
	"context"
	"io"
	"net"
	"net/http"
	"path/filepath"
	"strings"
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

func TestListAndInspect(t *testing.T) {
	c := stubClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/docker/images":
			w.Write([]byte(`[{"agent":"shell-docker","created_at":"2026-06-10T03:28:49Z","id":"sha256:0e27","repository":"docker.io/docker/sandbox-templates","tag":"shell-docker"}]`))
		case "/docker/images/inspect":
			require.Equal(t, "docker.io/docker/sandbox-templates:shell-docker", r.URL.Query().Get("name"))
			w.Write([]byte(`{"agent":"shell-docker","created_at":"2026-06-04T19:58:11Z","id":"sha256:0e27"}`))
		default:
			t.Fatalf("unexpected %s", r.URL.Path)
		}
	}))
	imgs, err := List(context.Background(), c)
	require.NoError(t, err)
	require.Len(t, imgs, 1)
	require.Equal(t, "shell-docker", imgs[0].Tag)
	require.Equal(t, "shell-docker", imgs[0].Agent)

	img, err := Inspect(context.Background(), c, "docker.io/docker/sandbox-templates:shell-docker")
	require.NoError(t, err)
	require.Equal(t, "sha256:0e27", img.ID)
}

func TestRemoveAndLoad(t *testing.T) {
	var loadBody []byte
	c := stubClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodDelete && r.URL.Path == "/docker/images/remove":
			require.Equal(t, "myimg:v1", r.URL.Query().Get("name"))
			w.WriteHeader(200)
		case r.Method == http.MethodPost && r.URL.Path == "/docker/images/load":
			loadBody, _ = io.ReadAll(r.Body)
			w.WriteHeader(200)
		default:
			t.Fatalf("unexpected %s %s", r.Method, r.URL.Path)
		}
	}))
	require.NoError(t, Remove(context.Background(), c, "myimg:v1"))
	require.NoError(t, Load(context.Background(), c, strings.NewReader("TARDATA")))
	require.Equal(t, "TARDATA", string(loadBody))
}
