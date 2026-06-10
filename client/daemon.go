package client

import (
	"context"
	"net/http"
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

// CheckVersion asks the daemon whether this client is compatible.
// Returns "compatible", "incompatible", or "unknown".
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
	APISocket    string `json:"api_socket"`
	DockerSocket string `json:"docker_socket"`
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
