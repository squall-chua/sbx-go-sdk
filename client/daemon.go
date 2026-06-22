package client

import (
	"context"
	"encoding/json"
	"net/http"
	"time"
)

// Health is the /health response.
type Health struct {
	Release bool   `json:"release"`
	Status  string `json:"status"`
	Version string `json:"version"`
}

// Health returns daemon liveness.
func (c *Client) Health(ctx context.Context) (*Health, error) {
	var h Health
	if err := c.tr.DoJSON(ctx, http.MethodGet, "/health", nil, &h); err != nil {
		return nil, mapHTTPError("health", err)
	}
	return &h, nil
}

// versionRequest is the body the CLI sends to /version (its own version string).
type versionRequest struct {
	Version string `json:"version"`
}

type versionResponse struct {
	Result string `json:"result"`
}

// ClientVersion is the sbx/daemon version this SDK was built/tested against.
const ClientVersion = "v0.32.0"

// TestedAPIVersion is the daemon REST api_version this SDK's wire types were
// generated from and validated against (see DaemonHealthResponse.APIVersion). The
// integration contract test (TestContract_VersionAlignment) fails when a live
// daemon drifts from it, signalling a re-sync is due.
const TestedAPIVersion = "0.10.0"

// CheckVersion asks the daemon whether this client is compatible.
// Returns "compatible", "incompatible", or "unknown".
//
// Informational only: /version is dead on non-release daemons (it returns
// "incompatible" for every string, including the daemon's own version), so the
// strict-version check in New uses DaemonHealth().APIVersion instead.
func (c *Client) CheckVersion(ctx context.Context) (string, error) {
	var resp versionResponse
	err := c.tr.DoJSON(ctx, http.MethodPost, "/version", versionRequest{Version: ClientVersion}, &resp)
	if err != nil {
		return "", mapHTTPError("version", err)
	}
	return resp.Result, nil
}

// DaemonInfo is the /daemon/info response.
type DaemonInfo struct {
	APISocket    string  `json:"api_socket"`
	DockerSocket *string `json:"docker_socket,omitempty"`
}

// Info returns the daemon's socket paths.
func (c *Client) Info(ctx context.Context) (*DaemonInfo, error) {
	var d DaemonInfo
	if err := c.tr.DoJSON(ctx, http.MethodGet, "/daemon/info", nil, &d); err != nil {
		return nil, mapHTTPError("daemon-info", err)
	}
	return &d, nil
}

// LogLevels is the /daemon/loglevel response.
type LogLevels struct {
	General string `json:"general"`
	Proxy   string `json:"proxy"`
}

// LogLevels returns the daemon's per-category log levels.
func (c *Client) LogLevels(ctx context.Context) (*LogLevels, error) {
	var l LogLevels
	if err := c.tr.DoJSON(ctx, http.MethodGet, "/daemon/loglevel", nil, &l); err != nil {
		return nil, mapHTTPError("loglevel", err)
	}
	return &l, nil
}

// SetLogLevel sets a category's level. category: "proxy", "general", or "all".
func (c *Client) SetLogLevel(ctx context.Context, category, level string) error {
	body := map[string]string{"target": category, "level": level}
	if err := c.tr.DoJSON(ctx, http.MethodPost, "/daemon/loglevel/set", body, nil); err != nil {
		return mapHTTPError("set-loglevel", err)
	}
	return nil
}

// StopDaemon shuts the daemon down (REST).
func (c *Client) StopDaemon(ctx context.Context) error {
	if err := c.tr.DoJSON(ctx, http.MethodPost, "/daemon/shutdown", nil, nil); err != nil {
		return mapHTTPError("shutdown", err)
	}
	return nil
}

// Reset resets all sandboxes and daemon state (REST).
func (c *Client) Reset(ctx context.Context) error {
	if err := c.tr.DoJSON(ctx, http.MethodPost, "/daemon/reset", nil, nil); err != nil {
		return mapHTTPError("reset", err)
	}
	return nil
}

// StartOptions configures daemon start.
type StartOptions struct {
	Policy string // "allow-all" | "balanced" | "deny-all"; empty = daemon default
}

// EnsureRunning returns nil if the daemon is healthy; otherwise it starts it
// (shell-out) and waits up to ~30s for the socket to become healthy.
func (c *Client) EnsureRunning(ctx context.Context) error {
	if _, err := c.Health(ctx); err == nil {
		return nil
	}
	if err := c.StartDaemon(ctx, StartOptions{}); err != nil {
		return err
	}
	return c.waitHealthy(ctx, 30*time.Second)
}

// StartDaemon starts sandboxd via `sbx daemon start --detach` (shell-out).
func (c *Client) StartDaemon(ctx context.Context, opts StartOptions) error {
	r, err := c.runnerOrErr()
	if err != nil {
		return err
	}
	args := []string{"daemon", "start", "--detach"}
	if opts.Policy != "" {
		args = append(args, "--policy", opts.Policy)
	}
	if _, err := r.Capture(ctx, nil, args...); err != nil {
		return err
	}
	return nil
}

// DaemonHealthResponse is the /daemon/health response (richer than /health).
type DaemonHealthResponse struct {
	APIVersion string `json:"api_version"`
	Release    bool   `json:"release"`
	Revision   string `json:"revision"`
	Status     string `json:"status"`
	Version    string `json:"version"`
}

// DaemonHealth returns the daemon's detailed health (api version, revision, …).
func (c *Client) DaemonHealth(ctx context.Context) (*DaemonHealthResponse, error) {
	var h DaemonHealthResponse
	if err := c.tr.DoJSON(ctx, http.MethodGet, "/daemon/health", nil, &h); err != nil {
		return nil, mapHTTPError("daemon-health", err)
	}
	return &h, nil
}

// Diagnostics returns the daemon self-check report as raw JSON (a large nested
// object under an "info" key); callers decode the fields they need.
func (c *Client) Diagnostics(ctx context.Context) (json.RawMessage, error) {
	var raw json.RawMessage
	if err := c.tr.DoJSON(ctx, http.MethodGet, "/daemon/diagnostics", nil, &raw); err != nil {
		return nil, mapHTTPError("diagnostics", err)
	}
	return raw, nil
}

// Status reports daemon liveness plus the socket it was probed on.
type Status struct {
	Running bool
	Socket  string
}

// DaemonStatus probes the socket via Health and reports running + path. A down
// daemon yields Running=false with a nil error (so callers can branch).
func (c *Client) DaemonStatus(ctx context.Context) (Status, error) {
	st := Status{Socket: c.tr.Socket()}
	if _, err := c.Health(ctx); err == nil {
		st.Running = true
		return st, nil
	}
	if ctx.Err() != nil {
		return st, ctx.Err()
	}
	return st, nil
}

func (c *Client) waitHealthy(ctx context.Context, d time.Duration) error {
	deadline := time.Now().Add(d)
	for {
		if _, err := c.Health(ctx); err == nil {
			return nil
		}
		if time.Now().After(deadline) {
			return ErrDaemonNotRunning
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(250 * time.Millisecond):
		}
	}
}
