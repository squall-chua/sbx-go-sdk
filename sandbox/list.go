package sandbox

import (
	"context"
	"net/http"

	"github.com/squall-chua/sbx-go-sdk/client"
	"github.com/squall-chua/sbx-go-sdk/internal/api"
)

// statusString reads the status field regardless of whether the generated type
// models it as a string or a defined string type.
func statusString(info api.SandboxInfo) string { return string(info.Status) }

// List returns all sandboxes.
func List(ctx context.Context, c *client.Client) ([]*Sandbox, error) {
	var infos []api.SandboxInfo
	if err := c.Transport().DoJSON(ctx, http.MethodGet, "/sandbox", nil, &infos); err != nil {
		return nil, client.MapError("list", err)
	}
	out := make([]*Sandbox, len(infos))
	for i, in := range infos {
		out[i] = newSandbox(c, in)
	}
	return out, nil
}

// Get returns a single sandbox by name.
func Get(ctx context.Context, c *client.Client, name string) (*Sandbox, error) {
	var info api.SandboxInfo
	if err := c.Transport().DoJSON(ctx, http.MethodGet, "/sandbox/"+name, nil, &info); err != nil {
		return nil, client.MapError("get", err)
	}
	return newSandbox(c, info), nil
}
