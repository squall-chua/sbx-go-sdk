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
