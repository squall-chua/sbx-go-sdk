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

// stubPolicyClient returns a client whose daemon socket is served by h.
func stubPolicyClient(t *testing.T, h http.Handler) *client.Client {
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

// The daemon's own response shape is a bare list of names
// (sandboxapi.PolicyProfilesListResponse holds one []string).
func TestProfileNames_DecodesNames(t *testing.T) {
	var gotPath string
	c := stubPolicyClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		w.Write([]byte(`{"profiles":["balanced","locked-down"]}`))
	}))

	got, err := ProfileNames(context.Background(), c)
	require.NoError(t, err)
	require.Equal(t, []string{"balanced", "locked-down"}, got)
	require.Equal(t, "/policy/network/profiles", gotPath)
}

// An ungoverned host answers {"profiles":[]} — empty, not an error.
func TestProfileNames_UngovernedHostIsEmptyNotAnError(t *testing.T) {
	c := stubPolicyClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Write([]byte(`{"profiles":[]}`))
	}))

	got, err := ProfileNames(context.Background(), c)
	require.NoError(t, err)
	require.Empty(t, got)
}

func TestProfileNames_ReportsFormatDrift(t *testing.T) {
	c := stubPolicyClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Write([]byte(`{"profiles":"not-a-list"}`))
	}))

	_, err := ProfileNames(context.Background(), c)
	require.ErrorIs(t, err, client.ErrUnexpectedFormat)
}

func TestList_UsesRESTAndAlwaysRequestsAllTypes(t *testing.T) {
	var gotPath, gotType, gotSandbox string
	c := stubPolicyClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotType = r.URL.Query().Get("type")
		gotSandbox = r.URL.Query().Get("sandbox")
		w.Write([]byte(`{"rules":[
			{"id":"r1","name":"net","policy_id":"local-policy","scope":"global","applies_to":"all",
			 "resource_type":"network","decision":"allow","resources":["a:443"],
			 "origin":"local","status":"active","editable":true},
			{"id":"r2","name":"fsr","policy_id":"local-policy","scope":"global","applies_to":"all",
			 "resource_type":"filesystem:read","decision":"allow","resources":["/"],
			 "origin":"local","status":"active","editable":true}
		]}`))
	}))

	rules, err := List(context.Background(), c, "")
	require.NoError(t, err)

	require.Equal(t, "/policy/network/rules", gotPath)
	require.Equal(t, "all", gotType, "type=all is mandatory: without it filesystem rules vanish silently")
	require.Empty(t, gotSandbox)

	require.Len(t, rules, 2)
	require.Equal(t, PolicyRule{
		ID:           "r1",
		Name:         "net",
		PolicyID:     "local-policy",
		Scope:        "global",
		AppliesTo:    "all",
		ResourceType: "network",
		Decision:     "allow",
		Resources:    []string{"a:443"},
		Origin:       "local",
		Status:       "active",
		Editable:     true,
	}, rules[0], "every PolicyRule field must decode, not just ResourceType")
	require.Equal(t, "filesystem:read", rules[1].ResourceType,
		"filesystem rules must survive the migration")
}

func TestList_ScopeBecomesSandboxQueryParam(t *testing.T) {
	var gotSandbox, gotType string
	c := stubPolicyClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotSandbox = r.URL.Query().Get("sandbox")
		gotType = r.URL.Query().Get("type")
		w.Write([]byte(`{"rules":[]}`))
	}))

	rules, err := List(context.Background(), c, "my-sandbox")
	require.NoError(t, err)
	require.Equal(t, "my-sandbox", gotSandbox)
	require.Equal(t, "all", gotType)
	require.Empty(t, rules)
}

func TestList_MalformedJSONReturnsErrUnexpectedFormat(t *testing.T) {
	c := stubPolicyClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"rules":"not-an-array"}`))
	}))

	_, err := List(context.Background(), c, "")
	require.ErrorIs(t, err, client.ErrUnexpectedFormat)
}

func TestInspectRaw_PassesSelectorThrough(t *testing.T) {
	argFile := filepath.Join(t.TempDir(), "args.txt")
	c := fakeClient(t, argFile, "Policy: Developer access")

	out, err := InspectRaw(context.Background(), c, "Developer access")
	require.NoError(t, err)
	require.Contains(t, out, "Developer access")

	args, err := os.ReadFile(argFile)
	require.NoError(t, err)
	require.Contains(t, string(args), "policy inspect Developer access")
}

func TestInspectRaw_EmptySelectorIsRejected(t *testing.T) {
	c := recordingClient(t, filepath.Join(t.TempDir(), "args.txt"))
	_, err := InspectRaw(context.Background(), c, "")
	require.Error(t, err)
}
