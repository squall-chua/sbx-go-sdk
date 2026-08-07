package sandbox

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/squall-chua/sbx-go-sdk/client"
)

// Summary is the report `sbx inspect --json` prints: a wider view of a sandbox
// than the daemon's own /sandbox/{name} response.
//
// Four things live here and nowhere else in the SDK — the auth mode, the
// injected secrets, the count of active sessions, and whether the MCP gateway is
// configured — because api.SandboxInfo carries none of them. Everything else
// overlaps with Inspect, which is REST and cheaper.
//
// Assembled CLI-side, so this shells out. Fields the CLI omits when they are
// empty or trivially false decode as zero values; see each field.
type Summary struct {
	Name      string   `json:"name"`
	Agent     string   `json:"agent"`
	Kits      []string `json:"kits"`
	State     string   `json:"state"`  // "running", "stopped", …
	Uptime    string   `json:"uptime"` // human text, e.g. "38m"; empty when stopped
	Workspace string   `json:"workspace"`

	Image       string `json:"image"`
	ImageDigest string `json:"image_digest"`

	// AuthMode is the sandbox's agent-credential mode. Omitted by the CLI when
	// unset, where `sbx inspect` renders "not configured" and this is "".
	AuthMode string `json:"auth_mode"`

	Network       string         `json:"network"`
	NetworkPolicy *NetworkPolicy `json:"network_policy"`
	Proxy         string         `json:"proxy"`

	// MountPolicyDenied mirrors Sandbox.MountPolicyDenied. Omitted when false.
	MountPolicyDenied bool `json:"mount_policy_denied"`

	// Secrets lists what is injected into the sandbox, masked to name and origin.
	// A custom secret (secret.SetCustom) appears with Source "custom"; the
	// gateway's own credential appears as "mcpgateway"/"uploaded".
	Secrets []SecretRef `json:"secrets"`

	// MCPGateway reports whether an MCP gateway is configured for the sandbox.
	MCPGateway bool `json:"mcp_gateway"`

	// Ports holds the published ports as the CLI formats them, e.g.
	// "127.0.0.1:44624->8080/tcp". Use Sandbox.Ports for structured values.
	Ports []string `json:"ports"`

	// Sessions is the number of active sessions (an attached agent, an open SSH
	// connection). A non-zero count is what makes a plain Remove fail.
	Sessions int `json:"sessions"`

	DaemonVersion string `json:"daemon_version"`
	DaemonUptime  string `json:"daemon_uptime"`
}

// NetworkPolicy names the policy in force for a sandbox.
type NetworkPolicy struct {
	Scope string `json:"scope"` // "global" or the sandbox name

	// Organization is set only under remote governance;
	// OrganizationUnavailable reports that the org policy could not be reached.
	Organization            string `json:"organization"`
	OrganizationUnavailable bool   `json:"organization_unavailable"`
}

// SecretRef is one masked secret injected into a sandbox.
type SecretRef struct {
	Name   string `json:"name"`
	Source string `json:"source"` // e.g. "custom", "uploaded"
}

// Summary returns the `sbx inspect --json` report for the sandbox.
//
// Prefer Inspect for anything it already carries: it is a REST call against the
// daemon, while this shells out to assemble a CLI-side view. Reach for Summary
// when you need AuthMode, Secrets, Sessions or MCPGateway.
func (s *Sandbox) Summary(ctx context.Context) (*Summary, error) {
	r, err := s.cli.Runner()
	if err != nil {
		return nil, err
	}
	out, err := r.Capture(ctx, nil, "inspect", "--json", s.info.Name)
	if err != nil {
		return nil, err
	}
	var sum Summary
	if err := json.Unmarshal([]byte(out), &sum); err != nil {
		return nil, fmt.Errorf("inspect %q: %w: %w", s.info.Name, client.ErrUnexpectedFormat, err)
	}
	return &sum, nil
}
