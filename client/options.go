package client

import "time"

type config struct {
	socketPath  string
	binaryPath  string
	autoStart   bool
	strictVer   bool
	httpTimeout time.Duration
}

// Option configures a Client.
type Option func(*config)

// WithSocketPath overrides the daemon socket path (highest precedence).
func WithSocketPath(p string) Option { return func(c *config) { c.socketPath = p } }

// WithBinaryPath overrides the sbx binary path (default: looked up on PATH).
func WithBinaryPath(p string) Option { return func(c *config) { c.binaryPath = p } }

// WithAutoStart makes New ensure the daemon is running before returning.
func WithAutoStart() Option { return func(c *config) { c.autoStart = true } }

// WithStrictVersion makes the client hard-fail on an incompatible daemon version.
func WithStrictVersion() Option { return func(c *config) { c.strictVer = true } }

// WithHTTPTimeout sets the per-request REST timeout (0 = none).
func WithHTTPTimeout(d time.Duration) Option { return func(c *config) { c.httpTimeout = d } }
