// Package kit reads and packages kit artifacts (`sbx kit`, EXPERIMENTAL
// upstream: "this command may change or be removed in future releases").
//
// A kit is a declarative YAML artifact — spec.yaml plus an optional files/
// directory — that contributes configuration to a sandbox. A kit of kind
// "mixin" extends an existing sandbox; one of kind "sandbox" supplies the
// base image instead. Attach a kit at creation with sandbox.WithKit, or
// afterwards with (*sandbox.Sandbox).AddKit.
//
// Every function here shells out to the sbx binary. The daemon exposes no kit
// REST endpoints (ADR 0001).
package kit

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/squall-chua/sbx-go-sdk/client"
)

// Info is what `sbx kit inspect --json` reports about a kit.
//
// It is a report, not the kit: the files/ directory is packed into the
// artifact by Pack but is not reported here.
//
// Per ADR 0005 every field is present, strings and string slices are typed,
// and struct-valued fields stay raw. Unmarshal one into a shape of your own
// when you need it.
//
// The shape follows the kit spec v2 document, flat and with no manifest
// wrapper. A schemaVersion "1" kit is normalized up to it — the report reads
// the same either way, apart from SchemaVersion itself and a deprecation
// entry in Warnings naming each legacy key.
//
// Sandbox, and the Security block that pairs with it, are only meaningful for
// kind "sandbox" kits and are empty for a mixin.
//
// Changed in sbx v0.39.0. Before it, inspect --json nested the identity fields
// under "manifest" and reported the v1 key names, which is why the Manifest
// type is gone. Nothing was lost, but several fields moved. Where a v0.38.0
// field went:
//
//	Manifest.SchemaVersion, .Kind, .Name, .Version   → the top level here
//	Manifest.DisplayName, .Description, .SourceURL   → the top level here
//	Manifest.Template                                → Sandbox, as "image"
//	Manifest.Binary                                  → Sandbox, as "entrypoint"
//	Manifest.RunOptions                              → Sandbox, as "command.default"
//	Manifest.InteractiveOptions                      → Sandbox, as "command.interactive"
//	Manifest.Resources                               → Sandbox, as "resources"
//	Manifest.Build                                   → Sandbox, as "build"
//	Manifest.AIFilename                              → AgentInstructions, as "filename"
//	Manifest.Security                                → Security
//	Manifest.Volumes                                 → Volumes
//	Caps                                             → Permissions
//	Commands                                         → Setup
//	AgentContext                                     → AgentInstructions, as "content"
//	PublishedPorts                                   → Ports
//
// Resources changed shape as well as place: its memory is now a unit string
// such as "2048m" where v1 reported a numeric "memoryMB".
type Info struct {
	SchemaVersion string   `json:"schemaVersion"`
	Kind          string   `json:"kind"` // "sandbox" or "mixin"
	Name          string   `json:"name"`
	Version       string   `json:"version"`
	DisplayName   string   `json:"displayName,omitempty"`
	Description   string   `json:"description,omitempty"`
	SourceURL     string   `json:"sourceURL,omitempty"`
	Extends       string   `json:"extends,omitempty"`
	Mixins        []string `json:"mixins,omitempty"`
	Locked        []string `json:"locked,omitempty"`
	Licenses      []string `json:"licenses,omitempty"`
	Warnings      []string `json:"warnings,omitempty"`

	Sandbox           json.RawMessage `json:"sandbox,omitempty"`
	Security          json.RawMessage `json:"security,omitempty"`
	Requires          json.RawMessage `json:"requires,omitempty"`
	AgentInstructions json.RawMessage `json:"agentInstructions,omitempty"`
	Permissions       json.RawMessage `json:"permissions,omitempty"`
	Credentials       json.RawMessage `json:"credentials,omitempty"`
	Environment       json.RawMessage `json:"environment,omitempty"`
	Volumes           json.RawMessage `json:"volumes,omitempty"`
	Setup             json.RawMessage `json:"setup,omitempty"`

	// Ports is what a schemaVersion "1" kit declared as "publishedPorts". Kit
	// spec v2 dropped the input key — a v2 spec declaring publishedPorts is
	// rejected outright — but a v1 kit still reports its ports here, under the
	// renamed output key.
	Ports json.RawMessage `json:"ports,omitempty"`
}

// Inspect loads a kit and reports its contents (`sbx kit inspect --json`).
//
// ref may be a local directory, a ZIP file, a git repository, or an OCI
// reference. A remote reference the kit.allowedSources setting forbids is
// refused by the CLI before any network access, as the CLI's own error — not
// as client.ErrKitRejected, which only Validate produces. The refusal is
// plain text on stderr, not JSON, even with --json set.
func Inspect(ctx context.Context, c *client.Client, ref string) (Info, error) {
	return InspectWith(ctx, c, ref)
}

// LoadOption configures InspectWith and ValidateWith.
type LoadOption func(*loadConfig)

type loadConfig struct {
	resolved, offline bool
}

// WithResolved merges the kit's extends: parent into the report
// (`--resolved`), so it shows what sbx applies rather than only what the kit
// declares. InspectWith only: ValidateWith refuses it, as the CLI has no such
// flag for validate.
//
// Added in sbx v0.51.0, where no parent could be resolved in a live
// probe: built-in agent names are refused as "no longer a built-in agent",
// and an OCI reference or a path is not accepted as a parent. A kit without
// extends: reports the same with or without it. How Info looks after a real
// merge is unverified.
func WithResolved() LoadOption { return func(c *loadConfig) { c.resolved = true } }

// WithOffline skips the daemon-backed kit policy (kit.allowedSources,
// kit.allowLocalKits, kit.requireSignature), so the call needs neither
// sandboxd nor a sign-in (`--offline`, sbx v0.49.1).
//
// The CLI then accepts only a local directory or ZIP; anything else is
// refused as a plain *client.CLIError. A kit spec v3 source kit cannot load
// offline either ("this load path has no kit builder configured"), with or
// without a dockerfile. Through ValidateWith that refusal is a "valid": false
// report, so it wraps client.ErrKitRejected even though the kit itself may
// be fine.
func WithOffline() LoadOption { return func(c *loadConfig) { c.offline = true } }

func (c *loadConfig) args() []string {
	var a []string
	if c.resolved {
		a = append(a, "--resolved")
	}
	if c.offline {
		a = append(a, "--offline")
	}
	return a
}

// InspectWith is Inspect with options.
func InspectWith(ctx context.Context, c *client.Client, ref string, opts ...LoadOption) (Info, error) {
	var cfg loadConfig
	for _, o := range opts {
		o(&cfg)
	}
	r, err := c.Runner()
	if err != nil {
		return Info{}, err
	}
	args := append(append([]string{"kit", "inspect", "--json"}, cfg.args()...), ref)
	out, err := r.Capture(ctx, nil, args...)
	if err != nil {
		return Info{}, err
	}
	var info Info
	if err := json.Unmarshal([]byte(out), &info); err != nil {
		return Info{}, fmt.Errorf("%w: kit inspect --json: %v", client.ErrUnexpectedFormat, err)
	}
	return info, nil
}

// Validate reports whether ref is a well-formed kit artifact
// (`sbx kit validate --json`). It returns nil when the CLI accepts the artifact.
//
// ref may be a local directory, a ZIP file, or a git repository. Unlike
// Inspect it may NOT be an OCI reference: the CLI refuses those for
// validation ("OCI references are not supported for validation"). That
// restriction is documented rather than enforced here — the CLI is the
// authority on what a reference is.
//
// A refusal returns an error wrapping client.ErrKitRejected, carrying the
// CLI's reason. A refusal is a report with "valid": false on stdout, which
// covers a malformed spec.yaml and, for a kit spec v3, a failed build. Any
// other failure — a missing path, a bad flag — prints no report and returns
// the raw *client.CLIError. (Through v0.42.1 the CLI marked a refusal with an
// "INVALID:" line on stderr instead; v0.51.0 dropped it.)
//
// Warnings are not reported. A kit can be valid and still warn — for example
// "field \"mixins\" is accepted but not yet implemented" — and those warnings
// are lost here. Inspect returns them structured, in Info.Warnings.
func Validate(ctx context.Context, c *client.Client, ref string) error {
	return ValidateWith(ctx, c, ref)
}

// ValidateWith is Validate with options. Only WithOffline applies; WithResolved
// is refused before the CLI runs.
func ValidateWith(ctx context.Context, c *client.Client, ref string, opts ...LoadOption) error {
	var cfg loadConfig
	for _, o := range opts {
		o(&cfg)
	}
	if cfg.resolved {
		return errors.New("kit: WithResolved does not apply to ValidateWith; kit validate has no --resolved flag")
	}
	r, err := c.Runner()
	if err != nil {
		return err
	}
	args := append(append([]string{"kit", "validate", "--json"}, cfg.args()...), ref)
	out, err := r.Capture(ctx, nil, args...)
	if err != nil {
		var report struct {
			Valid *bool  `json:"valid"`
			Error string `json:"error"`
		}
		if json.Unmarshal([]byte(out), &report) == nil && report.Valid != nil && !*report.Valid {
			return fmt.Errorf("%w: %s", client.ErrKitRejected, report.Error)
		}
		return err
	}
	return nil
}

// Pack validates a kit directory and writes it to out as a ZIP
// (`sbx kit pack DIRECTORY -o OUT`).
//
// out is required. The CLI would otherwise derive a name from the kit and the
// artifact format and write it into the calling process's working directory —
// a terminal convenience that is a trap in a library.
func Pack(ctx context.Context, c *client.Client, dir, out string) error {
	r, err := c.Runner()
	if err != nil {
		return err
	}
	_, err = r.Capture(ctx, nil, "kit", "pack", dir, "-o", out)
	return err
}

// Push packages a kit directory and pushes it to an OCI registry
// (`sbx kit push DIRECTORY REFERENCE`).
//
// The artifact format follows the kit's schemaVersion: "1" pushes a legacy
// ZIP-based artifact, "2" a tar+gzip layer carrying the spec in the manifest
// config blob. Authentication uses the Docker credential store.
//
// Unlike Inspect and Pull, Push does NOT consult kit.allowedSources: a
// forbidden reference is not refused up front. Instead the CLI opens real
// connections to registry infrastructure and, against an unreachable
// endpoint, retries indefinitely with no output, even with --debug. Verified
// 2026-07-28 against sbx v0.37.0.
//
// Pass a ctx with a deadline. On cancellation the SDK sends the process an
// interrupt and, if it doesn't exit in time, kills it and closes its pipes
// (internal/cli/cli.go's WaitDelay) — a child that ignores the interrupt, or
// a grandchild holding the output pipe, no longer blocks the call forever.
// A deadline therefore does return control to the caller. That is not the
// same as a clean abort: the CLI may be killed mid-push rather than exiting
// on its own, and if it succeeds but leaves a grandchild on the pipe past
// the delay, this call surfaces that as an error rather than success.
//
// Every push also attaches a SLSA provenance attestation as an OCI referrer.
// Without WithSign that attestation is unsigned; read it back with Provenance.
//
// Unverified: this path has never completed against a real registry, because
// no registry was reachable when it was written.
func Push(ctx context.Context, c *client.Client, dir, ref string, opts ...PushOption) error {
	var cfg pushConfig
	for _, o := range opts {
		o(&cfg)
	}
	r, err := c.Runner()
	if err != nil {
		return err
	}
	args := []string{"kit", "push", dir, ref}
	args = append(args, cfg.args()...)
	_, err = r.Capture(ctx, nil, args...)
	return err
}

type pushConfig struct {
	sign bool
	signConfig
}

// PushOption configures Push.
type PushOption func(*pushConfig)

// WithSign signs the pushed kit and attaches the signature as an OCI referrer
// (`--sign`), and signs the provenance attestation as a DSSE in-toto
// attestation with the same identity or key.
//
// It takes the same options as Sign: WithKey for key-based signing, or nothing
// for keyless. Unverified, like the rest of Push — no registry was reachable.
func WithSign(opts ...SignOption) PushOption {
	return func(c *pushConfig) {
		c.sign = true
		for _, o := range opts {
			o(&c.signConfig)
		}
	}
}

func (c *pushConfig) args() []string {
	if !c.sign {
		return nil
	}
	return append([]string{"--sign"}, c.signConfig.args()...)
}

// Pull fetches a kit artifact from an OCI registry and writes its layer
// payload to out (`sbx kit pull REFERENCE -o OUT`).
//
// The registry must support HTTPS. Registry secrets set with
// `sbx secret set --registry` take priority over the Docker credential store.
// As with Pack, out is required rather than derived.
//
// Unlike Push, Pull does enforce kit.allowedSources: a forbidden reference is
// refused instantly, before any network access. Verified 2026-07-28 against
// sbx v0.37.0.
//
// Unverified: this path has never been run against a real registry, because
// no registry was reachable when it was written.
func Pull(ctx context.Context, c *client.Client, ref, out string) error {
	r, err := c.Runner()
	if err != nil {
		return err
	}
	_, err = r.Capture(ctx, nil, "kit", "pull", ref, "-o", out)
	return err
}
