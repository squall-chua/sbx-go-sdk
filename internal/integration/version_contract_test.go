//go:build integration

package integration

import (
	"context"
	"os"
	"testing"

	"github.com/squall-chua/sbx-go-sdk/client"
	"github.com/stretchr/testify/require"
)

// TestContract_VersionAlignment guards the version coupling between this SDK and
// the live sbx/sandboxd it talks to. The SDK's REST wire structs are generated
// from a specific sbx binary's DWARF and its shell-out flags target versioned CLI
// behaviour, so both are pinned to a tested range (client.ClientVersion /
// client.TestedAPIVersion). This test detects when an installed daemon has drifted
// from that pin so a maintainer can re-sync — see the README "Version alignment"
// section for the playbook.
//
// On drift it fails by default so CI surfaces the upgrade. Set
// SBX_ALLOW_VERSION_DRIFT=1 to downgrade drift to a warning (e.g. while
// intentionally validating against a newer daemon mid-upgrade).
func TestContract_VersionAlignment(t *testing.T) {
	ctx := context.Background()
	c, err := client.New(ctx, client.WithAutoStart())
	require.NoError(t, err)

	report := func(format string, args ...any) {
		if os.Getenv("SBX_ALLOW_VERSION_DRIFT") != "" {
			t.Logf("version drift (tolerated): "+format, args...)
			return
		}
		t.Errorf("version drift: "+format+
			"\n  -> re-run `go run ./internal/tools/dwarfgen -bin $(which sbx)`, review the"+
			" internal/api/types_gen.go diff, run the integration suite, then bump"+
			" client.ClientVersion / client.TestedAPIVersion."+
			"\n  -> set SBX_ALLOW_VERSION_DRIFT=1 to tolerate this while upgrading.", args...)
	}

	// DaemonHealth.Version / APIVersion are the authoritative drift signals: they are
	// the strings the SDK's wire types and shell-out flags were pinned against.
	dh, err := c.DaemonHealth(ctx)
	require.NoError(t, err)

	if dh.Version != client.ClientVersion {
		report("daemon version %q != tested client.ClientVersion %q", dh.Version, client.ClientVersion)
	}
	if dh.APIVersion != client.TestedAPIVersion {
		report("daemon api_version %q != tested client.TestedAPIVersion %q", dh.APIVersion, client.TestedAPIVersion)
	}

	// The daemon's own POST /version verdict is informational only — it is NOT a
	// reliable drift signal. Non-release daemons (DaemonHealth.Release == false)
	// have been observed returning "incompatible" for every client string, including
	// their own exact version. WithStrictVersion() rejects such daemons; this is why
	// the SDK's default is lenient. Log it for context but never fail on it.
	if res, err := c.CheckVersion(ctx); err == nil {
		t.Logf("daemon POST /version verdict for client %q: %q (informational; release=%t)",
			client.ClientVersion, res, dh.Release)
	}
}
