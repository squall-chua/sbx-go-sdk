package client

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// Health is the subset of the /daemon/health response used for liveness.
type Health struct {
	Release bool   `json:"release"`
	Status  string `json:"status"`
	Version string `json:"version"`
}

// Health returns daemon liveness. The standalone /health endpoint was removed in
// sbx v0.35.0 (daemon api 0.22.0); /daemon/health is now the liveness signal and
// its response is a superset of Health, so unmarshaling drops the extra fields.
func (c *Client) Health(ctx context.Context) (*Health, error) {
	var h Health
	if err := c.tr.DoJSON(ctx, http.MethodGet, "/daemon/health", nil, &h); err != nil {
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
const ClientVersion = "v0.38.0"

// TestedAPIVersion is the daemon REST api_version this SDK's wire types were
// generated from and validated against (see DaemonHealthResponse.APIVersion). The
// integration contract test (TestContract_VersionAlignment) fails when a live
// daemon drifts from it, signalling a re-sync is due.
const TestedAPIVersion = "0.26.0"

// CheckVersion asks the daemon whether this client is compatible.
//
// Deprecated: POST /version was removed in sbx v0.37.0 and answers 404 on every
// verb, so this can no longer succeed against a current daemon. Compare
// DaemonHealth().Version and .APIVersion against ClientVersion and
// TestedAPIVersion instead. See docs/adr/0004.
func (c *Client) CheckVersion(ctx context.Context) (string, error) {
	var resp versionResponse
	err := c.tr.DoJSON(ctx, http.MethodPost, "/version", versionRequest{Version: ClientVersion}, &resp)
	if err != nil {
		return "", fmt.Errorf("check-version: POST /version was removed in sbx v0.37.0; "+
			"compare DaemonHealth().APIVersion against TestedAPIVersion instead: %w",
			mapHTTPError("version", err))
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

// RestartDaemon restarts sandboxd via `sbx daemon restart` (shell-out, added in
// sbx v0.38.0). Some settings only take effect after a restart — `sbx settings
// list` marks them, and Setting.RequiresRestart reports the same flag.
func (c *Client) RestartDaemon(ctx context.Context) error {
	r, err := c.runnerOrErr()
	if err != nil {
		return err
	}
	if _, err := r.Capture(ctx, nil, "daemon", "restart"); err != nil {
		return err
	}
	return c.waitHealthy(ctx, 30*time.Second)
}

// MCPGatewayModeResponse is the /mcp/gateway-mode response (sbx v0.38.0): which
// MCP gateway a new sandbox gets. Decision is "local" or "saas"; GatewayURL is
// "none" for the local gateway.
type MCPGatewayModeResponse struct {
	Decision   string `json:"decision"`
	GatewayURL string `json:"gateway_url"`
	Reason     string `json:"reason"`
}

// MCPGatewayMode reports whether sandboxes get the local or the hosted (SaaS)
// MCP gateway, and why. The choice depends on Docker entitlement and on the
// mcp.forceLocalGateway setting.
func (c *Client) MCPGatewayMode(ctx context.Context) (*MCPGatewayModeResponse, error) {
	var m MCPGatewayModeResponse
	if err := c.tr.DoJSON(ctx, http.MethodGet, "/mcp/gateway-mode", nil, &m); err != nil {
		return nil, mapHTTPError("mcp-gateway-mode", err)
	}
	return &m, nil
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

// Login signs in to Docker non-interactively (`sbx login --username U
// --password-stdin`). token is a Docker password or personal access token; it is
// written to the child process's stdin and never appears in the argument vector.
//
// This is the only scriptable path into `sbx login`. Bare `sbx login` opens a
// browser for OAuth and is not wrapped.
//
// Sandboxes need an authenticated host: without it the daemon cannot pull
// template images.
func (c *Client) Login(ctx context.Context, username, token string) error {
	if username == "" {
		return errors.New("login: username must not be empty")
	}
	if token == "" {
		return errors.New("login: token must not be empty")
	}
	r, err := c.runnerOrErr()
	if err != nil {
		return err
	}
	_, err = r.CaptureStdin(ctx, strings.NewReader(token+"\n"), nil,
		"login", "--username", username, "--password-stdin")
	return err
}

// Logout signs out of Docker (`sbx logout --yes`).
//
// It **stops every running sandbox** first — that is upstream's behaviour, not
// this SDK's addition. --yes is always passed, because the confirmation prompt
// would otherwise block on non-interactive stdin. Sandboxes are stopped, not
// removed; their state survives.
func (c *Client) Logout(ctx context.Context) error {
	r, err := c.runnerOrErr()
	if err != nil {
		return err
	}
	_, err = r.Capture(ctx, nil, "logout", "--yes")
	return err
}

// DiagnosticCheck is one check from `sbx diagnose -o json`. Detail and Hint are
// often empty; Hint carries the CLI's remediation advice when it has one.
type DiagnosticCheck struct {
	Name    string `json:"name"`
	Status  string `json:"status"` // "pass" | "warn" | "fail" | "skip"
	Message string `json:"message"`
	Detail  string `json:"detail"`
	Hint    string `json:"hint"`
}

// DiagnosticSummary counts the checks by status.
type DiagnosticSummary struct {
	Pass int `json:"pass"`
	Warn int `json:"warn"`
	Fail int `json:"fail"`
	Skip int `json:"skip"`
}

// Diagnosis is the `sbx diagnose -o json` report: the CLI-side install checker.
type Diagnosis struct {
	Version string            `json:"version"` // report schema version, "1.0" at sbx v0.38.0
	Checks  []DiagnosticCheck `json:"checks"`
	Summary DiagnosticSummary `json:"summary"`
}

// OK reports whether no check failed. Warnings do not make it false — a host
// with no internet to check for CLI updates warns, and still works.
func (d *Diagnosis) OK() bool { return d.Summary.Fail == 0 }

// Diagnose runs the CLI-side install checker (`sbx diagnose -o json`, whose JSON
// output arrived in sbx v0.38.0). It reports on the binary, the daemon, storage
// paths, the socket and authentication.
//
// This is not Diagnostics: that returns the daemon's own self-check from
// /daemon/diagnostics, a much larger and entirely different report.
//
// `sbx diagnose --upload` sends the report to Docker support. It is deliberately
// not wrapped — shipping host diagnostics to a third party should be an explicit
// act, not a side effect of a library call.
func (c *Client) Diagnose(ctx context.Context) (*Diagnosis, error) {
	r, err := c.runnerOrErr()
	if err != nil {
		return nil, err
	}
	out, err := r.Capture(ctx, nil, "diagnose", "-o", "json")
	if err != nil {
		return nil, err
	}
	var d Diagnosis
	if err := json.Unmarshal([]byte(out), &d); err != nil {
		return nil, fmt.Errorf("diagnose: %w: %w", ErrUnexpectedFormat, err)
	}
	return &d, nil
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
