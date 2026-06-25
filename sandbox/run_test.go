package sandbox

import (
	"bytes"
	"context"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"github.com/squall-chua/sbx-go-sdk/client"
	"github.com/stretchr/testify/require"
)

func TestDefinition_ToRunArgs(t *testing.T) {
	d := newDefinition(
		WithAgent("claude"),
		WithWorkspace("/abs/ws"),
		WithCPUs(4),
		WithAgentArgs("--model", "opus"),
	)
	args, err := d.toRunArgs()
	require.NoError(t, err)
	require.Equal(t, []string{
		"run", "claude", "/abs/ws", "--cpus", "4", "--", "--model", "opus",
	}, args)
}

func TestDefinition_ToRunArgs_NoAgentArgs(t *testing.T) {
	d := newDefinition(WithAgent("shell"), WithWorkspace("/w"))
	args, err := d.toRunArgs()
	require.NoError(t, err)
	require.Equal(t, []string{"run", "shell", "/w"}, args)
}

func TestDefinition_ToRunArgs_Name(t *testing.T) {
	d := newDefinition(WithAgent("claude"), WithWorkspace("/w"), WithName("proj"))
	args, err := d.toRunArgs()
	require.NoError(t, err)
	require.Equal(t, []string{"run", "claude", "/w", "--name", "proj"}, args)
}

func TestSandboxRun_ReattachesByName(t *testing.T) {
	// Re-attach must use `run --name <name>`; the positional form is deprecated in v0.33.0.
	argFile := filepath.Join(t.TempDir(), "args.txt")
	c := clientWithRecordingSbx(t, argFile)
	sb := NewForTest(c, "s1")

	code, err := sb.Run(context.Background(), WithStdio(nil, nil, nil))
	require.NoError(t, err)
	require.Equal(t, 0, code)

	data, _ := os.ReadFile(argFile)
	require.Contains(t, string(data), "run --name s1")
}

func TestRun_Package_InheritsExitCode(t *testing.T) {
	// fake sbx echoes the run args and exits 5; stub daemon needed for client.New.
	sock := filepath.Join(t.TempDir(), "d.sock")
	l, err := net.Listen("unix", sock)
	require.NoError(t, err)
	srv := &http.Server{Handler: http.NewServeMux()}
	go srv.Serve(l)
	t.Cleanup(func() { srv.Close() })
	bin := filepath.Join(t.TempDir(), "sbx")
	require.NoError(t, os.WriteFile(bin, []byte("#!/bin/sh\necho \"ran: $*\"; exit 5\n"), 0o755))
	c, err := client.New(context.Background(), client.WithSocketPath(sock), client.WithBinaryPath(bin))
	require.NoError(t, err)

	ws := filepath.Join(t.TempDir(), "wsr")
	require.NoError(t, os.Mkdir(ws, 0o755))
	var out bytes.Buffer
	code, err := Run(context.Background(), c,
		WithAgent("shell"), WithWorkspace(ws), WithStdio(nil, &out, &out))
	require.NoError(t, err)
	require.Equal(t, 5, code)
	require.Contains(t, out.String(), "ran: run shell")
}
