// Package ssh manages the sandboxd SSH endpoint (EXPERIMENTAL upstream). It composes
// over the settings package for the feature flag (feature.ssh) and shells out to
// `sbx ssh setup` for local client provisioning. Enabling requires experimental
// features (platform.allowExperimentalFeatures, default true); this package does
// not modify that host-wide setting.
//
// In sbx v0.35.0 the connection model changed: the sandbox name is the SSH
// hostname ("<name>.sbx"), routed through a single wildcard "Host *.sbx" block that
// `sbx ssh setup` writes. There is no loopback port or client key — auth is the
// daemon's unix-socket OS boundary plus an active Docker login. The old ssh.port
// setting and loopback model are gone.
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

// TargetFor builds connection info for a sandbox. It is a pure builder — no
// existence check. With ssh.autoCreate (default true) the sandbox is created on
// connect, so a target for any name is valid.
func TargetFor(sandboxName string) Target {
	return Target{Host: sandboxName + hostSuffix}
}

// Enable turns on the SSH endpoint (settings set feature.ssh true). Fire-and-forget:
// the daemon hot-reloads within ~5s. Requires platform.allowExperimentalFeatures
// (default true), which Enable does not modify.
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
