// Package sandbox manages sbx sandboxes: provisioning (shell-out) and lifecycle
// (REST), following the docker/go-sdk functional-options idiom.
package sandbox

import (
	"context"
	"net/http"

	"github.com/squall-chua/sbx-go-sdk/client"
	"github.com/squall-chua/sbx-go-sdk/internal/api"
)

// StatusRunning is the daemon's running-status string, as returned by the
// /sandbox and /sandbox/{name} endpoints (verified live against sandboxd v0.33.0).
const StatusRunning = "running"

// Info is the daemon's sandbox description returned by Inspect. It is an alias for
// the generated internal type so external importers (which cannot import
// internal/api) can name Inspect's return value.
type Info = api.SandboxInfo

// Sandbox is a handle to a single sandbox, addressed by name.
type Sandbox struct {
	cli  *client.Client
	info api.SandboxInfo
}

func newSandbox(c *client.Client, info api.SandboxInfo) *Sandbox {
	return &Sandbox{cli: c, info: info}
}

// Name returns the sandbox name (the primary identifier).
func (s *Sandbox) Name() string { return s.info.Name }

// ID returns the daemon's stable per-sandbox UUID (added in sbx v0.33.0). Empty
// for handles built without a daemon round-trip (NewForTest).
func (s *Sandbox) ID() string { return s.info.Id }

// MountPolicyDenied reports whether the daemon denied a workspace mount for this
// sandbox by policy (sbx v0.33.0). False when the daemon omits the field.
func (s *Sandbox) MountPolicyDenied() bool {
	return s.info.MountPolicyDenied != nil && *s.info.MountPolicyDenied
}

// Agent returns the agent the sandbox was created for ("" if unset).
func (s *Sandbox) Agent() string {
	if s.info.Agent == nil {
		return ""
	}
	return *s.info.Agent
}

// State returns the last-known status string.
func (s *Sandbox) State() string { return statusString(s.info) }

// IsRunning reports whether the last-known status is running.
func (s *Sandbox) IsRunning() bool { return statusString(s.info) == StatusRunning }

// Inspect refreshes and returns the sandbox info.
func (s *Sandbox) Inspect(ctx context.Context) (Info, error) {
	var info Info
	err := s.cli.Transport().DoJSON(ctx, http.MethodGet, "/sandbox/"+s.info.Name, nil, &info)
	if err != nil {
		return Info{}, client.MapError("inspect", err)
	}
	s.info = info
	return info, nil
}

// NewForTest builds a Sandbox handle around a client and name without a daemon
// round-trip. For tests only.
func NewForTest(c *client.Client, name string) *Sandbox {
	return &Sandbox{cli: c, info: api.SandboxInfo{Name: name}}
}

// Client returns the underlying client (used by the exec package).
func (s *Sandbox) Client() *client.Client { return s.cli }
