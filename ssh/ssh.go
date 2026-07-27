// Package ssh manages the sandboxd SSH endpoint (EXPERIMENTAL upstream). It
// composes over the settings package for the feature flag (feature.ssh) and
// shells out to `sbx setup ssh` for local client provisioning.
//
// As of sbx v0.37.0 SSH is a documented feature and feature.ssh defaults to
// enabled, so Enable is usually unnecessary; it remains for hosts where the
// flag was explicitly turned off. Enabling also requires experimental features
// (platform.allowExperimentalFeatures, default true), which this package does
// not modify.
//
// Connection model: the sandbox name is the SSH hostname ("<name>.sbx"), routed
// through a single wildcard "Host *.sbx" block. There is no loopback port and
// no client key — authentication is the daemon's unix-socket OS boundary plus
// an active Docker login. Connecting starts the daemon and the sandbox on
// demand.
//
// The ssh.acceptEnv setting allowlists which client environment variables are
// forwarded into the sandbox; read it with settings.Get. It is security
// relevant — it defaults to a list including ANTHROPIC_API_KEY, OPENAI_API_KEY
// and GITHUB_TOKEN.
package ssh

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/squall-chua/sbx-go-sdk/client"
	"github.com/squall-chua/sbx-go-sdk/settings"
)

const featureKey = "feature.ssh"

// hostSuffix is the ".sbx" suffix `sbx ssh setup` routes by default (Host *.sbx).
// ponytail: hardcoded to the setup default; TargetFor takes no alias override.
const hostSuffix = ".sbx"

// Target is the SSH connection info for a sandbox: the sandbox name plus the
// ".sbx" suffix that `sbx ssh setup` routes through the daemon.
type Target struct {
	Host string // "<sandbox-name>.sbx"
}

// Args returns the ssh client arguments, e.g. ["mybox.sbx"], suitable for
// exec.Command("ssh", t.Args()...).
func (t Target) Args() []string {
	return []string{t.Host}
}

// Command returns the display form, e.g. "ssh mybox.sbx".
func (t Target) Command() string {
	return "ssh " + t.Host
}

// TargetFor builds connection info for a sandbox. It is a pure builder — it
// performs no existence check.
//
// Connecting starts both the daemon and the target sandbox if they are not
// already running. The sandbox must exist, unless ssh.autoCreate is enabled —
// it defaults to false, so a target for an unknown name fails to connect.
func TargetFor(sandboxName string) Target {
	return Target{Host: sandboxName + hostSuffix}
}

// Enable turns on the SSH endpoint (settings set feature.ssh true). Fire-and-forget:
// the daemon hot-reloads within ~5s. Requires platform.allowExperimentalFeatures
// (default true), which Enable does not modify.
//
// feature.ssh defaults to enabled as of sbx v0.37.0, so this is usually a no-op.
func Enable(ctx context.Context, c *client.Client) error {
	return settings.Set(ctx, c, featureKey, "true")
}

// Disable turns off the SSH endpoint (settings set feature.ssh false — explicit, so
// the result is deterministic regardless of the default).
func Disable(ctx context.Context, c *client.Client) error {
	return settings.Set(ctx, c, featureKey, "false")
}

// Enabled reports whether the SSH endpoint feature flag is on. feature.ssh is a
// structured flag; its value is {"enabled":bool,…}.
func Enabled(ctx context.Context, c *client.Client) (bool, error) {
	s, err := settings.Get(ctx, c, featureKey)
	if err != nil {
		return false, err
	}
	var f struct {
		Enabled bool `json:"enabled"`
	}
	if err := json.Unmarshal(s.Value, &f); err != nil {
		return false, fmt.Errorf("ssh: parse %s value %s: %w: %w", featureKey, s.Value, client.ErrUnexpectedFormat, err)
	}
	return f.Enabled, nil
}
