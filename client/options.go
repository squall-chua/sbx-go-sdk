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
//
// Deprecated: this compares the daemon's api_version to TestedAPIVersion with
// exact string equality, and api_version bumps on every sbx release — so it
// fires on every upgrade, including ones where nothing the SDK uses changed. It
// left a downstream consumer unable to start at v0.37.0, whose wire types are
// byte-identical to v0.35.0.
//
// Drift is detected at development time by TestContract_VersionAlignment. For a
// runtime check, compare DaemonHealth().Version / .APIVersion against
// ClientVersion / TestedAPIVersion and apply your own policy. See docs/adr/0004.
func WithStrictVersion() Option { return func(c *config) { c.strictVer = true } }

// WithHTTPTimeout sets the per-request REST timeout (0 = none).
func WithHTTPTimeout(d time.Duration) Option { return func(c *config) { c.httpTimeout = d } }
