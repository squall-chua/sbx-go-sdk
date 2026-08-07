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
		require.Equal(t, "/daemon/health", r.URL.Path)
		w.Write([]byte(`{"release":false,"status":"healthy","version":"v0.35.0 abc","api_version":"0.22.0"}`))
	}))
	c, _ := New(context.Background(), WithSocketPath(sock))
	h, err := c.Health(context.Background())
	require.NoError(t, err)
	require.Equal(t, "healthy", h.Status)
	require.Equal(t, "v0.35.0 abc", h.Version)
}

// Trimmed from a live `sbx diagnose -o json` run at sbx v0.38.0. The "Binary
// version" warn is what a host with no reachable update endpoint reports — a
// warning must not make the whole diagnosis fail.
func TestDiagnose(t *testing.T) {
	const out = `{"version":"1.0","checks":[
	  {"name":"CLI binary","status":"pass","message":"found","detail":"/usr/bin/sbx","hint":""},
	  {"name":"Binary version","status":"warn","message":"could not check for updates","detail":"unexpected status code: 403","hint":""},
	  {"name":"Daemon","status":"pass","message":"healthy","detail":"version v0.38.0","hint":""}],
	  "summary":{"pass":2,"warn":1,"fail":0,"skip":0}}`

	dir := t.TempDir()
	bin := filepath.Join(dir, "sbx")
	script := "#!/bin/sh\nprintf '%s\\n' \"$*\" > " + filepath.Join(dir, "args.txt") + "\ncat <<'EOF'\n" + out + "\nEOF\n"
	require.NoError(t, os.WriteFile(bin, []byte(script), 0o755))

	c, err := New(context.Background(), WithBinaryPath(bin))
	require.NoError(t, err)

	d, err := c.Diagnose(context.Background())
	require.NoError(t, err)
	require.Equal(t, "1.0", d.Version)
	require.Len(t, d.Checks, 3)
	require.Equal(t, "warn", d.Checks[1].Status)
	require.Equal(t, "unexpected status code: 403", d.Checks[1].Detail)
	require.Equal(t, DiagnosticSummary{Pass: 2, Warn: 1}, d.Summary)
	require.True(t, d.OK(), "a warning is not a failure")

	args, err := os.ReadFile(filepath.Join(dir, "args.txt"))
	require.NoError(t, err)
	require.Contains(t, string(args), "diagnose -o json")
	require.NotContains(t, string(args), "--upload",
		"Diagnose must never ship the report to Docker support on its own")
}

// Login and Logout are never exercised against a live daemon: one would need
// real Docker credentials and the other would sign the user out and stop every
// running sandbox. The argument vector is the contract worth pinning.
func TestLogin_TokenGoesToStdinNotArgv(t *testing.T) {
	dir := t.TempDir()
	argFile, stdinFile := filepath.Join(dir, "args.txt"), filepath.Join(dir, "stdin.txt")
	bin := filepath.Join(dir, "sbx")
	script := "#!/bin/sh\nprintf '%s\\n' \"$*\" >> " + argFile + "\ncat > " + stdinFile + "\n"
	require.NoError(t, os.WriteFile(bin, []byte(script), 0o755))

	c, err := New(context.Background(), WithBinaryPath(bin))
	require.NoError(t, err)
	require.NoError(t, c.Login(context.Background(), "me", "dckr_pat_secret"))

	args, err := os.ReadFile(argFile)
	require.NoError(t, err)
	require.Contains(t, string(args), "login --username me --password-stdin")
	require.NotContains(t, string(args), "dckr_pat_secret",
		"the token must never appear in the argument vector")

	stdin, err := os.ReadFile(stdinFile)
	require.NoError(t, err)
	require.Equal(t, "dckr_pat_secret\n", string(stdin))
}

func TestLogin_RejectsEmptyInputBeforeShellOut(t *testing.T) {
	dir := t.TempDir()
	argFile := filepath.Join(dir, "args.txt")
	bin := filepath.Join(dir, "sbx")
	require.NoError(t, os.WriteFile(bin, []byte("#!/bin/sh\ntouch "+argFile+"\n"), 0o755))

	c, err := New(context.Background(), WithBinaryPath(bin))
	require.NoError(t, err)
	require.Error(t, c.Login(context.Background(), "", "tok"))
	require.Error(t, c.Login(context.Background(), "me", ""))
	require.NoFileExists(t, argFile, "no call should have reached the CLI")
}

func TestLogout_AlwaysSkipsTheConfirmationPrompt(t *testing.T) {
	dir := t.TempDir()
	argFile := filepath.Join(dir, "args.txt")
	bin := filepath.Join(dir, "sbx")
	script := "#!/bin/sh\nprintf '%s\\n' \"$*\" >> " + argFile + "\n"
	require.NoError(t, os.WriteFile(bin, []byte(script), 0o755))

	c, err := New(context.Background(), WithBinaryPath(bin))
	require.NoError(t, err)
	require.NoError(t, c.Logout(context.Background()))

	args, err := os.ReadFile(argFile)
	require.NoError(t, err)
	require.Contains(t, string(args), "logout --yes",
		"without --yes the prompt blocks on non-interactive stdin")
}

func TestDiagnose_FailCountDrivesOK(t *testing.T) {
	d := &Diagnosis{Summary: DiagnosticSummary{Pass: 1, Fail: 1}}
	require.False(t, d.OK())
}

// Captured verbatim from a v0.38.0 daemon on a host with no SaaS entitlement.
func TestMCPGatewayMode(t *testing.T) {
	sock := stub(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, http.MethodGet, r.Method)
		require.Equal(t, "/mcp/gateway-mode", r.URL.Path)
		w.Write([]byte(`{"decision":"local","gateway_url":"none","reason":"not entitled to the SaaS gateway → local"}`))
	}))
	c, _ := New(context.Background(), WithSocketPath(sock))
	m, err := c.MCPGatewayMode(context.Background())
	require.NoError(t, err)
	require.Equal(t, "local", m.Decision)
	require.Equal(t, "none", m.GatewayURL)
	require.Contains(t, m.Reason, "not entitled")
}

// CheckVersion is now informational only (see daemon.go) — it still wraps the
// dead /version endpoint, returning whatever the daemon reports verbatim.
func TestCheckVersion(t *testing.T) {
	sock := stub(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, http.MethodPost, r.Method)
		require.Equal(t, "/version", r.URL.Path)
		w.Write([]byte(`{"result":"incompatible"}`))
	}))
	c, _ := New(context.Background(), WithSocketPath(sock))
	res, err := c.CheckVersion(context.Background())
	require.NoError(t, err)
	require.Equal(t, "incompatible", res)
}

func TestCheckVersion_ReportsEndpointRemoval(t *testing.T) {
	// POST /version was removed in sbx v0.37.0 and answers 404 on every verb.
	sock := stub(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte(`{"message":"Not Found"}`))
	}))
	c, err := New(context.Background(), WithSocketPath(sock))
	require.NoError(t, err)

	_, err = c.CheckVersion(context.Background())
	require.Error(t, err)
	require.Contains(t, err.Error(), "v0.37.0",
		"the error should name the release that removed the endpoint")
}

// WithStrictVersion verifies compatibility via /daemon/health's api_version, NOT
// the dead /version endpoint. Matching TestedAPIVersion accepts the daemon.
func TestStrictVersion_Match(t *testing.T) {
	sock := stub(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/daemon/health", r.URL.Path)
		w.Write([]byte(`{"api_version":"` + TestedAPIVersion + `","status":"healthy","version":"v0.32.0"}`))
	}))
	c, err := New(context.Background(), WithSocketPath(sock), WithStrictVersion())
	require.NoError(t, err)
	require.NotNil(t, c)
}

// A drifted api_version fails New with ErrIncompatibleVersion.
func TestStrictVersion_Mismatch(t *testing.T) {
	sock := stub(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/daemon/health", r.URL.Path)
		w.Write([]byte(`{"api_version":"9.9.9","status":"healthy","version":"v9.9.9"}`))
	}))
	_, err := New(context.Background(), WithSocketPath(sock), WithStrictVersion())
	require.ErrorIs(t, err, ErrIncompatibleVersion)
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
