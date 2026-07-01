// Package ssh manages the sandboxd SSH endpoint (EXPERIMENTAL upstream). It composes
// over the settings package for the feature flag (feature.ssh) and port (ssh.port),
// and shells out to `sbx ssh setup` for local client provisioning. Enabling requires
// experimental features (platform.allowExperimentalFeatures, default true); this
// package does not modify that host-wide setting.
package ssh

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"

	"github.com/squall-chua/sbx-go-sdk/client"
	"github.com/squall-chua/sbx-go-sdk/settings"
)

const (
	featureKey = "feature.ssh"
	portKey    = "ssh.port"
)

// Port returns the configured sandboxd SSH loopback port (ssh.port, default 2222).
func Port(ctx context.Context, c *client.Client) (int, error) {
	s, err := settings.Get(ctx, c, portKey)
	if err != nil {
		return 0, err
	}
	var p int
	if err := json.Unmarshal(s.Value, &p); err != nil {
		return 0, fmt.Errorf("ssh: parse %s value %s: %w: %w", portKey, s.Value, client.ErrUnexpectedFormat, err)
	}
	return p, nil
}

// Target is the SSH connection info for a sandbox. The sandbox name is the SSH
// username; the host is always loopback.
type Target struct {
	User string // sandbox name
	Host string // "127.0.0.1"
	Port int    // ssh.port
}

// Args returns the ssh client arguments, e.g. ["mybox@127.0.0.1", "-p", "2222"],
// suitable for exec.Command("ssh", t.Args()...).
func (t Target) Args() []string {
	return []string{t.User + "@" + t.Host, "-p", strconv.Itoa(t.Port)}
}

// Command returns the display form, e.g. "ssh mybox@127.0.0.1 -p 2222".
func (t Target) Command() string {
	return "ssh " + t.User + "@" + t.Host + " -p " + strconv.Itoa(t.Port)
}

// TargetFor builds connection info for a sandbox. It is a pure builder — no
// existence check. With ssh.autoCreate (default true) the sandbox is created on
// connect, so a target for any name is valid.
func TargetFor(ctx context.Context, c *client.Client, sandboxName string) (Target, error) {
	port, err := Port(ctx, c)
	if err != nil {
		return Target{}, err
	}
	return Target{User: sandboxName, Host: "127.0.0.1", Port: port}, nil
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
