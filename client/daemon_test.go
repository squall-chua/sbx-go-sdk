package client

import (
	"context"
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestHealth(t *testing.T) {
	sock := stub(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/health", r.URL.Path)
		w.Write([]byte(`{"release":false,"status":"healthy","version":"v0.32.0 abc"}`))
	}))
	c, _ := New(context.Background(), WithSocketPath(sock))
	h, err := c.Health(context.Background())
	require.NoError(t, err)
	require.Equal(t, "healthy", h.Status)
	require.Equal(t, "v0.32.0 abc", h.Version)
}

func TestCheckVersion(t *testing.T) {
	sock := stub(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, http.MethodPost, r.Method)
		require.Equal(t, "/version", r.URL.Path)
		w.Write([]byte(`{"result":"compatible"}`))
	}))
	c, _ := New(context.Background(), WithSocketPath(sock))
	res, err := c.CheckVersion(context.Background())
	require.NoError(t, err)
	require.Equal(t, "compatible", res)
}

func TestDaemonInfoAndLogLevels(t *testing.T) {
	sock := stub(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/daemon/info":
			w.Write([]byte(`{"api_socket":"/a.sock","docker_socket":"/d.sock"}`))
		case "/daemon/loglevel":
			w.Write([]byte(`{"general":"info","proxy":"info"}`))
		default:
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
	}))
	c, _ := New(context.Background(), WithSocketPath(sock))
	info, err := c.Info(context.Background())
	require.NoError(t, err)
	require.Equal(t, "/d.sock", *info.DockerSocket)
	ll, err := c.LogLevels(context.Background())
	require.NoError(t, err)
	require.Equal(t, "info", ll.Proxy)
}

func TestStopAndReset(t *testing.T) {
	var paths []string
	sock := stub(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.Method+" "+r.URL.Path)
		w.WriteHeader(200)
	}))
	c, _ := New(context.Background(), WithSocketPath(sock))
	require.NoError(t, c.StopDaemon(context.Background()))
	require.NoError(t, c.Reset(context.Background()))
	require.Equal(t, []string{"POST /daemon/shutdown", "POST /daemon/reset"}, paths)
}

func TestDaemonHealthAndDiagnostics(t *testing.T) {
	sock := stub(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/daemon/health":
			w.Write([]byte(`{"api_version":"0.10.0","release":false,"revision":"abc","status":"healthy","version":"v0.32.0"}`))
		case "/daemon/diagnostics":
			w.Write([]byte(`{"info":{"State":{"Sandboxes":{"Total":0}}}}`))
		default:
			t.Fatalf("unexpected %s", r.URL.Path)
		}
	}))
	c, _ := New(context.Background(), WithSocketPath(sock))
	dh, err := c.DaemonHealth(context.Background())
	require.NoError(t, err)
	require.Equal(t, "0.10.0", dh.APIVersion)
	require.Equal(t, "healthy", dh.Status)

	diag, err := c.Diagnostics(context.Background())
	require.NoError(t, err)
	require.Contains(t, string(diag), "Sandboxes")
}

func TestDaemonStatus_Running(t *testing.T) {
	sock := stub(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"status":"healthy"}`))
	}))
	c, _ := New(context.Background(), WithSocketPath(sock))
	st, err := c.DaemonStatus(context.Background())
	require.NoError(t, err)
	require.True(t, st.Running)
	require.Equal(t, sock, st.Socket)
}

func TestDaemonStatus_Down(t *testing.T) {
	sock := stub(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	c, _ := New(context.Background(), WithSocketPath(sock))
	st, err := c.DaemonStatus(context.Background())
	require.NoError(t, err)
	require.False(t, st.Running)
	require.Equal(t, sock, st.Socket)
}

func TestEnsureRunning_AlreadyHealthy(t *testing.T) {
	sock := stub(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"status":"healthy"}`))
	}))
	// binary path points at a fake that would FAIL if called — proves we don't start.
	bin := filepath.Join(t.TempDir(), "sbx")
	os.WriteFile(bin, []byte("#!/bin/sh\nexit 1\n"), 0o755)
	c, _ := New(context.Background(), WithSocketPath(sock), WithBinaryPath(bin))
	require.NoError(t, c.EnsureRunning(context.Background()))
}
