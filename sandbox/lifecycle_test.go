package sandbox

import (
	"context"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"github.com/squall-chua/sbx-go-sdk/client"
	"github.com/stretchr/testify/require"
)

func clientWithFakeSbx(t *testing.T, h http.Handler, sbxBody string) *client.Client {
	t.Helper()
	sock := filepath.Join(t.TempDir(), "d.sock")
	l, err := net.Listen("unix", sock)
	require.NoError(t, err)
	srv := &http.Server{Handler: h}
	go srv.Serve(l)
	t.Cleanup(func() { srv.Close() })
	bin := filepath.Join(t.TempDir(), "sbx")
	require.NoError(t, os.WriteFile(bin, []byte("#!/bin/sh\n"+sbxBody), 0o755))
	c, err := client.New(context.Background(), client.WithSocketPath(sock), client.WithBinaryPath(bin))
	require.NoError(t, err)
	return c
}

func TestCreate_OwnsNameAndHydrates(t *testing.T) {
	var sawCreate bool
	c := clientWithFakeSbx(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/sandbox": // List for collision check
			w.Write([]byte(`[]`))
		case "/sandbox/claude-myws":
			w.Write([]byte(`{"name":"claude-myws","agent":"claude","status":"running"}`))
		default:
			t.Fatalf("unexpected %s", r.URL.Path)
		}
	}), `echo "args: $*"; case "$*" in *"--name claude-myws"*) exit 0;; esac; exit 0`)

	ws := filepath.Join(t.TempDir(), "myws")
	require.NoError(t, os.Mkdir(ws, 0o755))
	sb, err := Create(context.Background(), c, WithAgent("claude"), WithWorkspace(ws))
	require.NoError(t, err)
	require.Equal(t, "claude-myws", sb.Name())
	require.True(t, sb.IsRunning())
	_ = sawCreate
}
