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

// fakeClient returns a client whose fake sbx binary records its args to argFile
// and prints stdout. argFile may be "" if the test does not inspect args.
func fakeClient(t *testing.T, argFile, stdout string) *client.Client {
	t.Helper()
	sock := filepath.Join(t.TempDir(), "d.sock")
	l, err := net.Listen("unix", sock)
	require.NoError(t, err)
	srv := &http.Server{Handler: http.NewServeMux()}
	go srv.Serve(l)
	t.Cleanup(func() { srv.Close() })
	if argFile == "" {
		argFile = filepath.Join(t.TempDir(), "args.txt")
	}
	bin := filepath.Join(t.TempDir(), "sbx")
	script := "#!/bin/sh\nprintf '%s\\n' \"$*\" >> " + argFile + "\ncat <<'EOF'\n" + stdout + "\nEOF\nexit 0\n"
	require.NoError(t, os.WriteFile(bin, []byte(script), 0o755))
	c, err := client.New(context.Background(), client.WithSocketPath(sock), client.WithBinaryPath(bin))
	require.NoError(t, err)
	return c
}

func recordingClient(t *testing.T, argFile string) *client.Client {
	return fakeClient(t, argFile, "")
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

func TestPolicyLog(t *testing.T) {
	sock := filepath.Join(t.TempDir(), "d.sock")
	l, err := net.Listen("unix", sock)
	require.NoError(t, err)
	srv := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/network/log", r.URL.Path)
		w.Write([]byte(`{"blocked_hosts":[],"allowed_hosts":[{"host":"api.github.com:443","vm_name":"s1","proxy_type":"forward","rule":"domain-allowed","last_seen":"2026-06-10T11:29:10Z","since":"2026-06-10T11:29:10Z","count_since":2}]}`))
	})}
	go srv.Serve(l)
	t.Cleanup(func() { srv.Close() })
	c, err := client.New(context.Background(), client.WithSocketPath(sock))
	require.NoError(t, err)

	logs, err := Log(context.Background(), c)
	require.NoError(t, err)
	require.Len(t, logs.AllowedHosts, 1)
	require.Equal(t, "api.github.com:443", logs.AllowedHosts[0].Host)
}

func TestListRawAndProfiles(t *testing.T) {
	argFile := filepath.Join(t.TempDir(), "args.txt")
	c := fakeClient(t, argFile, "POLICY-TEXT")
	ctx := context.Background()

	raw, err := ListRaw(ctx, c, "s1")
	require.NoError(t, err)
	require.Contains(t, raw, "POLICY-TEXT")
	data, _ := os.ReadFile(argFile)
	require.Contains(t, string(data), "policy ls s1")

	prof, err := Profiles(ctx, c)
	require.NoError(t, err)
	require.Contains(t, prof, "POLICY-TEXT")
}

func TestListJSON(t *testing.T) {
	json := `{"rules":[` +
		`{"id":"default-ai","name":"default-ai","policy_id":"local-policy","scope":"global","applies_to":"all","resource_type":"network","decision":"allow","resources":["a.example.com:443","b.example.com:443"],"origin":"local","status":"active","editable":true},` +
		`{"id":"block-bad","name":"block-bad","policy_id":"local-policy","scope":"global","applies_to":"web","resource_type":"network","decision":"deny","resources":["evil.example.com:443"],"origin":"local","status":"active","editable":true}` +
		`]}`
	argFile := filepath.Join(t.TempDir(), "args.txt")
	c := fakeClient(t, argFile, json)

	rules, err := List(context.Background(), c, "")
	require.NoError(t, err)
	require.Len(t, rules, 2)
	require.Equal(t, PolicyRule{
		ID: "default-ai", Name: "default-ai", PolicyID: "local-policy",
		Scope: "global", AppliesTo: "all", ResourceType: "network", Decision: "allow",
		Resources: []string{"a.example.com:443", "b.example.com:443"},
		Origin:    "local", Status: "active", Editable: true,
	}, rules[0])
	require.Equal(t, "deny", rules[1].Decision)
	require.Equal(t, []string{"evil.example.com:443"}, rules[1].Resources)

	data, _ := os.ReadFile(argFile)
	require.Contains(t, string(data), "policy ls --json")
}

func TestListJSON_Empty(t *testing.T) {
	c := fakeClient(t, "", `{"rules":[]}`)
	rules, err := List(context.Background(), c, "")
	require.NoError(t, err)
	require.Empty(t, rules)
}

func TestListJSON_Drift(t *testing.T) {
	c := fakeClient(t, "", "not json at all")
	_, err := List(context.Background(), c, "")
	require.ErrorIs(t, err, client.ErrUnexpectedFormat)
}
