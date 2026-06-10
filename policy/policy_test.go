package policy

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

func recordingClient(t *testing.T, argFile string) *client.Client {
	t.Helper()
	sock := filepath.Join(t.TempDir(), "d.sock")
	l, err := net.Listen("unix", sock)
	require.NoError(t, err)
	srv := &http.Server{Handler: http.NewServeMux()}
	go srv.Serve(l)
	t.Cleanup(func() { srv.Close() })
	bin := filepath.Join(t.TempDir(), "sbx")
	require.NoError(t, os.WriteFile(bin, []byte("#!/bin/sh\nprintf '%s\\n' \"$*\" >> "+argFile+"\nexit 0\n"), 0o755))
	c, err := client.New(context.Background(), client.WithSocketPath(sock), client.WithBinaryPath(bin))
	require.NoError(t, err)
	return c
}

func TestPolicyMutations(t *testing.T) {
	argFile := filepath.Join(t.TempDir(), "args.txt")
	c := recordingClient(t, argFile)
	ctx := context.Background()
	require.NoError(t, SetDefault(ctx, c, "balanced"))
	require.NoError(t, Allow(ctx, c, "", "example.com", "api.github.com"))
	require.NoError(t, Deny(ctx, c, "mysandbox", "evil.example"))
	require.NoError(t, RemoveRule(ctx, c, "mysandbox"))
	require.NoError(t, Reset(ctx, c))
	data, _ := os.ReadFile(argFile)
	lines := string(data)
	require.Contains(t, lines, "policy set-default balanced")
	require.Contains(t, lines, "policy allow network example.com api.github.com")
	require.Contains(t, lines, "policy deny network --sandbox mysandbox evil.example")
	require.Contains(t, lines, "policy rm network --sandbox mysandbox")
	require.Contains(t, lines, "policy reset")
}
