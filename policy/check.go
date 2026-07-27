package policy

import (
	"context"
	"errors"
	"net/http"

	"github.com/squall-chua/sbx-go-sdk/client"
)

// Authorization is the outcome of evaluating the whole policy against one access
// request (REST POST /policy/network/check, matching `sbx policy check network`).
//
// This is distinct from a PolicyRule's Decision: a rule's decision is one rule's
// own allow/deny, whereas an Authorization is derived from every rule that
// applies. See CONTEXT.md.
type Authorization struct {
	Allowed bool `json:"allowed"`

	// Action is the evaluated operation, e.g. "net:connect:tcp".
	Action string `json:"action"`
	// Context is "global" or "sandbox:<id>".
	Context string `json:"context"`
	// DenyKind is "implicit" when nothing matched and the default deny applied.
	// Empty when allowed, or when an explicit deny rule matched.
	DenyKind string `json:"deny_kind"`
	// Reason is the daemon's explanation for a denial.
	Reason string `json:"reason"`
	// ResourceType is the resource class, e.g. "net:domain".
	ResourceType string `json:"resource_type"`
	// ResourceValue is the normalised resource, e.g. "api.example.com:443".
	ResourceValue string `json:"resource_value"`
	// Rule names the deciding rule when there is one.
	Rule string `json:"rule"`
	// Origin is where the deciding rule came from: "local", "org", or "kit".
	Origin string `json:"origin"`
	// Target echoes the request target.
	Target string `json:"target"`
	// Type is the request type, always "network" for now.
	Type string `json:"type"`

	Governance GovernanceStatus `json:"governance"`
}

// GovernanceStatus describes remote-governance state at evaluation time.
type GovernanceStatus struct {
	Active                  bool   `json:"active"`
	Organization            string `json:"organization"`
	OrganizationUnavailable bool   `json:"organization_unavailable"`
	LastSyncedStatus        string `json:"last_synced_status"`
	LastSyncedMessage       string `json:"last_synced_message"`
}

type checkConfig struct{ sandbox string }

// CheckOption configures Check.
type CheckOption func(*checkConfig)

// WithCheckSandbox evaluates the request in a sandbox's policy context rather
// than globally. The daemon rejects an unknown sandbox name.
func WithCheckSandbox(name string) CheckOption {
	return func(c *checkConfig) { c.sandbox = name }
}

// checkRequest is the POST body. sandbox_id is omitted when empty.
type checkRequest struct {
	Type      string `json:"type"`
	Target    string `json:"target"`
	SandboxID string `json:"sandbox_id,omitempty"`
}

// Check asks the daemon whether the current policy would authorize network
// access to target, without making the connection.
//
// target may be a hostname, host:port, IP literal, or URL; the daemon evaluates
// a bare host at port 443. Pass WithCheckSandbox to evaluate in a sandbox's
// context instead of globally.
func Check(ctx context.Context, c *client.Client, target string, opts ...CheckOption) (*Authorization, error) {
	if target == "" {
		return nil, errors.New("policy check: target must not be empty")
	}
	var cfg checkConfig
	for _, o := range opts {
		o(&cfg)
	}
	body := checkRequest{Type: "network", Target: target, SandboxID: cfg.sandbox}

	var auth Authorization
	if err := c.Transport().DoJSON(ctx, http.MethodPost, "/policy/network/check", body, &auth); err != nil {
		return nil, client.MapError("policy-check", err)
	}
	return &auth, nil
}
