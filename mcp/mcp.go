// Package mcp registers and manages MCP servers for sandbox sessions
// (`sbx mcp`, added in sbx v0.38.0).
//
// A registered server is stored once on the host and then reused: name it in
// sandbox.WithStaticMCP at creation time, or attach it to an already-running
// sandbox with Load. Connected agents see a loaded server's tools immediately
// through the standard MCP tools/list_changed notification, without restarting.
//
// OAuth credentials for a remote server stay on the host — they are never
// injected into a sandbox. Which gateway a sandbox gets (the local one or the
// hosted SaaS one) is reported by client.MCPGatewayMode.
//
// Every function shells out to the sbx binary. The daemon exposes no MCP
// registry REST endpoints (ADR 0001); only /mcp/gateway-mode is REST.
package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/squall-chua/sbx-go-sdk/client"
	"github.com/squall-chua/sbx-go-sdk/internal/oauthflow"
)

// Server is one server of `sbx mcp ls`.
//
// The listing carries no endpoint: a server's URL or command line is read from
// Inspect.
type Server struct {
	// Name is the registered name, the handle used everywhere else in this
	// package and by sandbox.WithStaticMCP.
	Name string
	// Type is "local" (a stdio command run on the host) or "remote" (an MCP
	// endpoint reached over HTTP).
	Type string
	// Transport is how the server is spoken to: "stdio" for a local server,
	// "http" for a remote one.
	Transport string
	// Status is the readiness word the CLI reports, e.g. "ready".
	Status string
}

// List returns the registered MCP servers (`sbx mcp ls --json`, added in sbx
// v0.42.0). None registered is an empty slice and a nil error. Output with no
// "servers" key is client.ErrUnexpectedFormat, never an empty list.
func List(ctx context.Context, c *client.Client) ([]Server, error) {
	raw, err := capture(ctx, c, "mcp", "ls", "--json")
	if err != nil {
		return nil, err
	}
	var v struct {
		Servers *[]struct {
			Name      string `json:"name"`
			Type      string `json:"type"`
			Transport string `json:"transport"`
			Status    string `json:"status"`
		} `json:"servers"`
	}
	if err := json.Unmarshal([]byte(raw), &v); err != nil {
		return nil, fmt.Errorf("mcp ls: %w: %w", client.ErrUnexpectedFormat, err)
	}
	if v.Servers == nil {
		return nil, fmt.Errorf("mcp ls: %w: no \"servers\" key in output", client.ErrUnexpectedFormat)
	}
	out := []Server{}
	for _, s := range *v.Servers {
		out = append(out, Server{
			Name: s.Name,
			Type: s.Type,
			// The CLI reports "local stdio" / "remote http"; the first word is Type.
			Transport: strings.TrimPrefix(s.Transport, s.Type+" "),
			Status:    s.Status,
		})
	}
	return out, nil
}

// Details is what `sbx mcp inspect` reports about one server. A field that does
// not apply to the server's type is empty.
type Details struct {
	Name string
	Type string // "local" or "remote"

	URL       string // remote: the endpoint
	Transport string // remote: e.g. "streamable-http"

	Command  string // local: the command line as registered, joined by spaces
	Resolved string // local: the absolute executable the command resolved to

	Image    string // registry-sourced servers: the OCI image
	Registry string // registry-sourced servers: the registry URL

	// RequiresOAuth reports that the server needs OAuth. Authorize such a
	// server with Authorize; AuthStatus reports whether that has happened.
	RequiresOAuth bool

	// Issuer and Registration are the server's authorization server and its
	// dynamic-client registration endpoint. Registration is empty when the
	// server advertises none, which is exactly when WithClientID becomes
	// mandatory.
	Issuer       string
	Registration string

	// Fields maps the labels the text `sbx mcp inspect` printed ("Name", "URL",
	// "OAuth", …) to their values, for each field above that is set.
	//
	// Deprecated: Inspect reads `mcp inspect --json` since sbx v0.42.0, and
	// every field it reports has a named field above. Fields is rebuilt from
	// them and will not grow.
	Fields map[string]string
}

// Inspect returns the details of one registered server (`sbx mcp inspect NAME
// --json`). An unregistered name exits non-zero and surfaces as the raw
// *client.CLIError.
func Inspect(ctx context.Context, c *client.Client, name string) (*Details, error) {
	if name == "" {
		return nil, errors.New("mcp inspect: name must not be empty")
	}
	raw, err := capture(ctx, c, "mcp", "inspect", name, "--json")
	if err != nil {
		return nil, err
	}
	var v struct {
		Name            string   `json:"name"`
		Type            string   `json:"type"`
		URL             string   `json:"url"`
		Command         []string `json:"command"`
		Image           string   `json:"image"`
		RegistryURL     string   `json:"registry_url"`
		RemoteTransport string   `json:"remote_transport"`
		RequiresOAuth   bool     `json:"requires_oauth"`
		OAuthProviders  []struct {
			Issuer               string `json:"issuer"`
			RegistrationEndpoint string `json:"registration_endpoint"`
		} `json:"oauth_providers"`
		ResolvedCommand string `json:"resolved_command"`
	}
	if err := json.Unmarshal([]byte(raw), &v); err != nil {
		return nil, fmt.Errorf("mcp inspect %q: %w: %w", name, client.ErrUnexpectedFormat, err)
	}
	if v.Name == "" {
		return nil, fmt.Errorf("mcp inspect %q: %w: no name in output", name, client.ErrUnexpectedFormat)
	}
	d := &Details{
		Name:          v.Name,
		Type:          v.Type,
		URL:           v.URL,
		Transport:     v.RemoteTransport,
		Command:       strings.Join(v.Command, " "),
		Resolved:      v.ResolvedCommand,
		Image:         v.Image,
		Registry:      v.RegistryURL,
		RequiresOAuth: v.RequiresOAuth,
	}
	if len(v.OAuthProviders) > 0 {
		d.Issuer = v.OAuthProviders[0].Issuer
		d.Registration = v.OAuthProviders[0].RegistrationEndpoint
	}
	d.Fields = map[string]string{}
	for label, val := range map[string]string{
		"Name": d.Name, "Type": d.Type, "URL": d.URL, "Transport": d.Transport,
		"Command": d.Command, "Resolved": d.Resolved, "Image": d.Image,
		"Registry": d.Registry, "Issuer": d.Issuer, "Registration": d.Registration,
	} {
		if val != "" {
			d.Fields[label] = val
		}
	}
	if d.RequiresOAuth {
		d.Fields["OAuth"] = "required"
	}
	return d, nil
}

type addConfig struct {
	local         bool
	skipAuth      bool
	skipSSRFCheck bool
	clientID      string
	authServer    string
	workingDir    string
	scopes        []string
}

// AddOption configures AddRemote and AddLocal.
type AddOption func(*addConfig)

// WithLocalRun runs a registry-sourced OCI server locally via docker run
// (`--local`, stdio packages only). Remote servers only.
func WithLocalRun() AddOption { return func(c *addConfig) { c.local = true } }

// WithSkipAuth registers an OAuth server without starting the hosted OAuth flow
// (`--skip-auth`). That flow opens a browser and waits, so a non-interactive
// caller registering an OAuth server wants this; authorize separately with
// `sbx mcp auth <name>` afterwards.
func WithSkipAuth() AddOption { return func(c *addConfig) { c.skipAuth = true } }

// WithSkipSSRFCheck allows a --url whose host resolves to a private, loopback,
// link-local, or cloud-metadata address (`--skip-ssrf-check`). It also disables
// the DNS-rebinding and redirect re-checks, so use it only for hosts you
// control — a split-horizon or VPN-only endpoint is the intended case.
func WithSkipSSRFCheck() AddOption { return func(c *addConfig) { c.skipSSRFCheck = true } }

// WithClientID supplies a pre-registered OAuth client id (`--client-id`). It is
// required when the server's discovered authorization metadata advertises no
// registration_endpoint, since dynamic client registration is then impossible.
//
// A confidential client's secret is not passed here and has no flag: store it
// as the global secret "mcp:<name>.client_secret" (secret.SetToken) before
// adding the server.
func WithClientID(id string) AddOption { return func(c *addConfig) { c.clientID = id } }

// WithOAuthAuthorizationServer hand-supplies RFC 8414 authorization-server
// metadata for a server that publishes none (`--oauth-authorization-server`).
// The argument is a local file path or an http(s) URL to the JSON document. The
// CLI requires WithClientID alongside it.
func WithOAuthAuthorizationServer(pathOrURL string) AddOption {
	return func(c *addConfig) { c.authServer = pathOrURL }
}

// WithScopes records the default OAuth scopes to request at consent time
// (`--scope`, repeatable). They are validated against the server's advertised
// scopes_supported when it publishes one. Remote servers only.
func WithScopes(scopes ...string) AddOption {
	return func(c *addConfig) { c.scopes = append(c.scopes, scopes...) }
}

// WithWorkingDir sets the working directory of a local command server
// (`--dir`). Local servers only.
func WithWorkingDir(dir string) AddOption { return func(c *addConfig) { c.workingDir = dir } }

func (cfg *addConfig) args() []string {
	var a []string
	if cfg.local {
		a = append(a, "--local")
	}
	if cfg.skipAuth {
		// Not --skip_auth: sbx v0.42.0 deprecated it and prints a warning.
		a = append(a, "--skip-auth")
	}
	if cfg.skipSSRFCheck {
		a = append(a, "--skip-ssrf-check")
	}
	if cfg.clientID != "" {
		a = append(a, "--client-id", cfg.clientID)
	}
	if cfg.authServer != "" {
		a = append(a, "--oauth-authorization-server", cfg.authServer)
	}
	if cfg.workingDir != "" {
		a = append(a, "--dir", cfg.workingDir)
	}
	for _, s := range cfg.scopes {
		a = append(a, "--scope", s)
	}
	return a
}

// AddRemote registers a remote MCP server (`sbx mcp add NAME --url URL`).
//
// The url may be a remote MCP endpoint, an MCP community-registry URL, a plain
// server-manifest URL (server.json / server.yaml), or a Docker Hardened Image
// ref (dhi.io/...). Other image refs are rejected by the CLI. Registering
// contacts the URL, so this is a network call.
//
// If the server needs OAuth, the CLI starts the hosted authorization flow and
// waits for a browser round trip unless WithSkipAuth is passed.
func AddRemote(ctx context.Context, c *client.Client, name, url string, opts ...AddOption) error {
	if name == "" {
		return errors.New("mcp add: name must not be empty")
	}
	if url == "" {
		return errors.New("mcp add: url must not be empty")
	}
	cfg := apply(opts)
	args := append([]string{"mcp", "add", name, "--url", url}, cfg.args()...)
	return run(ctx, c, args)
}

// AddLocal registers a local stdio server run as a host subprocess
// (`sbx mcp add NAME --command CMD --args ...`).
//
// The command runs on the HOST, outside any sandbox, with the calling user's
// full permissions — it can read the filesystem, reach the network, and call
// any API that user can. It has no identity and no supply-chain verification.
// Upstream documents this as ad-hoc development only; do not point it at an
// executable you do not trust.
func AddLocal(ctx context.Context, c *client.Client, name, command string, cmdArgs []string, opts ...AddOption) error {
	if name == "" {
		return errors.New("mcp add: name must not be empty")
	}
	if command == "" {
		return errors.New("mcp add: command must not be empty")
	}
	cfg := apply(opts)
	args := []string{"mcp", "add", name, "--command", command}
	if len(cmdArgs) > 0 {
		// The CLI takes --args as one comma-separated list, matching its own
		// `--args "run,-i,--rm,mcp/postgres"` example.
		args = append(args, "--args", strings.Join(cmdArgs, ","))
	}
	return run(ctx, c, append(args, cfg.args()...))
}

// Remove deletes a registered server (`sbx mcp rm NAME`). There is no
// confirmation prompt, and removing a name that was never registered is a
// silent success — the CLI exits 0.
//
// This removes only the local registration. Hosted OAuth credentials survive
// it; drop those with `sbx mcp auth rm <name>`.
func Remove(ctx context.Context, c *client.Client, name string) error {
	if name == "" {
		return errors.New("mcp rm: name must not be empty")
	}
	return run(ctx, c, []string{"mcp", "rm", name})
}

// Load attaches an already-registered server to a running sandbox's gateway
// (`sbx mcp load NAME --sandbox SANDBOX`). Connected agents pick up its tools
// immediately; no agent restart is needed.
//
// Load is the after-the-fact path. To fix a sandbox's MCP set when it is
// created, pass sandbox.WithStaticMCP instead.
func Load(ctx context.Context, c *client.Client, name, sandbox string) error {
	if name == "" {
		return errors.New("mcp load: name must not be empty")
	}
	if sandbox == "" {
		return errors.New("mcp load: sandbox must not be empty")
	}
	return run(ctx, c, []string{"mcp", "load", name, "--sandbox", sandbox})
}

// AuthResult is the hosted OAuth state of one registered MCP server, as
// `sbx mcp auth status --format json` reports it.
//
// Only ServerName and Status are always present; the rest are omitted unless
// they apply. Verified against a live registration at sbx v0.38.0:
// `[{"server_name":"sdkoauth","status":"unauthorized"}]`.
type AuthResult struct {
	ServerName string `json:"server_name"`

	// Status is the credential state. "unauthorized" is the observed value for a
	// server registered but never authorized. Upstream defines no closed set, so
	// compare against the values you care about rather than exhausting them.
	Status string `json:"status,omitempty"`

	// CredentialID identifies the stored hosted credential, once there is one.
	CredentialID string `json:"credential_id,omitempty"`

	// AuthorizationURL is the consent URL an interactive flow would open.
	AuthorizationURL string `json:"authorization_url,omitempty"`

	// Error carries a per-server failure; the call itself still succeeds.
	Error string `json:"error,omitempty"`
}

// Authorized reports whether the server has a usable hosted credential.
func (a AuthResult) Authorized() bool { return a.Status == "authorized" }

// Authorize runs the OAuth flow for one registered server (`sbx mcp auth NAME`),
// authorizing or reauthorizing it.
//
// With no terminal the CLI prints an authorization URL and then waits on a
// loopback callback until the user completes consent. Authorize hands that URL
// to onURL as soon as it appears and blocks until the flow finishes, so the
// caller decides how to present it — print it, open a browser, post it to a
// chat. Cancel ctx to abandon a flow nobody completes.
//
// onURL is called once, from another goroutine, while the child still runs; keep
// it quick and make it safe to call concurrently.
//
// If the stored credential is merely expired the CLI refreshes it first and only
// falls back to consent when the user must re-approve, so onURL may never fire
// on a successful call. scopes overrides the default recorded at registration.
//
// The URL emission, the blocking, and cancellation are verified against sbx
// v0.38.0; the success path is not — completing it needs a human at a browser.
func Authorize(ctx context.Context, c *client.Client, name string, onURL func(string), scopes ...string) error {
	if name == "" {
		return errors.New("mcp auth: name must not be empty")
	}
	r, err := c.Runner()
	if err != nil {
		return err
	}
	args := []string{"mcp", "auth", name}
	for _, s := range scopes {
		args = append(args, "--scope", s)
	}
	return oauthflow.Run(ctx, r, onURL, args...)
}

// AuthStatus returns the hosted OAuth credential state of every registered
// OAuth server (`sbx mcp auth status --all --format json`). It starts no OAuth
// flow and refreshes nothing. Servers that need no OAuth are not listed, so an
// empty slice is the normal answer on a host with only plain servers.
//
// The sibling `sbx mcp auth <name>` (authorize / reauthorize) opens a browser
// and waits for consent, so it is deliberately not wrapped. Register with
// WithSkipAuth from non-interactive code, then authorize out of band.
func AuthStatus(ctx context.Context, c *client.Client) ([]AuthResult, error) {
	return decodeAuthResults(ctx, c, "mcp auth status",
		[]string{"mcp", "auth", "status", "--all", "--format", "json"})
}

// AuthRemove deletes the hosted OAuth credentials of one registered server
// (`sbx mcp auth rm NAME`), leaving its registration in place. It returns the
// resulting state of that server.
//
// Removing credentials a server never had is a success, not an error: the CLI
// reports "No local OAuth token found" and exits 0.
func AuthRemove(ctx context.Context, c *client.Client, name string) ([]AuthResult, error) {
	if name == "" {
		return nil, errors.New("mcp auth rm: name must not be empty")
	}
	return decodeAuthResults(ctx, c, "mcp auth rm",
		[]string{"mcp", "auth", "rm", name, "--format", "json"})
}

func decodeAuthResults(ctx context.Context, c *client.Client, what string, args []string) ([]AuthResult, error) {
	out, err := capture(ctx, c, args...)
	if err != nil {
		return nil, err
	}
	var rs []AuthResult
	if err := json.Unmarshal([]byte(strings.TrimSpace(out)), &rs); err != nil {
		return nil, fmt.Errorf("%s: %w: %w", what, client.ErrUnexpectedFormat, err)
	}
	return rs, nil
}

func apply(opts []AddOption) addConfig {
	var cfg addConfig
	for _, o := range opts {
		o(&cfg)
	}
	return cfg
}

func capture(ctx context.Context, c *client.Client, args ...string) (string, error) {
	r, err := c.Runner()
	if err != nil {
		return "", err
	}
	return r.Capture(ctx, nil, args...)
}

func run(ctx context.Context, c *client.Client, args []string) error {
	_, err := capture(ctx, c, args...)
	return err
}
