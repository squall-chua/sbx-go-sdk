package client

import (
	"context"
	"net/http"
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
