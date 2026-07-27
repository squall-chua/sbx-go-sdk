package sandbox

import (
	"context"
	"net/http"
	"net/url"
	"path/filepath"
	"strings"

	"github.com/squall-chua/sbx-go-sdk/client"
)

// Create provisions a sandbox (shell-out `sbx create`) and returns a hydrated
// handle. Workspaces are resolved to absolute; the SDK owns the name so it never
// parses create output.
func Create(ctx context.Context, c *client.Client, opts ...Option) (*Sandbox, error) {
	d := newDefinition(opts...)

	// Resolve workspaces to absolute, preserving any ":ro" suffix.
	for i, ws := range d.workspaces {
		path, ro, _ := strings.Cut(ws, ":")
		abs, err := filepath.Abs(path)
		if err != nil {
			return nil, err
		}
		if ro == "ro" {
			d.workspaces[i] = abs + ":ro"
		} else {
			d.workspaces[i] = abs
		}
	}

	// Determine the name (own identity).
	if d.name == "" {
		existing, err := listNames(ctx, c)
		if err != nil {
			return nil, err
		}
		primary, _, _ := strings.Cut(d.workspaces[0], ":")
		d.name = generateName(d.agent, primary, existing)
	} else {
		existing, err := listNames(ctx, c)
		if err != nil {
			return nil, err
		}
		if existing[d.name] {
			return nil, client.ErrSandboxExists
		}
	}

	args, err := d.toCreateArgs()
	if err != nil {
		return nil, err
	}
	r, err := c.Runner()
	if err != nil {
		return nil, err
	}
	if _, err := r.Capture(ctx, nil, args...); err != nil {
		return nil, err
	}
	return Get(ctx, c, d.name)
}

func listNames(ctx context.Context, c *client.Client) (map[string]bool, error) {
	sbs, err := List(ctx, c)
	if err != nil {
		return nil, err
	}
	m := make(map[string]bool, len(sbs))
	for _, s := range sbs {
		m[s.Name()] = true
	}
	return m, nil
}

// Start starts the sandbox VM (REST).
func (s *Sandbox) Start(ctx context.Context) error {
	return s.post(ctx, "/start")
}

// Stop stops the sandbox VM without removing it (REST).
func (s *Sandbox) Stop(ctx context.Context) error {
	return s.post(ctx, "/stop")
}

type removeConfig struct{ force bool }

// RemoveOption configures Remove.
type RemoveOption func(*removeConfig)

// WithForce removes the sandbox even when it is in use — for example with an
// open SSH connection (`?force=true`, matching `sbx rm --force`).
//
// A merely running sandbox does not need this; a plain Remove stops and deletes
// it. Force is for an active session, which became reachable in v0.37.0 now
// that feature.ssh defaults to enabled.
func WithForce() RemoveOption { return func(c *removeConfig) { c.force = true } }

// Remove deletes the sandbox (REST DELETE; no confirmation prompt).
func (s *Sandbox) Remove(ctx context.Context, opts ...RemoveOption) error {
	var cfg removeConfig
	for _, o := range opts {
		o(&cfg)
	}
	route := "/sandbox/" + s.info.Name
	if cfg.force {
		route += "?" + url.Values{"force": {"true"}}.Encode()
	}
	if err := s.cli.Transport().DoJSON(ctx, http.MethodDelete, route, nil, nil); err != nil {
		return client.MapError("remove", err)
	}
	return nil
}

func (s *Sandbox) post(ctx context.Context, suffix string) error {
	err := s.cli.Transport().DoJSON(ctx, http.MethodPost, "/sandbox/"+s.info.Name+suffix, nil, nil)
	if err != nil {
		return client.MapError("lifecycle", err)
	}
	return nil
}
