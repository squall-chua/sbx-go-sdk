// Package kit reads and packages kit artifacts (`sbx kit`, EXPERIMENTAL
// upstream: "this command may change or be removed in future releases").
//
// A kit is a declarative YAML artifact — spec.yaml plus an optional files/
// directory — that contributes configuration to a sandbox. A kit of kind
// "mixin" extends an existing sandbox; one of kind "sandbox" supplies the
// base image instead. Attach a kit at creation with sandbox.WithKit, or
// afterwards with (*sandbox.Sandbox).AddKit.
//
// All five functions shell out to the sbx binary. The daemon exposes no kit
// REST endpoints (ADR 0001).
package kit

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/squall-chua/sbx-go-sdk/client"
)

// Manifest is the identity block of a kit, as reported by
// `sbx kit inspect --json`.
//
// Per ADR 0005 every field is present, strings and string slices are typed,
// and struct-valued fields stay raw. Nine fields are only meaningful for
// kind "sandbox" kits and are empty for a mixin.
type Manifest struct {
	SchemaVersion string          `json:"schemaVersion"`
	Kind          string          `json:"kind"` // "sandbox" or "mixin"
	Name          string          `json:"name"`
	Version       string          `json:"version"`
	DisplayName   string          `json:"displayName,omitempty"`
	Description   string          `json:"description,omitempty"`
	SourceURL     string          `json:"sourceURL,omitempty"`
	Binary        string          `json:"binary,omitempty"`
	Template      string          `json:"template,omitempty"`
	AIFilename    string          `json:"aiFilename,omitempty"`
	RunOptions    []string        `json:"runOptions,omitempty"`
	Resources     json.RawMessage `json:"resources,omitempty"`
	Build         json.RawMessage `json:"build,omitempty"`
	Security      json.RawMessage `json:"security,omitempty"`
	Volumes       json.RawMessage `json:"volumes,omitempty"`
}

// Info is what `sbx kit inspect --json` reports about a kit.
//
// It is a report, not the kit: the files/ directory is packed into the
// artifact by Pack but is not reported here.
//
// Struct-valued fields are left as raw JSON deliberately; see ADR 0005.
// Unmarshal one into a shape of your own when you need it.
type Info struct {
	Manifest       Manifest        `json:"manifest"`
	Extends        string          `json:"extends,omitempty"`
	Mixins         []string        `json:"mixins,omitempty"`
	Locked         []string        `json:"locked,omitempty"`
	Licenses       []string        `json:"licenses,omitempty"`
	AgentContext   string          `json:"agentContext,omitempty"`
	Warnings       []string        `json:"warnings,omitempty"`
	Requires       json.RawMessage `json:"requires,omitempty"`
	PublishedPorts json.RawMessage `json:"publishedPorts,omitempty"`
	Caps           json.RawMessage `json:"caps,omitempty"`
	Credentials    json.RawMessage `json:"credentials,omitempty"`
	Environment    json.RawMessage `json:"environment,omitempty"`
	Commands       json.RawMessage `json:"commands,omitempty"`
}

// Inspect loads a kit and reports its contents (`sbx kit inspect --json`).
//
// ref may be a local directory, a ZIP file, a git repository, or an OCI
// reference. A remote reference the kit.allowedSources setting forbids is
// refused by the CLI before any network access.
func Inspect(ctx context.Context, c *client.Client, ref string) (Info, error) {
	r, err := c.Runner()
	if err != nil {
		return Info{}, err
	}
	out, err := r.Capture(ctx, nil, "kit", "inspect", "--json", ref)
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
// (`sbx kit validate`). It returns nil when the CLI accepts the artifact.
//
// ref may be a local directory, a ZIP file, or a git repository. Unlike
// Inspect it may NOT be an OCI reference: the CLI refuses those for
// validation ("OCI references are not supported for validation"). That
// restriction is documented rather than enforced here — the CLI is the
// authority on what a reference is.
//
// A refusal returns an error wrapping client.ErrKitRejected. The CLI marks
// every refusal with an "INVALID:" line, so that sentinel covers both a
// malformed spec.yaml and a source that kit.allowedSources forbids; read the
// message to tell which.
//
// Warnings are not reported. A kit can be valid and still warn — for example
// "field \"mixins\" is accepted but not yet implemented" — and those warnings
// are lost here. Inspect returns them structured, in Info.Warnings.
func Validate(ctx context.Context, c *client.Client, ref string) error {
	r, err := c.Runner()
	if err != nil {
		return err
	}
	if _, err = r.Capture(ctx, nil, "kit", "validate", ref); err != nil {
		var ce *client.CLIError
		if errors.As(err, &ce) && strings.HasPrefix(strings.TrimSpace(ce.Stderr), "INVALID:") {
			return fmt.Errorf("%w: %s", client.ErrKitRejected, strings.TrimSpace(ce.Stderr))
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
// Unverified: this path has never been run against a real registry, because
// no registry was reachable when it was written.
func Push(ctx context.Context, c *client.Client, dir, ref string) error {
	r, err := c.Runner()
	if err != nil {
		return err
	}
	_, err = r.Capture(ctx, nil, "kit", "push", dir, ref)
	return err
}

// Pull fetches a kit artifact from an OCI registry and writes its layer
// payload to out (`sbx kit pull REFERENCE -o OUT`).
//
// The registry must support HTTPS. Registry secrets set with
// `sbx secret set --registry` take priority over the Docker credential store.
// As with Pack, out is required rather than derived.
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
