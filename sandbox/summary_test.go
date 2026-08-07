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

// clientPrintingSbx is clientWithRecordingSbx plus a canned stdout.
func clientPrintingSbx(t *testing.T, argFile, out string) *client.Client {
	t.Helper()
	sock := filepath.Join(t.TempDir(), "d.sock")
	l, err := net.Listen("unix", sock)
	require.NoError(t, err)
	srv := &http.Server{Handler: http.NewServeMux()}
	go srv.Serve(l)
	t.Cleanup(func() { srv.Close() })

	bin := filepath.Join(t.TempDir(), "sbx")
	script := "#!/bin/sh\nprintf '%s\\n' \"$*\" >> " + argFile + "\ncat <<'SBXEOF'\n" + out + "\nSBXEOF\n"
	require.NoError(t, os.WriteFile(bin, []byte(script), 0o755))

	c, err := client.New(context.Background(), client.WithSocketPath(sock), client.WithBinaryPath(bin))
	require.NoError(t, err)
	return c
}

// Captured verbatim from `sbx inspect --json` at sbx v0.38.0, against a sandbox
// carrying a kit, a published port and a sandbox-scoped custom secret.
const summaryJSON = `{
  "name": "sdk-inspect-probe",
  "agent": "shell",
  "kits": ["/home/me/fixture-kit"],
  "state": "running",
  "uptime": "5s",
  "image": "sandboxes-swap/sdk-inspect-probe:bd3d07ba",
  "image_digest": "sha256:bb5d62dc0397eeae54eb30999c5014ecc979b6514cc054cb7a92ca6fd6f20303",
  "workspace": "/tmp/sdkws",
  "network": "sdk-inspect-probe",
  "network_policy": {"scope": "global"},
  "proxy": "172.17.0.0:3128",
  "secrets": [
    {"name": "PROBE_KEY", "source": "custom"},
    {"name": "mcpgateway", "source": "uploaded"}
  ],
  "mcp_gateway": true,
  "ports": ["127.0.0.1:44624->8080/tcp", "::1:44624->8080/tcp"],
  "sessions": 0,
  "daemon_version": "v0.38.0",
  "daemon_uptime": "40m"
}`

func TestSummary(t *testing.T) {
	argFile := filepath.Join(t.TempDir(), "args.txt")
	c := clientPrintingSbx(t, argFile, summaryJSON)
	sb := NewForTest(c, "sdk-inspect-probe")

	got, err := sb.Summary(context.Background())
	require.NoError(t, err)

	require.Equal(t, "sdk-inspect-probe", got.Name)
	require.Equal(t, "shell", got.Agent)
	require.Equal(t, "running", got.State)
	require.Equal(t, []string{"/home/me/fixture-kit"}, got.Kits)

	// The four fields that exist nowhere else in the SDK.
	require.Equal(t, 0, got.Sessions)
	require.True(t, got.MCPGateway)
	require.Empty(t, got.AuthMode, "the CLI omits auth_mode when it is not configured")
	require.Equal(t, []SecretRef{
		{Name: "PROBE_KEY", Source: "custom"},
		{Name: "mcpgateway", Source: "uploaded"},
	}, got.Secrets)

	require.NotNil(t, got.NetworkPolicy)
	require.Equal(t, "global", got.NetworkPolicy.Scope)
	require.Len(t, got.Ports, 2, "a loopback publish creates one key per address family")

	args, err := os.ReadFile(argFile)
	require.NoError(t, err)
	require.Contains(t, string(args), "inspect --json sdk-inspect-probe")
}

// The CLI omits kits, ports, secrets and mount_policy_denied on a bare sandbox.
// Their absence must decode as zero values, not fail.
func TestSummary_OmittedFieldsDecodeAsZero(t *testing.T) {
	const bare = `{"name":"plain","agent":"shell","state":"stopped","workspace":"/ws",
	  "daemon_version":"v0.38.0"}`
	c := clientPrintingSbx(t, filepath.Join(t.TempDir(), "args.txt"), bare)
	sb := NewForTest(c, "plain")

	got, err := sb.Summary(context.Background())
	require.NoError(t, err)
	require.Empty(t, got.Kits)
	require.Empty(t, got.Ports)
	require.Empty(t, got.Secrets)
	require.False(t, got.MountPolicyDenied)
	require.False(t, got.MCPGateway)
	require.Nil(t, got.NetworkPolicy)
}

func TestSummary_ReportsFormatDrift(t *testing.T) {
	c := clientPrintingSbx(t, filepath.Join(t.TempDir(), "args.txt"), "Name: not-json")
	sb := NewForTest(c, "x")

	_, err := sb.Summary(context.Background())
	require.ErrorIs(t, err, client.ErrUnexpectedFormat)
}
