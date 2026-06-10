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
