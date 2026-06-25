package sandbox

import (
	"context"
	"net/http"

	"github.com/squall-chua/sbx-go-sdk/client"
)

// Port is a published port mapping (host <-> sandbox).
type Port struct {
	HostIP      string `json:"host_ip,omitempty"`
	HostPort    int    `json:"host_port,omitempty"`
	Protocol    string `json:"protocol,omitempty"`
	SandboxPort int    `json:"sandbox_port"`
}

// Ports lists the sandbox's published ports (REST GET /sandbox/{name}/ports).
func (s *Sandbox) Ports(ctx context.Context) ([]Port, error) {
	var ports []Port
	if err := s.cli.Transport().DoJSON(ctx, http.MethodGet, "/sandbox/"+s.info.Name+"/ports", nil, &ports); err != nil {
		return nil, client.MapError("ports", err)
	}
	return ports, nil
}

// PublishPort publishes one port mapping and returns the full published set
// (REST POST /sandbox/{name}/ports; the body is a one-element array — the endpoint
// is additive). A zero HostPort requests an ephemeral host port.
func (s *Sandbox) PublishPort(ctx context.Context, p Port) ([]Port, error) {
	var out []Port
	if err := s.cli.Transport().DoJSON(ctx, http.MethodPost, "/sandbox/"+s.info.Name+"/ports", []Port{p}, &out); err != nil {
		return nil, client.MapError("publish-port", err)
	}
	return out, nil
}

// UnpublishPort removes a published port. No REST unpublish path is confirmed in
// v0.33.0, so this shells out to `sbx ports {name} --unpublish SPEC`, where spec is
// the CLI port spec, e.g. "127.0.0.1:18080:8080/tcp" or "18080:8080".
func (s *Sandbox) UnpublishPort(ctx context.Context, spec string) error {
	r, err := s.cli.Runner()
	if err != nil {
		return err
	}
	_, err = r.Capture(ctx, nil, "ports", s.info.Name, "--unpublish", spec)
	return err
}
