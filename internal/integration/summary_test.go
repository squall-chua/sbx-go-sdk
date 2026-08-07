//go:build integration

package integration

import (
	"context"
	"testing"

	"github.com/squall-chua/sbx-go-sdk/client"
	"github.com/squall-chua/sbx-go-sdk/policy"
	"github.com/squall-chua/sbx-go-sdk/sandbox"
	"github.com/squall-chua/sbx-go-sdk/secret"
	"github.com/stretchr/testify/require"
)

// Summary is the only place the SDK surfaces auth mode, injected secrets,
// session count and MCP-gateway state, so this checks them against a real
// sandbox carrying a published port and a sandbox-scoped custom secret.
func TestSmoke_SandboxSummary(t *testing.T) {
	ctx := context.Background()
	c, err := client.New(ctx, client.WithAutoStart())
	require.NoError(t, err)

	sb, err := sandbox.Create(ctx, c,
		sandbox.WithAgent("shell"),
		sandbox.WithWorkspace(t.TempDir()),
		sandbox.WithPublish("8080"),
	)
	require.NoError(t, err)
	t.Cleanup(func() { _ = sb.Remove(ctx, sandbox.WithForce()) })

	require.NoError(t, secret.SetCustom(ctx, c, sb.Name(), secret.CustomSecret{
		Host:  "api.summary.test",
		Env:   "SUMMARY_PROBE_KEY",
		Value: "not-a-real-secret",
	}))
	t.Cleanup(func() { _ = secret.RemoveCustom(ctx, c, sb.Name(), "api.summary.test") })

	s, err := sb.Summary(ctx)
	require.NoError(t, err)

	require.Equal(t, sb.Name(), s.Name)
	require.Equal(t, "shell", s.Agent)
	require.NotEmpty(t, s.State)
	require.NotEmpty(t, s.Image)
	require.NotEmpty(t, s.DaemonVersion)
	require.Equal(t, 0, s.Sessions, "nothing is attached to this sandbox")
	require.NotNil(t, s.NetworkPolicy, "a sandbox always has a policy in force")
	require.NotEmpty(t, s.Ports, "the create-time publish must show up")

	var names []string
	for _, sec := range s.Secrets {
		names = append(names, sec.Name)
	}
	require.Contains(t, names, "SUMMARY_PROBE_KEY",
		"a sandbox-scoped custom secret must appear in the summary")
}

// The CLI-side install checker. It must parse, and on a working host nothing
// should fail — warnings (e.g. no internet to check for CLI updates) are fine.
func TestSmoke_Diagnose(t *testing.T) {
	ctx := context.Background()
	c, err := client.New(ctx, client.WithAutoStart())
	require.NoError(t, err)

	d, err := c.Diagnose(ctx)
	require.NoError(t, err)
	require.NotEmpty(t, d.Version)
	require.NotEmpty(t, d.Checks)
	require.Equal(t, len(d.Checks), d.Summary.Pass+d.Summary.Warn+d.Summary.Fail+d.Summary.Skip,
		"the summary must account for every check")
	for _, ch := range d.Checks {
		require.NotEmpty(t, ch.Name)
		require.Contains(t, []string{"pass", "warn", "fail", "skip"}, ch.Status)
	}
	require.True(t, d.OK(), "this host runs the rest of the suite, so nothing should fail")
}

// `sbx setup` degrades to a read-only detection pass with no terminal. Parsing
// it is the only thing verified here — the wizard half is never triggered,
// because the SDK hands the child an empty stdin.
func TestSmoke_DetectSetup(t *testing.T) {
	ctx := context.Background()
	c, err := client.New(ctx, client.WithAutoStart())
	require.NoError(t, err)

	rep, err := c.DetectSetup(ctx)
	require.NoError(t, err)
	require.NotEmpty(t, rep.Items)
	for _, it := range rep.Items {
		require.NotEmpty(t, it.Section, "every row belongs to a section")
		require.NotEmpty(t, it.Name)
	}
	require.NotEmpty(t, rep.Section("PREREQUISITES"), "the host always reports prerequisites")
}

// An ungoverned host has no profiles; the call must still succeed and decode.
func TestSmoke_PolicyProfiles(t *testing.T) {
	ctx := context.Background()
	c, err := client.New(ctx, client.WithAutoStart())
	require.NoError(t, err)

	got, err := policy.ProfileNames(ctx, c)
	require.NoError(t, err)
	for _, p := range got {
		require.NotEmpty(t, p, "a profile is named, never blank")
	}

	raw, err := policy.Profiles(ctx, c)
	require.NoError(t, err)
	require.NotEmpty(t, raw, "the CLI prints prose even with no profiles")
}
