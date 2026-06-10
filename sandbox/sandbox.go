// Package sandbox manages sbx sandboxes: provisioning (shell-out) and lifecycle
// (REST), following the docker/go-sdk functional-options idiom.
package sandbox

import (
	"context"
	"net/http"

	"github.com/squall-chua/sbx-go-sdk/client"
	"github.com/squall-chua/sbx-go-sdk/internal/api"
)

// StatusRunning is the daemon's running-status string.
const StatusRunning = "SANDBOX_STATUS_RUNNING"

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
func (s *Sandbox) Inspect(ctx context.Context) (api.SandboxInfo, error) {
	var info api.SandboxInfo
	err := s.cli.Transport().DoJSON(ctx, http.MethodGet, "/sandbox/"+s.info.Name, nil, &info)
	if err != nil {
		return api.SandboxInfo{}, client.MapError("inspect", err)
	}
	s.info = info
	return info, nil
}
