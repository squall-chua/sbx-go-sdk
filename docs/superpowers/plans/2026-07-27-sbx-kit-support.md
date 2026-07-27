# `sbx kit` Support Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Cover all six `sbx kit` subcommands, `--kit` on create, and reading a sandbox's kit list.

**Architecture:** A new `kit` package shells out to the `sbx` binary for the five artifact operations (`inspect`, `validate`, `pack`, `push`, `pull`), per ADR 0001 — no kit REST endpoints exist. Sandbox-scoped work lives on `*sandbox.Sandbox`, matching `SaveTemplate` and `PublishPort`: `AddKit` shells out, `Kits` reads a label off an endpoint the SDK already calls. `kit inspect --json` is unmarshalled under one rule from ADR 0005 — strings typed, structs left as `json.RawMessage`.

**Tech Stack:** Go 1.25. Standard library only — `encoding/json`, `os`, `path/filepath`, `strings`. Tests use `stretchr/testify/require`. No new dependencies.

## Global Constraints

- **No new dependencies.** New code uses the standard library only.
- **Source compatibility is mandatory.** `sbx-swarm-node` pins this SDK. No existing exported signature may change. Everything in this plan is additive.
- **`kit` imports `client`, never `internal/cli`.** `client.CLIError` (`client/errors.go:36`) is the exported alias of `cli.Error`, and carries `ExitCode int` and `Stderr string`.
- **Never run `sbx reset`.** It wipes all sandboxes, Docker auth, the default network policy and the skills store, and would break the user's other project.
- **Two sandboxes belong to another project** (`sbx-swarm-node`, names beginning `jzrdxd6r3pw3l6py.`). Do not remove, start, stop or modify them. Integration tests must create uniquely-named sandboxes and remove them in `t.Cleanup`.
- **Never write to the live secret store.**
- **Doc comments must not assert unverified CLI behaviour.** The spec's "Not verified" section lists six items; none may be stated as fact in code comments or docs.
- Existing test helpers to copy, not reinvent: `fakeClient(t, argFile, stdout)` (`policy/policy_test.go:17`), `recordingClient(t, argFile)` (`skillstore/skillstore_test.go:13`), `sandbox.NewForTest(c, name)` (`sandbox/sandbox.go:72`), `stubClient(t, http.Handler)` (`sandbox/list_test.go:14`).
- Reference: the spec at `docs/superpowers/specs/2026-07-27-sbx-kit-design.md`, and ADRs `0001` (shell-out default), `0002` (its ledger names the `INVALID:` match), `0005` (the typing rule).
- Branch: create `feat/sbx-kit-support` off `main`. `main` is 37 commits ahead of `origin/main` and deliberately unpushed — do not push, rebase or rewrite it.

---

## File Structure

| File | Responsibility |
|---|---|
| `client/errors.go` (modify) | add the `ErrKitRejected` sentinel |
| `kit/kit.go` (create) | `Info`, `Manifest`, and the five package functions |
| `kit/kit_test.go` (create) | argument vectors, JSON decoding, `ErrKitRejected` mapping |
| `sandbox/kit.go` (create) | `AddKit`, `Kits`, and the unexported `absLocal` shared with `WithKit` |
| `sandbox/kit_test.go` (create) | `absLocal`, `AddKit` args, `Kits` label decoding |
| `sandbox/definition.go` (modify) | `kits` field on `Definition`; emit `--kit` per reference |
| `sandbox/options.go` (modify) | `WithKit` |
| `sandbox/options_test.go` (modify) | `WithKit` emission and absence |
| `internal/integration/testdata/fixture-kit/spec.yaml` (create) | a kit `sbx kit add` will accept |
| `internal/integration/kit_test.go` (create) | live `Validate`/`Inspect`/`Pack`, and the `AddKit` absolute-path regression test |
| `docs/sbx-version-coverage.md` (modify) | rows 37, 44, 45 plus new rows |
| `REVERSE_ENGINEERING.md` (modify) | schema provenance; `SandboxInfo` carries no kit field |
| `README.md` (modify) | limitations |

`absLocal` lives in `sandbox/kit.go` because both `AddKit` and `WithKit` need it and both are in package `sandbox`.

---

### Task 1: `ErrKitRejected` and the `kit` package's read paths

**Files:**
- Modify: `client/errors.go:13-22`
- Create: `kit/kit.go`
- Test: `kit/kit_test.go`

**Interfaces:**
- Consumes: `client.Client`, `client.Client.Runner()`, `client.CLIError`, `client.ErrUnexpectedFormat`.
- Produces: `client.ErrKitRejected`; `kit.Info`, `kit.Manifest`; `kit.Inspect(ctx, c, ref) (Info, error)`; `kit.Validate(ctx, c, ref) error`.

- [ ] **Step 1: Write the failing tests**

Create `kit/kit_test.go`:

```go
package kit

import (
	"context"
	"errors"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/squall-chua/sbx-go-sdk/client"
	"github.com/stretchr/testify/require"
)

// fakeClient returns a client whose fake sbx binary records its args to argFile,
// prints stdout, prints stderr, and exits with code. argFile may be "".
func fakeClient(t *testing.T, argFile, stdout, stderr string, code int) *client.Client {
	t.Helper()
	sock := filepath.Join(t.TempDir(), "d.sock")
	l, err := net.Listen("unix", sock)
	require.NoError(t, err)
	srv := &http.Server{Handler: http.NewServeMux()}
	go srv.Serve(l)
	t.Cleanup(func() { srv.Close() })
	if argFile == "" {
		argFile = filepath.Join(t.TempDir(), "args.txt")
	}
	bin := filepath.Join(t.TempDir(), "sbx")
	script := "#!/bin/sh\n" +
		"printf '%s\\n' \"$*\" >> " + argFile + "\n" +
		"cat <<'SBXOUT'\n" + stdout + "\nSBXOUT\n" +
		"cat >&2 <<'SBXERR'\n" + stderr + "\nSBXERR\n" +
		"exit " + strconv.Itoa(code) + "\n"
	require.NoError(t, os.WriteFile(bin, []byte(script), 0o755))
	c, err := client.New(context.Background(), client.WithSocketPath(sock), client.WithBinaryPath(bin))
	require.NoError(t, err)
	return c
}

const sampleJSON = `{
  "manifest": {
    "schemaVersion": "2",
    "kind": "sandbox",
    "name": "fullkit",
    "version": "0.1.0",
    "template": "alpine:3.20",
    "runOptions": ["--foo"],
    "resources": {"cpu": 2, "memoryMB": 2048}
  },
  "mixins": ["ghcr.io/org/other:1.0"],
  "caps": {"network": {"allow": ["api.example.com"]}},
  "warnings": ["field \"mixins\" is accepted but not yet implemented"]
}`

func TestInspect_PassesJSONFlagAndRef(t *testing.T) {
	argFile := filepath.Join(t.TempDir(), "args.txt")
	c := fakeClient(t, argFile, sampleJSON, "", 0)

	_, err := Inspect(context.Background(), c, "./mykit")
	require.NoError(t, err)

	args, err := os.ReadFile(argFile)
	require.NoError(t, err)
	require.Contains(t, string(args), "kit inspect")
	require.Contains(t, string(args), "--json")
	require.Contains(t, string(args), "./mykit")
}

// Guards ADR 0005: every manifest field is present, strings typed and
// structs raw. An earlier draft hand-picked six fields and dropped
// "template", which real output contains for kind: sandbox kits.
func TestInspect_DecodesTypedStringsAndRawStructs(t *testing.T) {
	c := fakeClient(t, "", sampleJSON, "", 0)

	info, err := Inspect(context.Background(), c, "./mykit")
	require.NoError(t, err)

	require.Equal(t, "2", info.Manifest.SchemaVersion)
	require.Equal(t, "sandbox", info.Manifest.Kind)
	require.Equal(t, "fullkit", info.Manifest.Name)
	require.Equal(t, "alpine:3.20", info.Manifest.Template)
	require.Equal(t, []string{"--foo"}, info.Manifest.RunOptions)
	require.JSONEq(t, `{"cpu":2,"memoryMB":2048}`, string(info.Manifest.Resources))

	require.Equal(t, []string{"ghcr.io/org/other:1.0"}, info.Mixins)
	require.JSONEq(t, `{"network":{"allow":["api.example.com"]}}`, string(info.Caps))
	require.Len(t, info.Warnings, 1)
}

func TestInspect_MalformedJSONIsUnexpectedFormat(t *testing.T) {
	c := fakeClient(t, "", "not json at all", "", 0)

	_, err := Inspect(context.Background(), c, "./mykit")
	require.ErrorIs(t, err, client.ErrUnexpectedFormat)
}

func TestValidate_PassesRefAndSucceedsOnExitZero(t *testing.T) {
	argFile := filepath.Join(t.TempDir(), "args.txt")
	c := fakeClient(t, argFile, "VALID: ./mykit (directory)", "", 0)

	require.NoError(t, Validate(context.Background(), c, "./mykit"))

	args, err := os.ReadFile(argFile)
	require.NoError(t, err)
	require.Contains(t, string(args), "kit validate")
	require.Contains(t, string(args), "./mykit")
}

// `sbx kit validate` exits 1 both for a refused artifact and for a missing
// path. Only the leading "INVALID:" on stderr separates them, so that prefix
// is what the sentinel keys on. See the ledger in ADR 0002.
func TestValidate_InvalidPrefixMapsToErrKitRejected(t *testing.T) {
	c := fakeClient(t, "", "", "INVALID: artifact: spec.yaml is required\nERROR: artifact validation failed", 1)

	err := Validate(context.Background(), c, "./mykit")
	require.ErrorIs(t, err, client.ErrKitRejected)
	require.Contains(t, err.Error(), "spec.yaml is required")
}

func TestValidate_ErrorWithoutInvalidPrefixIsNotErrKitRejected(t *testing.T) {
	c := fakeClient(t, "", "", `ERROR: kit reference "./nope": path does not exist`, 1)

	err := Validate(context.Background(), c, "./mykit")
	require.Error(t, err)
	require.NotErrorIs(t, err, client.ErrKitRejected)
}
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./kit/... 2>&1 | head -20`
Expected: FAIL — the `kit` package does not exist yet.

- [ ] **Step 3: Add the sentinel**

In `client/errors.go`, inside the existing `var (...)` block at lines 13-22, after `ErrUnexpectedFormat`:

```go
	// ErrKitRejected reports that `sbx kit validate` refused an artifact. The
	// CLI marks every refusal with an "INVALID:" line, whether the cause is a
	// malformed spec.yaml or a source that kit.allowedSources forbids; the
	// wrapped message carries which. Verified 2026-07-27 against sbx v0.37.0.
	ErrKitRejected = errors.New("kit rejected by sbx")
```

- [ ] **Step 4: Write `kit/kit.go`**

```go
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
```

- [ ] **Step 5: Run the tests to verify they pass**

Run: `go test ./kit/... ./client/... -v 2>&1 | tail -30`
Expected: PASS for all six tests.

- [ ] **Step 6: Commit**

```bash
git add client/errors.go kit/kit.go kit/kit_test.go
git commit -m "feat(kit): add Inspect and Validate

- Add kit package with Info and Manifest types.
- Type strings, keep structs as raw JSON, per ADR 0005.
- Add client.ErrKitRejected for an INVALID: refusal."
```

---

### Task 2: `Pack`, `Push` and `Pull`

**Files:**
- Modify: `kit/kit.go`
- Test: `kit/kit_test.go`

**Interfaces:**
- Consumes: the `fakeClient` helper and `client.Client` from Task 1.
- Produces: `kit.Pack(ctx, c, dir, out string) error`; `kit.Push(ctx, c, dir, ref string) error`; `kit.Pull(ctx, c, ref, out string) error`.

- [ ] **Step 1: Write the failing tests**

Append to `kit/kit_test.go`:

```go
// out is a required argument so the SDK always passes -o. Left to itself the
// CLI derives a name from the kit and writes it into the calling process's
// working directory.
func TestPack_AlwaysPassesOutputFlag(t *testing.T) {
	argFile := filepath.Join(t.TempDir(), "args.txt")
	c := fakeClient(t, argFile, "Packed artifact to /tmp/out.zip", "", 0)

	require.NoError(t, Pack(context.Background(), c, "./mykit", "/tmp/out.zip"))

	args, err := os.ReadFile(argFile)
	require.NoError(t, err)
	require.Contains(t, string(args), "kit pack ./mykit -o /tmp/out.zip")
}

func TestPull_AlwaysPassesOutputFlag(t *testing.T) {
	argFile := filepath.Join(t.TempDir(), "args.txt")
	c := fakeClient(t, argFile, "", "", 0)

	require.NoError(t, Pull(context.Background(), c, "ghcr.io/org/kit:1.0", "/tmp/out.tar.gz"))

	args, err := os.ReadFile(argFile)
	require.NoError(t, err)
	require.Contains(t, string(args), "kit pull ghcr.io/org/kit:1.0 -o /tmp/out.tar.gz")
}

// `sbx kit push DIRECTORY REFERENCE` — directory first. Reversing them would
// try to push a registry reference as a directory.
func TestPush_PassesDirectoryBeforeReference(t *testing.T) {
	argFile := filepath.Join(t.TempDir(), "args.txt")
	c := fakeClient(t, argFile, "", "", 0)

	require.NoError(t, Push(context.Background(), c, "./mykit", "ghcr.io/org/kit:1.0"))

	args, err := os.ReadFile(argFile)
	require.NoError(t, err)
	require.Contains(t, string(args), "kit push ./mykit ghcr.io/org/kit:1.0")
}

func TestPack_NonZeroExitIsAnError(t *testing.T) {
	c := fakeClient(t, "", "", "ERROR: pack: invalid artifact: spec.yaml is required", 1)

	err := Pack(context.Background(), c, "./mykit", "/tmp/out.zip")
	require.Error(t, err)
	require.NotErrorIs(t, err, client.ErrKitRejected)
}
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./kit/... -run 'TestPack|TestPull|TestPush' 2>&1 | head -20`
Expected: FAIL — `undefined: Pack`, `undefined: Push`, `undefined: Pull`.

- [ ] **Step 3: Implement the three functions**

Append to `kit/kit.go`:

```go
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
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test ./kit/... -v 2>&1 | tail -30`
Expected: PASS for all ten tests.

- [ ] **Step 5: Commit**

```bash
git add kit/kit.go kit/kit_test.go
git commit -m "feat(kit): add Pack, Push and Pull

- Output path is a required argument, so -o is always passed.
- Push and Pull are unverified against a real registry; the doc
  comments say so."
```

---

### Task 3: `absLocal` and `AddKit`

**Files:**
- Create: `sandbox/kit.go`
- Test: `sandbox/kit_test.go`

**Interfaces:**
- Consumes: `sandbox.NewForTest(c, name)` (`sandbox/sandbox.go:72`), `s.cli.Runner()`.
- Produces: `(*Sandbox).AddKit(ctx, ref string) error`; unexported `absLocal(ref string) string`, used again by Task 5.

- [ ] **Step 1: Write the failing tests**

Create `sandbox/kit_test.go`:

```go
package sandbox

import (
	"context"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"github.com/squall-chua/sbx-go-sdk/client"
	"github.com/stretchr/testify/require"
)

// kitFakeClient returns a client whose fake sbx binary records its args.
func kitFakeClient(t *testing.T, argFile string) *client.Client {
	t.Helper()
	sock := filepath.Join(t.TempDir(), "d.sock")
	l, err := net.Listen("unix", sock)
	require.NoError(t, err)
	srv := &http.Server{Handler: http.NewServeMux()}
	go srv.Serve(l)
	t.Cleanup(func() { srv.Close() })
	bin := filepath.Join(t.TempDir(), "sbx")
	script := "#!/bin/sh\nprintf '%s\\n' \"$*\" >> " + argFile + "\nexit 0\n"
	require.NoError(t, os.WriteFile(bin, []byte(script), 0o755))
	c, err := client.New(context.Background(), client.WithSocketPath(sock), client.WithBinaryPath(bin))
	require.NoError(t, err)
	return c
}

// The daemon records the kit list verbatim and re-resolves it on every later
// add, resolving a relative path against its own working directory rather
// than the caller's. A relative path therefore records one that does not
// exist and breaks every subsequent add on that sandbox.
func TestAbsLocal_ExistingRelativePathBecomesAbsolute(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.Mkdir(filepath.Join(dir, "mykit"), 0o755))
	t.Chdir(dir)

	got := absLocal("./mykit")

	require.True(t, filepath.IsAbs(got), "want an absolute path, got %q", got)
	require.Equal(t, filepath.Join(dir, "mykit"), filepath.Clean(got))
}

func TestAbsLocal_NonPathReferencesPassThroughUntouched(t *testing.T) {
	t.Chdir(t.TempDir())

	for _, ref := range []string{
		"ghcr.io/myorg/mcp-postgres:1.0",
		"git+https://github.com/org/kits.git#dir=mcp-postgres",
		"./does-not-exist",
	} {
		require.Equal(t, ref, absLocal(ref))
	}
}

func TestAddKit_PassesSandboxNameThenReference(t *testing.T) {
	argFile := filepath.Join(t.TempDir(), "args.txt")
	c := kitFakeClient(t, argFile)
	sb := NewForTest(c, "my-sandbox")

	require.NoError(t, sb.AddKit(context.Background(), "ghcr.io/org/kit:1.0"))

	args, err := os.ReadFile(argFile)
	require.NoError(t, err)
	require.Contains(t, string(args), "kit add my-sandbox ghcr.io/org/kit:1.0")
}

func TestAddKit_AbsolutisesALocalDirectory(t *testing.T) {
	argFile := filepath.Join(t.TempDir(), "args.txt")
	c := kitFakeClient(t, argFile)
	sb := NewForTest(c, "my-sandbox")

	dir := t.TempDir()
	require.NoError(t, os.Mkdir(filepath.Join(dir, "mykit"), 0o755))
	t.Chdir(dir)

	require.NoError(t, sb.AddKit(context.Background(), "./mykit"))

	args, err := os.ReadFile(argFile)
	require.NoError(t, err)
	require.NotContains(t, string(args), "./mykit",
		"a relative path reaches the daemon and poisons the recorded kit list")
	require.Contains(t, string(args), filepath.Join(dir, "mykit"))
}
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./sandbox/... -run 'TestAbsLocal|TestAddKit' 2>&1 | head -20`
Expected: FAIL — `undefined: absLocal`, `sb.AddKit undefined`.

- [ ] **Step 3: Write `sandbox/kit.go`**

```go
package sandbox

import (
	"context"
	"os"
	"path/filepath"
)

// absLocal makes ref absolute when it names something that exists on disk,
// and returns it untouched otherwise.
//
// The daemon records a sandbox's kit list verbatim and re-resolves the whole
// list on every later add, resolving a relative path against its OWN working
// directory rather than the caller's. The add still reports success and the
// kit is applied, so the damage is invisible until a later, unrelated add on
// the same sandbox fails with "re-resolve original kit 0 (...): path does not
// exist" — after which that sandbox can take no kits at all. Verified
// 2026-07-27 against sbx v0.37.0 for both `kit add` and `create --kit`.
//
// The stat is a fact check, not a guess at reference grammar: an OCI
// reference or a git URL does not stat, so it passes through and the CLI
// stays the authority on what it is.
func absLocal(ref string) string {
	if _, err := os.Stat(ref); err != nil {
		return ref
	}
	abs, err := filepath.Abs(ref)
	if err != nil {
		return ref
	}
	return abs
}

// AddKit adds a kit artifact to an existing sandbox (`sbx kit add`).
//
// ref may be a local directory, a ZIP file, a git repository, or an OCI
// reference. A local path is made absolute first; see absLocal for why.
//
// The container is recreated with the kit appended to the sandbox's kit list.
// Kit-owned volumes, such as agent session state, survive the swap.
// Bind-mounted workspaces keep their host mount, and --clone sandboxes keep
// their in-container tree via a named workspace volume that reattaches.
//
// AddKit applies only part of a kit. The CLI refuses, before touching
// anything, any kit declaring a field the recreate flow does not implement:
//
//	applied: environment.variables, caps.network, commands.install, agentContext
//	refused: commands.startup, commands.initFiles, credentials
//	         (including environment.proxyManaged), publishedPorts, volumes
//
// The remedy in each case is to recreate the sandbox with WithKit, which the
// CLI's own message names. Verified 2026-07-27 against sbx v0.37.0; the list
// is worded "does not yet" upstream and is expected to shrink.
//
// The CLI also refuses a sandbox using a legacy git worktree, and one created
// before the kit-add recreate feature shipped. Neither refusal is classified
// here: both arrive as the CLI's own error, carrying its explanation.
func (s *Sandbox) AddKit(ctx context.Context, ref string) error {
	r, err := s.cli.Runner()
	if err != nil {
		return err
	}
	_, err = r.Capture(ctx, nil, "kit", "add", s.info.Name, absLocal(ref))
	return err
}
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test ./sandbox/... -run 'TestAbsLocal|TestAddKit' -v 2>&1 | tail -20`
Expected: PASS for all four tests.

- [ ] **Step 5: Commit**

```bash
git add sandbox/kit.go sandbox/kit_test.go
git commit -m "feat(sandbox): add AddKit

- Make a local kit path absolute before passing it on. A relative
  path is resolved by the daemon against its own working directory,
  which records a path that does not exist and breaks every later add.
- Document which kit fields kit add applies and which it refuses."
```

---

### Task 4: `Kits`

**Files:**
- Modify: `sandbox/kit.go`
- Test: `sandbox/kit_test.go`

**Interfaces:**
- Consumes: `(*Sandbox).Inspect(ctx)` (`sandbox/sandbox.go:60`), `api.SandboxInfo.Labels *map[string]string`, `client.ErrUnexpectedFormat`.
- Produces: `(*Sandbox).Kits(ctx) ([]string, error)`.

- [ ] **Step 1: Write the failing tests**

Append to `sandbox/kit_test.go`:

```go
// The kit list is a container label, not a SandboxInfo field: sandboxapi's
// SandboxInfo has no kit field in DWARF. The label value is a JSON string
// array. Verified 2026-07-27 against sandboxd 0.24.0.
func TestKits_DecodesTheLabelJSONArray(t *testing.T) {
	c := stubClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"id":"x","name":"my-sandbox","status":"running","workspace":"/ws",
			"labels":{"com.docker.sandbox.kits":"[\"/abs/a\",\"/abs/b\"]"}}`))
	}))
	sb := NewForTest(c, "my-sandbox")

	kits, err := sb.Kits(context.Background())

	require.NoError(t, err)
	require.Equal(t, []string{"/abs/a", "/abs/b"}, kits)
}

func TestKits_MissingLabelIsEmptyNotAnError(t *testing.T) {
	c := stubClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"id":"x","name":"my-sandbox","status":"running","workspace":"/ws",
			"labels":{"com.docker.sandbox.agent":"shell"}}`))
	}))
	sb := NewForTest(c, "my-sandbox")

	kits, err := sb.Kits(context.Background())

	require.NoError(t, err)
	require.Empty(t, kits)
}

func TestKits_MalformedLabelIsUnexpectedFormat(t *testing.T) {
	c := stubClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"id":"x","name":"my-sandbox","status":"running","workspace":"/ws",
			"labels":{"com.docker.sandbox.kits":"not json"}}`))
	}))
	sb := NewForTest(c, "my-sandbox")

	_, err := sb.Kits(context.Background())

	require.ErrorIs(t, err, client.ErrUnexpectedFormat)
}
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./sandbox/... -run TestKits 2>&1 | head -20`
Expected: FAIL — `sb.Kits undefined`.

- [ ] **Step 3: Implement `Kits`**

Add `"encoding/json"` and `"fmt"` to the imports of `sandbox/kit.go`, plus
`"github.com/squall-chua/sbx-go-sdk/client"`. Then append:

```go
// kitsLabel is where the daemon records a sandbox's kit list.
const kitsLabel = "com.docker.sandbox.kits"

// Kits returns the kit references recorded on the sandbox, in the order they
// were added.
//
// The list is read from the com.docker.sandbox.kits label, which the daemon
// writes as a JSON string array; SandboxInfo has no kit field of its own.
// Verified 2026-07-27 against sandboxd 0.24.0.
//
// Kits refreshes the sandbox first. The handle's cached info is a snapshot
// taken at Get or Create, and AddKit recreates the container, so a cached
// read would report the list as it was before the add.
//
// A sandbox with no kits returns an empty slice. If upstream renames the
// label, this returns empty rather than an error — a silent answer, and a
// known risk of reading a label instead of a documented field.
func (s *Sandbox) Kits(ctx context.Context) ([]string, error) {
	info, err := s.Inspect(ctx)
	if err != nil {
		return nil, err
	}
	if info.Labels == nil {
		return nil, nil
	}
	raw, ok := (*info.Labels)[kitsLabel]
	if !ok || raw == "" {
		return nil, nil
	}
	var kits []string
	if err := json.Unmarshal([]byte(raw), &kits); err != nil {
		return nil, fmt.Errorf("%w: %s label: %v", client.ErrUnexpectedFormat, kitsLabel, err)
	}
	return kits, nil
}
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test ./sandbox/... -v 2>&1 | tail -30`
Expected: PASS, including the pre-existing sandbox tests.

- [ ] **Step 5: Commit**

```bash
git add sandbox/kit.go sandbox/kit_test.go
git commit -m "feat(sandbox): add Kits

- Read the kit list from the com.docker.sandbox.kits label, which
  holds a JSON string array. SandboxInfo has no kit field.
- Refresh before reading, because AddKit recreates the container."
```

---

### Task 5: `WithKit`

**Files:**
- Modify: `sandbox/definition.go:9-24` (struct) and `:56-58` (arg building)
- Modify: `sandbox/options.go`
- Test: `sandbox/options_test.go`

**Interfaces:**
- Consumes: `absLocal` from Task 3; `Definition`, `newDefinition(opts ...Option)`, `(*Definition).toCreateArgs()`.
- Produces: `WithKit(refs ...string) Option`.

- [ ] **Step 1: Write the failing tests**

Append to `sandbox/options_test.go`:

```go
func TestWithKit_EmitsRepeatedFlagOnCreate(t *testing.T) {
	d := newDefinition(
		WithAgent("shell"),
		WithWorkspace("/ws"),
		WithKit("ghcr.io/org/a:1.0"),
		WithKit("ghcr.io/org/b:1.0", "ghcr.io/org/c:1.0"),
	)

	args, err := d.toCreateArgs()
	require.NoError(t, err)

	joined := strings.Join(args, " ")
	require.Contains(t, joined, "--kit ghcr.io/org/a:1.0")
	require.Contains(t, joined, "--kit ghcr.io/org/b:1.0")
	require.Contains(t, joined, "--kit ghcr.io/org/c:1.0")
}

func TestWithKit_AbsentWhenUnset(t *testing.T) {
	d := newDefinition(WithAgent("shell"), WithWorkspace("/ws"))
	args, err := d.toCreateArgs()
	require.NoError(t, err)
	require.NotContains(t, args, "--kit")
}

// create --kit records the kit list the same way kit add does, so a relative
// path is resolved against the daemon's working directory and recorded wrong.
func TestWithKit_AbsolutisesALocalDirectory(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.Mkdir(filepath.Join(dir, "mykit"), 0o755))
	t.Chdir(dir)

	d := newDefinition(WithAgent("shell"), WithWorkspace("/ws"), WithKit("./mykit"))
	args, err := d.toCreateArgs()
	require.NoError(t, err)

	joined := strings.Join(args, " ")
	require.NotContains(t, joined, "--kit ./mykit")
	require.Contains(t, joined, "--kit "+filepath.Join(dir, "mykit"))
}
```

Add `"os"`, `"path/filepath"` and `"strings"` to that file's imports if they are not already present.

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./sandbox/... -run TestWithKit 2>&1 | head -20`
Expected: FAIL — `undefined: WithKit`.

- [ ] **Step 3: Add the field, the option, and the argument**

In `sandbox/definition.go`, add to the `Definition` struct after `publish`:

```go
	kits       []string // --kit refs, applied at create time
```

In the same file, inside `toCreateArgs`, immediately after the `d.publish` loop:

```go
	for _, ref := range d.kits {
		args = append(args, "--kit", absLocal(ref))
	}
```

In `sandbox/options.go`, after `WithPublish`:

```go
// WithKit attaches kit artifacts at creation time (`--kit`, EXPERIMENTAL
// upstream). Each ref may be a local directory, a ZIP file, or an OCI
// reference. May be called once with several refs or repeatedly.
//
// A local path is made absolute when the argument vector is built; the daemon
// records the kit list verbatim and resolves a relative path against its own
// working directory, which would record one that does not exist.
//
// Prefer WithKit over AddKit for anything beyond a trivial kit: AddKit
// refuses any kit declaring credentials, publishedPorts, volumes,
// commands.startup or commands.initFiles, whereas creation applies all of
// them.
//
// Refs are otherwise passed straight through without validation; the CLI is
// the authority on the grammar and rejects a malformed ref itself.
func WithKit(refs ...string) Option {
	return func(d *Definition) { d.kits = append(d.kits, refs...) }
}
```

- [ ] **Step 4: Run the full unit suite**

Run: `go build ./... && go vet ./... && go test ./... 2>&1 | tail -20`
Expected: PASS for every package. The `internal/integration` package is
build-tagged out and will report `[no test files]`.

- [ ] **Step 5: Commit**

```bash
git add sandbox/definition.go sandbox/options.go sandbox/options_test.go
git commit -m "feat(sandbox): add WithKit create option

- Emit one --kit per reference, like WithPublish.
- Make local paths absolute, for the same reason AddKit does."
```

---

### Task 6: Live integration tests

**Files:**
- Create: `internal/integration/testdata/fixture-kit/spec.yaml`
- Create: `internal/integration/kit_test.go`

**Interfaces:**
- Consumes: `kit.Validate`, `kit.Inspect`, `kit.Pack` (Tasks 1-2); `sandbox.Create`, `WithAgent`, `WithWorkspace`, `WithName`, `(*Sandbox).AddKit`, `(*Sandbox).Kits`, `(*Sandbox).Remove` (Tasks 3-5).
- Produces: nothing consumed by later tasks.

- [ ] **Step 1: Write the fixture**

Create `internal/integration/testdata/fixture-kit/spec.yaml`:

```yaml
# A minimal kit that `sbx kit add` accepts.
#
# DO NOT add credentials, publishedPorts, volumes, commands.startup or
# commands.initFiles here. The kit-add recreate flow refuses all five
# pre-flight, and TestSmoke_AddKitRecordsAbsolutePath would then fail for a
# reason unrelated to what it tests.
schemaVersion: "2"
kind: mixin
name: fixture-kit
version: 0.1.0
description: fixture for sbx-go-sdk integration tests
environment:
  variables:
    SBX_GO_SDK_FIXTURE: "1"
caps:
  network:
    allow:
      - api.example.com
```

- [ ] **Step 2: Write the tests**

Create `internal/integration/kit_test.go`:

```go
//go:build integration

package integration

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/squall-chua/sbx-go-sdk/client"
	"github.com/squall-chua/sbx-go-sdk/kit"
	"github.com/squall-chua/sbx-go-sdk/sandbox"
	"github.com/stretchr/testify/require"
)

// fixtureKit copies testdata/fixture-kit into a fresh temp directory and
// returns that directory. Copying keeps every test free to chdir without
// disturbing the repo.
func fixtureKit(t *testing.T) string {
	t.Helper()
	src := filepath.Join("testdata", "fixture-kit", "spec.yaml")
	body, err := os.ReadFile(src)
	require.NoError(t, err)

	dir := filepath.Join(t.TempDir(), "fixture-kit")
	require.NoError(t, os.MkdirAll(dir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "spec.yaml"), body, 0o644))
	return dir
}

// Requires a real sbx on PATH. Needs no daemon, no sandbox and no network.
func TestSmoke_KitValidateInspectPack(t *testing.T) {
	ctx := context.Background()
	c, err := client.New(ctx, client.WithAutoStart())
	require.NoError(t, err)

	dir := fixtureKit(t)

	require.NoError(t, kit.Validate(ctx, c, dir))

	info, err := kit.Inspect(ctx, c, dir)
	require.NoError(t, err)
	require.Equal(t, "fixture-kit", info.Manifest.Name)
	require.Equal(t, "mixin", info.Manifest.Kind)
	require.Equal(t, "2", info.Manifest.SchemaVersion)
	require.NotEmpty(t, info.Caps, "caps.network should survive as raw JSON")

	out := filepath.Join(t.TempDir(), "fixture-kit.zip")
	require.NoError(t, kit.Pack(ctx, c, dir, out))
	st, err := os.Stat(out)
	require.NoError(t, err)
	require.Greater(t, st.Size(), int64(0))
}

// A malformed kit must come back as ErrKitRejected, and a missing path must
// not — `sbx kit validate` exits 1 for both, and only the "INVALID:" prefix
// separates them.
func TestSmoke_KitValidateRejectionIsClassified(t *testing.T) {
	ctx := context.Background()
	c, err := client.New(ctx, client.WithAutoStart())
	require.NoError(t, err)

	bad := filepath.Join(t.TempDir(), "bad-kit")
	require.NoError(t, os.MkdirAll(bad, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(bad, "spec.yaml"),
		[]byte("schemaVersion: \"2\"\nkind: mixin\nname: bad\nversion: 0.1.0\nnope: true\n"), 0o644))

	err = kit.Validate(ctx, c, bad)
	require.ErrorIs(t, err, client.ErrKitRejected)

	err = kit.Validate(ctx, c, filepath.Join(t.TempDir(), "no-such-directory"))
	require.Error(t, err)
	require.NotErrorIs(t, err, client.ErrKitRejected)
}

// AddKit must record an ABSOLUTE path. A relative one is resolved by the
// daemon against its own working directory, silently recording a path that
// does not exist and breaking every later add on that sandbox. A unit test
// can only prove the SDK passed an absolute path; only this proves the
// daemon recorded the right one.
//
// Creates and removes its own sandbox. Recreates a container, so it is slow.
func TestSmoke_AddKitRecordsAbsolutePath(t *testing.T) {
	ctx := context.Background()
	c, err := client.New(ctx, client.WithAutoStart())
	require.NoError(t, err)

	sb, err := sandbox.Create(ctx, c,
		sandbox.WithAgent("shell"),
		sandbox.WithName("sdk-kit-addkit-test"),
		sandbox.WithWorkspace(t.TempDir()),
	)
	require.NoError(t, err)
	t.Cleanup(func() { sb.Remove(ctx) })

	dir := fixtureKit(t)
	t.Chdir(filepath.Dir(dir))

	// Deliberately relative. This is the call that used to poison the list.
	require.NoError(t, sb.AddKit(ctx, "./fixture-kit"))

	kits, err := sb.Kits(ctx)
	require.NoError(t, err)
	require.Contains(t, kits, dir,
		"the daemon recorded a path resolved against its own working directory")
}
```

- [ ] **Step 3: Confirm the package still builds under the tag**

Run: `go vet -tags integration ./internal/integration/...`
Expected: no output.

- [ ] **Step 4: Run the live tests**

Run: `go test -tags integration ./internal/integration/... -run 'TestSmoke_Kit|TestSmoke_AddKit' -v 2>&1 | tail -40`
Expected: PASS for all three. `TestSmoke_AddKitRecordsAbsolutePath` takes
roughly a minute, because adding a kit recreates the container.

If `TestSmoke_KitValidateInspectPack` fails on the allowlist, the host's
`kit.allowedSources` has been narrowed; the fixture is a local directory and
should not be subject to it. Report it rather than widening the setting.

- [ ] **Step 5: Confirm no stray sandbox is left behind**

Run: `sbx ls`
Expected: only the two `jzrdxd6r3pw3l6py.*` sandboxes belonging to
`sbx-swarm-node`. If `sdk-kit-addkit-test` is still listed, remove it with
`sbx rm -f sdk-kit-addkit-test` and fix the `t.Cleanup`.

- [ ] **Step 6: Commit**

```bash
git add internal/integration/kit_test.go internal/integration/testdata/fixture-kit/spec.yaml
git commit -m "test(kit): add live integration tests

- Validate, Inspect and Pack against a fixture kit; no daemon needed.
- Check that an INVALID: refusal maps to ErrKitRejected and a
  missing path does not.
- Check that AddKit with a relative reference records an absolute
  path, which is what absLocal exists to guarantee."
```

---

### Task 7: Documentation

**Files:**
- Modify: `docs/sbx-version-coverage.md:36-37`, `:44-45`, and the table's end
- Modify: `REVERSE_ENGINEERING.md` (near line 244, the `SandboxCreateRequest` entry)
- Modify: `README.md` (limitations list)

**Interfaces:**
- Consumes: every public name from Tasks 1-5.
- Produces: nothing.

- [ ] **Step 1: Update the coverage table**

In `docs/sbx-version-coverage.md`, replace the row at line 37:

```markdown
| OCI v2 kit artifact streaming | v0.34.0 | `kit.Push`, `kit.Pull` | covered — format follows the kit's `schemaVersion`; unverified against a live registry |
```

Replace the rows at lines 44-45:

```markdown
| `sbx inspect` (kits, auth mode, active sessions) | v0.35.0 | `Sandbox.Kits` | partly covered — kits read from the `com.docker.sandbox.kits` label; auth mode and active sessions remain gaps, and `api.SandboxInfo` carries neither |
| `sbx kit add` recreates container, applies kit policy | v0.35.0 | `Sandbox.AddKit` | covered — applies `environment.variables`, `caps.network`, `commands.install`, `agentContext`; the CLI refuses kits declaring `credentials`, `publishedPorts`, `volumes`, `commands.startup` or `commands.initFiles` |
```

Add these rows at the end of the table:

```markdown
| `sbx kit inspect` / `validate` / `pack` | v0.34.0 | `kit.Inspect`, `kit.Validate`, `kit.Pack` | covered |
| `sbx create --kit` | v0.34.0 | `sandbox.WithKit` | covered |
```

- [ ] **Step 2: Record the schema provenance**

In `REVERSE_ENGINEERING.md`, near the `SandboxCreateRequest` entry around
line 244, add:

```markdown
### Kit spec schema

The `spec.yaml` schema is `github.com/docker/sbx-kits-contrib/spec`, reachable from the `sbx`
binary's DWARF. Two types matter:

- **`SpecFile`** — what a kit author writes. 24 fields, eight named `Legacy*`. `credentials` and
  `volumes` each accept either a list or a legacy map through custom unmarshalers. YAML decoding is
  strict: an unknown key fails validation.
- **`Artifact`** — what `sbx kit inspect --json` reports. 14 fields. Modelled as `kit.Info`, not
  `kit.Artifact`, because it omits the `files/` payload that `kit pack` writes into the ZIP.

The two shapes differ. Flat `template` / `binary` / `runOptions` keys are rejected on input ("use
the 'sandbox:' block instead") yet emitted on output, derived from the `sandbox:` block.

`sandboxapi.SandboxInfo` has **no** kit field — 14 fields in DWARF, matching `types_gen.go` exactly.
A sandbox's kit list is the container label `com.docker.sandbox.kits`, holding a JSON string array,
returned inside `labels` by `GET /sandbox/{name}`.

The binary also carries `/sandbox/:name/swap-container`, the endpoint behind the `kit add` recreate.
The SDK does not call it: `kit add` re-resolves the recorded kit list and runs the accept/refuse
check around it, and shelling out gets both for free.
```

- [ ] **Step 3: Update the README limitations**

Add to the README's limitations list:

```markdown
- **`kit.Inspect` does not report a kit's files.** `sbx kit inspect --json` omits the `files/`
  payload, although `kit.Pack` writes it into the artifact. `Info` is a report, not the kit.
- **`kit.Push` and `kit.Pull` are unverified.** Neither has been run against a real OCI registry;
  both ship with argument-vector unit tests only.
- **Local kit paths are made absolute.** `AddKit` and `WithKit` rewrite a reference that exists on
  disk. The daemon records the kit list verbatim and resolves a relative path against its own
  working directory, which would record a path that does not exist and break every later add.
- **`Sandbox.AddKit` cannot apply every kit.** The CLI refuses, pre-flight, any kit declaring
  `credentials`, `publishedPorts`, `volumes`, `commands.startup` or `commands.initFiles`. Use
  `sandbox.WithKit` at creation for those.
- **`Sandbox.Kits` reads a label.** If upstream renames `com.docker.sandbox.kits`, it returns empty
  rather than an error.
```

- [ ] **Step 4: Verify the drift gate still passes**

Run: `go test -tags integration ./internal/integration/... -run TestContract 2>&1 | tail -20`
Expected: PASS. `TestContract_VersionAlignment` reads the coverage table.

- [ ] **Step 5: Commit**

```bash
git add docs/sbx-version-coverage.md REVERSE_ENGINEERING.md README.md
git commit -m "docs: record kit coverage and the kit spec schema

- Close the two deferred kit rows and the sbx inspect kits gap.
- Record where the spec.yaml schema lives and that SandboxInfo
  carries no kit field.
- List the five kit limitations in the README."
```

---

## Final verification

- [ ] `go build ./... && go vet ./...` — clean
- [ ] `go test ./...` — all unit tests pass
- [ ] `go test -tags integration ./internal/integration/...` — all live tests pass
- [ ] `sbx ls` shows only the two `jzrdxd6r3pw3l6py.*` sandboxes
- [ ] `git log --oneline main..HEAD` shows seven commits
- [ ] Nothing pushed; `main` untouched

## Known gaps this plan does not close

Carried from the spec's "Not verified" section, so a reviewer does not mistake
them for oversights:

- `Push` and `Pull` against a real registry — no registry is reachable.
- Whether `kit add` behaves differently on a stopped sandbox.
- Git references — `kit.allowedSources` refused them before they resolved.
- Legacy `schemaVersion: "1"` kits.
- Whether `create --kit` accepts git references; its help says "directory, ZIP,
  or OCI" while `kit add`'s help adds git.
- The `build` JSON tag on `Manifest`, inferred from DWARF rather than observed.
