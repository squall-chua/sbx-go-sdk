package client

import (
	"context"

	"github.com/squall-chua/sbx-go-sdk/internal/cli"
	"github.com/squall-chua/sbx-go-sdk/internal/transport"
)

// Runner drives the sbx binary for shell-out operations. It is an alias for the
// generated internal type so external importers (which cannot import internal/cli)
// can name the value returned by Client.Runner.
type Runner = cli.Runner

// Client talks to a local sandboxd daemon (REST over a unix socket) and drives
// the sbx binary for orchestration-heavy operations.
type Client struct {
	cfg    config
	tr     *transport.Transport
	runner *cli.Runner // lazily created on first shell-out use
}

// New constructs a Client. By default it resolves the socket path (explicit >
// $DOCKER_SANDBOXES_API > XDG default) and does not start the daemon.
func New(ctx context.Context, opts ...Option) (*Client, error) {
	var cfg config
	for _, o := range opts {
		o(&cfg)
	}
	sock, err := transport.ResolveSocketPath(cfg.socketPath)
	if err != nil {
		return nil, err
	}
	tr := transport.New(sock)
	if cfg.httpTimeout > 0 {
		tr.SetTimeout(cfg.httpTimeout)
	}
	c := &Client{cfg: cfg, tr: tr}
	if cfg.autoStart {
		if err := c.EnsureRunning(ctx); err != nil {
			return nil, err
		}
	}
	if cfg.strictVer {
		res, err := c.CheckVersion(ctx)
		if err != nil {
			return nil, err
		}
		if res != "compatible" {
			return nil, ErrIncompatibleVersion
		}
	}
	return c, nil
}

// SocketPath returns the resolved daemon socket path.
func (c *Client) SocketPath() string { return c.tr.Socket() }

// Transport exposes the low-level transport to sibling packages within the module.
func (c *Client) Transport() *transport.Transport { return c.tr }

// runnerOrErr lazily resolves the sbx binary runner.
func (c *Client) runnerOrErr() (*cli.Runner, error) {
	if c.runner != nil {
		return c.runner, nil
	}
	r, err := cli.NewRunner(c.cfg.binaryPath)
	if err != nil {
		return nil, err
	}
	c.runner = r
	return r, nil
}

// Runner resolves and returns the sbx-binary runner (for shell-out ops in sibling packages).
func (c *Client) Runner() (*Runner, error) { return c.runnerOrErr() }

// DefaultClient is a lazily-initialized client over the default socket.
// It is created on first use by callers that want zero-config access.
var DefaultClient = mustDefault()

func mustDefault() *Client {
	sock, _ := transport.ResolveSocketPath("")
	return &Client{tr: transport.New(sock)}
}
