package policy

import (
	"context"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
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
	require.NoError(t, RemoveRule(ctx, c, "mysandbox", "evil.example"))
	require.NoError(t, Reset(ctx, c))
	data, _ := os.ReadFile(argFile)
	lines := string(data)
	require.Contains(t, lines, "policy init balanced")
	require.Contains(t, lines, "policy allow network example.com api.github.com")
	require.Contains(t, lines, "policy deny network --sandbox mysandbox evil.example")
	require.Contains(t, lines, "policy rm network --sandbox mysandbox --resource evil.example")
	require.Contains(t, lines, "policy reset")
}

func TestPolicyListProfilesAndLog(t *testing.T) {
	// List/Profiles: capturing runner returns the fake sbx stdout.
	argFile := filepath.Join(t.TempDir(), "args.txt")
	// fake sbx prints a banner to stdout so ListRaw returns non-empty text.
	sock := filepath.Join(t.TempDir(), "d.sock")
	l, err := net.Listen("unix", sock)
	require.NoError(t, err)
	srv := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/network/log", r.URL.Path)
		w.Write([]byte(`{"blocked_hosts":[],"allowed_hosts":[{"host":"api.github.com:443","vm_name":"s1","proxy_type":"forward","rule":"domain-allowed","last_seen":"2026-06-10T11:29:10Z","since":"2026-06-10T11:29:10Z","count_since":2}]}`))
	})}
	go srv.Serve(l)
	t.Cleanup(func() { srv.Close() })
	bin := filepath.Join(t.TempDir(), "sbx")
	require.NoError(t, os.WriteFile(bin, []byte("#!/bin/sh\nprintf '%s\\n' \"$*\" >> "+argFile+"\necho POLICY-TEXT\nexit 0\n"), 0o755))
	c, err := client.New(context.Background(), client.WithSocketPath(sock), client.WithBinaryPath(bin))
	require.NoError(t, err)
	ctx := context.Background()

	raw, err := ListRaw(ctx, c, "s1")
	require.NoError(t, err)
	require.Contains(t, raw, "POLICY-TEXT")
	data, _ := os.ReadFile(argFile)
	require.Contains(t, string(data), "policy ls s1")

	// "POLICY-TEXT" has no recognizable header → empty rule list, no error.
	rules, err := List(ctx, c, "s1")
	require.NoError(t, err)
	require.Empty(t, rules)

	prof, err := Profiles(ctx, c)
	require.NoError(t, err)
	require.Contains(t, prof, "POLICY-TEXT")

	logs, err := Log(ctx, c)
	require.NoError(t, err)
	require.Len(t, logs.AllowedHosts, 1)
	require.Equal(t, "api.github.com:443", logs.AllowedHosts[0].Host)
}

func TestParsePolicyList(t *testing.T) {
	hdr := "PROVENANCE  APPLIES_TO  POLICY/RULE  TYPE     DECISION  RESOURCES"
	ri := strings.Index(hdr, "RESOURCES") // RESOURCES column offset (56)
	raw := "Starting sandboxd daemon...\n" +
		"Daemon started (PID: 17849, socket: /x/sandboxd.sock)\n" +
		hdr + "\n" +
		"local       all         default-ai   network  allow     a.example.com:443\n" +
		strings.Repeat(" ", ri) + "b.example.com:443\n" +
		"\n" +
		"local       web         block-bad    network  deny      evil.example.com:443\n"

	rules, err := parsePolicyList(raw)
	require.NoError(t, err)
	require.Len(t, rules, 2)

	require.Equal(t, PolicyRule{
		Provenance: "local",
		AppliesTo:  "all",
		Rule:       "default-ai",
		Type:       "network",
		Decision:   "allow",
		Resources:  []string{"a.example.com:443", "b.example.com:443"},
	}, rules[0])

	require.Equal(t, "block-bad", rules[1].Rule)
	require.Equal(t, "deny", rules[1].Decision)
	require.Equal(t, []string{"evil.example.com:443"}, rules[1].Resources)
}

func TestParsePolicyList_Empty(t *testing.T) {
	rules, err := parsePolicyList("No policies found.\n")
	require.NoError(t, err)
	require.Empty(t, rules)
}

func TestParsePolicyList_Drift(t *testing.T) {
	raw := "PROVENANCE  APPLIES_TO  RULE  TYPE  DECISION  RESOURCES\n" +
		"local       all         x     net   allow     a:443\n"
	_, err := parsePolicyList(raw)
	require.ErrorIs(t, err, client.ErrUnexpectedFormat)
}
