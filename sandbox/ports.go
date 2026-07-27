package sandbox

import (
	"context"
	"fmt"
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

// UnpublishPort removes a published port, given the CLI port spec form
// [[HOST_IP:]HOST_PORT:]SANDBOX_PORT[/PROTOCOL] — e.g. "127.0.0.1:18080:8080/tcp"
// or "18080:8080" (REST POST /sandbox/{name}/ports/unpublish; the body is a bare
// array of port keys).
//
// The published-ports list is always fetched and filtered against it: a key
// matches when its sandbox port equals the spec's, and its protocol, host port
// and host IP each equal the spec's whenever the spec gave that field. A
// loopback publish creates one key per address family, so a spec that omits the
// host address matches every address family and one call still fully
// unpublishes the port. A spec matching no published port is an error.
func (s *Sandbox) UnpublishPort(ctx context.Context, spec string) error {
	want, err := parsePortSpec(spec)
	if err != nil {
		return err
	}

	published, err := s.Ports(ctx)
	if err != nil {
		return fmt.Errorf("unpublish-port: %w", err)
	}
	var keys []Port
	for _, p := range published {
		if p.SandboxPort != want.SandboxPort {
			continue
		}
		if want.Protocol != "" && p.Protocol != want.Protocol {
			continue
		}
		if want.HostPort != 0 && p.HostPort != want.HostPort {
			continue
		}
		if want.HostIP != "" && p.HostIP != want.HostIP {
			continue
		}
		keys = append(keys, p)
	}
	if len(keys) == 0 {
		return fmt.Errorf("unpublish-port: no published port matches %q", spec)
	}

	route := "/sandbox/" + s.info.Name + "/ports/unpublish"
	if err := s.cli.Transport().DoJSON(ctx, http.MethodPost, route, keys, nil); err != nil {
		return client.MapError("unpublish-port", err)
	}
	return nil
}
