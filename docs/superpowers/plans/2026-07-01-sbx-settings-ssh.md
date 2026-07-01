# sbx v0.34.0 `settings` + `ssh` Packages Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add two new shell-out SDK packages — `settings` (read/write sandboxd settings via `sbx settings … --json`) and `ssh` (manage the experimental SSH endpoint, composed over `settings`).

**Architecture:** Both packages shell out through `client.Client.Runner()` exactly like `policy`/`secret`. Read paths use `--json` for a stable structured contract (no table parsing). `ssh` imports `settings` for the feature flag and port; it never re-implements shell-out. Errors are the raw `*client.CLIError` from the shell-out; a malformed `--json` payload is wrapped with `client.ErrUnexpectedFormat`.

**Tech Stack:** Go 1.25, `encoding/json`, `github.com/stretchr/testify`. No new dependencies.

## Global Constraints

- Module path: `github.com/squall-chua/sbx-go-sdk`; Go floor **1.25** (`go.mod`).
- Shell-out only via `c.Runner()` then `r.Capture(ctx, nil, args...)` — mirror `policy`/`secret`; do **not** add REST calls.
- Shell-out failures return the raw error from `Capture` (a `*client.CLIError`) unchanged. Only wrap when `--json` output fails to parse, using `client.ErrUnexpectedFormat`.
- `Set`/`Unset`/`Enable`/`Disable`/`Setup` are **fire-and-forget** — no polling, no sleeps; document the ~5s daemon hot-reload lag in doc comments.
- Integration tests (`//go:build integration`) for these packages are **read-only** and use **structural** assertions (never exact default values like `2222`).
- Work on branch `feat/sbx-v0.34.0-settings-ssh`. Run `gofmt -w` on every file before committing.

---

### Task 1: `settings` — `Setting` type + `Bool()`/`Text()` accessors

**Files:**
- Create: `settings/settings.go`
- Test: `settings/settings_test.go`

**Interfaces:**
- Consumes: nothing (pure type).
- Produces:
  - `type Setting struct { Key string; Value json.RawMessage; Type string; Source string; Description string }` with JSON tags `key`/`value`/`type`/`source`/`description`.
  - `func (s Setting) Bool() (bool, error)`
  - `func (s Setting) Text() string`

- [ ] **Step 1: Write the failing test**

Create `settings/settings_test.go`:

```go
package settings

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSettingBool(t *testing.T) {
	s := Setting{Key: "feature.x", Value: json.RawMessage(`false`), Type: "bool"}
	b, err := s.Bool()
	require.NoError(t, err)
	require.False(t, b)

	_, err = Setting{Key: "ssh.port", Value: json.RawMessage(`2222`)}.Bool()
	require.Error(t, err) // not a bool
}

func TestSettingText(t *testing.T) {
	require.Equal(t, "shell", Setting{Value: json.RawMessage(`"shell"`)}.Text())      // string -> unquoted
	require.Equal(t, "2222", Setting{Value: json.RawMessage(`2222`)}.Text())          // number -> as-is
	require.Equal(t, `["docker.io/"]`, Setting{Value: json.RawMessage(`["docker.io/"]`)}.Text()) // array -> raw JSON
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./settings/ -run 'TestSetting' -v`
Expected: FAIL — `undefined: Setting` (package/type does not exist yet).

- [ ] **Step 3: Write the minimal implementation**

Create `settings/settings.go`:

```go
// Package settings reads and writes persistent sandboxd settings by shelling out
// to `sbx settings` (which edits settings.json; the daemon hot-reloads it within
// ~5s). Read paths use `--json` for a stable structured contract; mutations are
// fire-and-forget — Set/Unset return once the file is written, before the daemon
// reload. Shell-out failures return the raw *client.CLIError, as in policy/secret.
package settings

import (
	"encoding/json"
	"fmt"
	"strings"
)

// Setting is one entry from `sbx settings {get,list} --json`.
type Setting struct {
	Key         string          `json:"key"`
	Value       json.RawMessage `json:"value"` // typed JSON: bool/int/float/string/array/object
	Type        string          `json:"type"`  // "bool"|"int"|"float"|"string"|"json"
	Source      string          `json:"source"`
	Description string          `json:"description"`
}

// Bool decodes the value as a JSON boolean.
func (s Setting) Bool() (bool, error) {
	var b bool
	if err := json.Unmarshal(s.Value, &b); err != nil {
		return false, fmt.Errorf("setting %q value %s is not a bool: %w", s.Key, s.Value, err)
	}
	return b, nil
}

// Text renders the value as text: an unquoted scalar (string/number/bool) or the
// raw JSON for arrays/objects.
func (s Setting) Text() string {
	var str string
	if json.Unmarshal(s.Value, &str) == nil {
		return str
	}
	return strings.TrimSpace(string(s.Value))
}
```

- [ ] **Step 4: Run the test to verify it passes**

Run: `gofmt -w settings/ && go test ./settings/ -run 'TestSetting' -v`
Expected: PASS (both tests).

- [ ] **Step 5: Commit**

```bash
git add settings/settings.go settings/settings_test.go
git commit -m "feat(settings): Setting type with Bool/Text accessors"
```

---

### Task 2: `settings` — `List` + `Get` (read via `--json`)

**Files:**
- Modify: `settings/settings.go`
- Test: `settings/settings_test.go`

**Interfaces:**
- Consumes: `client.Client.Runner()`, `Runner.Capture(ctx, nil, args...)`, `client.ErrUnexpectedFormat`, `client.New`, `client.WithSocketPath`, `client.WithBinaryPath`.
- Produces:
  - `func List(ctx context.Context, c *client.Client) ([]Setting, error)` — runs `settings list --json`.
  - `func Get(ctx context.Context, c *client.Client, key string) (*Setting, error)` — runs `settings get --json <key>`.
  - test helper `newFakeSbx(t, exit int, stdout, stderr string) (*client.Client, string)` (returns client + args-file path).

- [ ] **Step 1: Write the failing test**

Add to `settings/settings_test.go` (add imports `context`, `net`, `net/http`, `os`, `path/filepath`, `strconv`, and `github.com/squall-chua/sbx-go-sdk/client`):

```go
// newFakeSbx builds a Client whose sbx binary is a shell script: it records its
// args (space-joined) to the returned file, prints stdout, prints stderr, and
// exits with the given code. A stub unix socket satisfies client.New.
func newFakeSbx(t *testing.T, exit int, stdout, stderr string) (*client.Client, string) {
	t.Helper()
	dir := t.TempDir()
	argFile := filepath.Join(dir, "args.txt")
	sock := filepath.Join(dir, "d.sock")
	l, err := net.Listen("unix", sock)
	require.NoError(t, err)
	srv := &http.Server{Handler: http.NewServeMux()}
	go srv.Serve(l)
	t.Cleanup(func() { srv.Close() })
	bin := filepath.Join(dir, "sbx")
	script := "#!/bin/sh\n" +
		"printf '%s\\n' \"$*\" >> " + argFile + "\n" +
		"cat <<'STDOUT_EOF'\n" + stdout + "\nSTDOUT_EOF\n" +
		"cat >&2 <<'STDERR_EOF'\n" + stderr + "\nSTDERR_EOF\n" +
		"exit " + strconv.Itoa(exit) + "\n"
	require.NoError(t, os.WriteFile(bin, []byte(script), 0o755))
	c, err := client.New(context.Background(), client.WithSocketPath(sock), client.WithBinaryPath(bin))
	require.NoError(t, err)
	return c, argFile
}

func TestList(t *testing.T) {
	out := `[{"key":"ssh.port","value":2222,"type":"int","source":"default","description":"port"},
	         {"key":"feature.ssh","value":{"enabled":false},"type":"json","source":"default","description":"flag"}]`
	c, argFile := newFakeSbx(t, 0, out, "")
	ss, err := List(context.Background(), c)
	require.NoError(t, err)
	require.Len(t, ss, 2)
	require.Equal(t, "ssh.port", ss[0].Key)
	require.Equal(t, "int", ss[0].Type)
	args, _ := os.ReadFile(argFile)
	require.Contains(t, string(args), "settings list --json")
}

func TestGet(t *testing.T) {
	c, argFile := newFakeSbx(t, 0, `{"key":"ssh.port","value":2222,"type":"int","source":"default","description":"port"}`, "")
	s, err := Get(context.Background(), c, "ssh.port")
	require.NoError(t, err)
	require.Equal(t, "ssh.port", s.Key)
	require.Equal(t, "2222", s.Text())
	args, _ := os.ReadFile(argFile)
	require.Contains(t, string(args), "settings get --json ssh.port")
}

func TestGetErrorPropagatesCLIError(t *testing.T) {
	c, _ := newFakeSbx(t, 1, "", "key not defined")
	_, err := Get(context.Background(), c, "nope.key")
	require.Error(t, err)
	var ce *client.CLIError
	require.ErrorAs(t, err, &ce) // raw shell-out error, no sentinel
	require.Equal(t, 1, ce.ExitCode)
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./settings/ -run 'TestList|TestGet' -v`
Expected: FAIL — `undefined: List` / `undefined: Get`.

- [ ] **Step 3: Write the minimal implementation**

Add to `settings/settings.go` — extend the import block to include `context` and `github.com/squall-chua/sbx-go-sdk/client`, then append:

```go
func capture(ctx context.Context, c *client.Client, args ...string) (string, error) {
	r, err := c.Runner()
	if err != nil {
		return "", err
	}
	return r.Capture(ctx, nil, args...)
}

// List returns all settings (`sbx settings list --json`).
func List(ctx context.Context, c *client.Client) ([]Setting, error) {
	out, err := capture(ctx, c, "settings", "list", "--json")
	if err != nil {
		return nil, err
	}
	var ss []Setting
	if err := json.Unmarshal([]byte(out), &ss); err != nil {
		return nil, fmt.Errorf("settings list: %w: %w", client.ErrUnexpectedFormat, err)
	}
	return ss, nil
}

// Get returns one setting (`sbx settings get --json <key>`). An undefined key makes
// the CLI exit non-zero, which surfaces as the raw *client.CLIError.
func Get(ctx context.Context, c *client.Client, key string) (*Setting, error) {
	out, err := capture(ctx, c, "settings", "get", "--json", key)
	if err != nil {
		return nil, err
	}
	var s Setting
	if err := json.Unmarshal([]byte(out), &s); err != nil {
		return nil, fmt.Errorf("settings get %q: %w: %w", key, client.ErrUnexpectedFormat, err)
	}
	return &s, nil
}
```

- [ ] **Step 4: Run the test to verify it passes**

Run: `gofmt -w settings/ && go test ./settings/ -v`
Expected: PASS (all settings tests).

- [ ] **Step 5: Commit**

```bash
git add settings/settings.go settings/settings_test.go
git commit -m "feat(settings): List/Get via sbx settings --json"
```

---

### Task 3: `settings` — `Set` + `Unset` (write)

**Files:**
- Modify: `settings/settings.go`
- Test: `settings/settings_test.go`

**Interfaces:**
- Consumes: `newFakeSbx` (Task 2), `capture` helper's sibling `run`.
- Produces:
  - `func Set(ctx context.Context, c *client.Client, key, value string) error` — runs `settings set <key> <value>`.
  - `func Unset(ctx context.Context, c *client.Client, key string) error` — runs `settings unset <key>`.

- [ ] **Step 1: Write the failing test**

Add to `settings/settings_test.go`:

```go
func TestSetUnsetArgs(t *testing.T) {
	c, argFile := newFakeSbx(t, 0, "", "")
	ctx := context.Background()
	require.NoError(t, Set(ctx, c, "feature.ssh", "true"))
	require.NoError(t, Set(ctx, c, "kit.allowedSources", `["docker.io/"]`))
	require.NoError(t, Unset(ctx, c, "feature.ssh"))
	args, _ := os.ReadFile(argFile)
	lines := string(args)
	require.Contains(t, lines, "settings set feature.ssh true")
	require.Contains(t, lines, `settings set kit.allowedSources ["docker.io/"]`)
	require.Contains(t, lines, "settings unset feature.ssh")
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./settings/ -run 'TestSetUnsetArgs' -v`
Expected: FAIL — `undefined: Set` / `undefined: Unset`.

- [ ] **Step 3: Write the minimal implementation**

Add to `settings/settings.go`:

```go
func run(ctx context.Context, c *client.Client, args ...string) error {
	_, err := capture(ctx, c, args...)
	return err
}

// Set writes an override (`sbx settings set <key> <value>`). The value is parsed by
// the setting's declared type; JSON values (e.g. `["docker.io/"]`) pass through as a
// single argument. Fire-and-forget: returns once the file is written; the daemon
// hot-reloads within ~5s.
func Set(ctx context.Context, c *client.Client, key, value string) error {
	return run(ctx, c, "settings", "set", key, value)
}

// Unset removes an override (`sbx settings unset <key>`), reverting to the env or
// default value. Fire-and-forget (see Set).
func Unset(ctx context.Context, c *client.Client, key string) error {
	return run(ctx, c, "settings", "unset", key)
}
```

- [ ] **Step 4: Run the test to verify it passes**

Run: `gofmt -w settings/ && go test ./settings/ -v`
Expected: PASS (all settings tests).

- [ ] **Step 5: Commit**

```bash
git add settings/settings.go settings/settings_test.go
git commit -m "feat(settings): Set/Unset overrides (fire-and-forget)"
```

---

### Task 4: `ssh` — `Target`, `Args`/`Command`, `Port`, `TargetFor`

**Files:**
- Create: `ssh/ssh.go`
- Test: `ssh/ssh_test.go`

**Interfaces:**
- Consumes: `settings.Get` (Task 2), `client.ErrUnexpectedFormat`, `newFakeSbx`-style helper.
- Produces:
  - `type Target struct { User string; Host string; Port int }`
  - `func (t Target) Args() []string`, `func (t Target) Command() string`
  - `func Port(ctx context.Context, c *client.Client) (int, error)`
  - `func TargetFor(ctx context.Context, c *client.Client, sandboxName string) (Target, error)`
  - test helper `newFakeSbx` (same shape as Task 2, re-declared in this package's test file).

- [ ] **Step 1: Write the failing test**

Create `ssh/ssh_test.go`:

```go
package ssh

import (
	"context"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/squall-chua/sbx-go-sdk/client"
	"github.com/stretchr/testify/require"
)

func newFakeSbx(t *testing.T, exit int, stdout, stderr string) (*client.Client, string) {
	t.Helper()
	dir := t.TempDir()
	argFile := filepath.Join(dir, "args.txt")
	sock := filepath.Join(dir, "d.sock")
	l, err := net.Listen("unix", sock)
	require.NoError(t, err)
	srv := &http.Server{Handler: http.NewServeMux()}
	go srv.Serve(l)
	t.Cleanup(func() { srv.Close() })
	bin := filepath.Join(dir, "sbx")
	script := "#!/bin/sh\n" +
		"printf '%s\\n' \"$*\" >> " + argFile + "\n" +
		"cat <<'STDOUT_EOF'\n" + stdout + "\nSTDOUT_EOF\n" +
		"cat >&2 <<'STDERR_EOF'\n" + stderr + "\nSTDERR_EOF\n" +
		"exit " + strconv.Itoa(exit) + "\n"
	require.NoError(t, os.WriteFile(bin, []byte(script), 0o755))
	c, err := client.New(context.Background(), client.WithSocketPath(sock), client.WithBinaryPath(bin))
	require.NoError(t, err)
	return c, argFile
}

func TestTargetArgsCommand(t *testing.T) {
	tg := Target{User: "mybox", Host: "127.0.0.1", Port: 2222}
	require.Equal(t, []string{"mybox@127.0.0.1", "-p", "2222"}, tg.Args())
	require.Equal(t, "ssh mybox@127.0.0.1 -p 2222", tg.Command())
}

func TestPortAndTargetFor(t *testing.T) {
	c, argFile := newFakeSbx(t, 0, `{"key":"ssh.port","value":2222,"type":"int","source":"default","description":"port"}`, "")
	ctx := context.Background()

	p, err := Port(ctx, c)
	require.NoError(t, err)
	require.Equal(t, 2222, p)

	tg, err := TargetFor(ctx, c, "mybox")
	require.NoError(t, err)
	require.Equal(t, Target{User: "mybox", Host: "127.0.0.1", Port: 2222}, tg)

	args, _ := os.ReadFile(argFile)
	require.Contains(t, string(args), "settings get --json ssh.port")
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./ssh/ -run 'TestTarget|TestPort' -v`
Expected: FAIL — `undefined: Target` / `undefined: Port` / `undefined: TargetFor`.

- [ ] **Step 3: Write the minimal implementation**

Create `ssh/ssh.go`:

```go
// Package ssh manages the sandboxd SSH endpoint (EXPERIMENTAL upstream). It composes
// over the settings package for the feature flag (feature.ssh) and port (ssh.port),
// and shells out to `sbx ssh setup` for local client provisioning. Enabling requires
// experimental features (platform.allowExperimentalFeatures, default true); this
// package does not modify that host-wide setting.
package ssh

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"

	"github.com/squall-chua/sbx-go-sdk/client"
	"github.com/squall-chua/sbx-go-sdk/settings"
)

const (
	featureKey = "feature.ssh"
	portKey    = "ssh.port"
)

// Port returns the configured sandboxd SSH loopback port (ssh.port, default 2222).
func Port(ctx context.Context, c *client.Client) (int, error) {
	s, err := settings.Get(ctx, c, portKey)
	if err != nil {
		return 0, err
	}
	var p int
	if err := json.Unmarshal(s.Value, &p); err != nil {
		return 0, fmt.Errorf("ssh: parse %s value %s: %w: %w", portKey, s.Value, client.ErrUnexpectedFormat, err)
	}
	return p, nil
}

// Target is the SSH connection info for a sandbox. The sandbox name is the SSH
// username; the host is always loopback.
type Target struct {
	User string // sandbox name
	Host string // "127.0.0.1"
	Port int    // ssh.port
}

// Args returns the ssh client arguments, e.g. ["mybox@127.0.0.1", "-p", "2222"],
// suitable for exec.Command("ssh", t.Args()...).
func (t Target) Args() []string {
	return []string{t.User + "@" + t.Host, "-p", strconv.Itoa(t.Port)}
}

// Command returns the display form, e.g. "ssh mybox@127.0.0.1 -p 2222".
func (t Target) Command() string {
	return "ssh " + t.User + "@" + t.Host + " -p " + strconv.Itoa(t.Port)
}

// TargetFor builds connection info for a sandbox. It is a pure builder — no
// existence check. With ssh.autoCreate (default true) the sandbox is created on
// connect, so a target for any name is valid.
func TargetFor(ctx context.Context, c *client.Client, sandboxName string) (Target, error) {
	port, err := Port(ctx, c)
	if err != nil {
		return Target{}, err
	}
	return Target{User: sandboxName, Host: "127.0.0.1", Port: port}, nil
}
```

- [ ] **Step 4: Run the test to verify it passes**

Run: `gofmt -w ssh/ && go test ./ssh/ -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add ssh/ssh.go ssh/ssh_test.go
git commit -m "feat(ssh): Target/Port connection helpers over settings"
```

---

### Task 5: `ssh` — `Enable` / `Disable` / `Enabled`

**Files:**
- Modify: `ssh/ssh.go`
- Test: `ssh/ssh_test.go`

**Interfaces:**
- Consumes: `settings.Set`/`settings.Get` (Tasks 2-3), `newFakeSbx` (Task 4).
- Produces:
  - `func Enable(ctx context.Context, c *client.Client) error`
  - `func Disable(ctx context.Context, c *client.Client) error`
  - `func Enabled(ctx context.Context, c *client.Client) (bool, error)`

- [ ] **Step 1: Write the failing test**

Add to `ssh/ssh_test.go`:

```go
func TestEnableDisableArgs(t *testing.T) {
	c, argFile := newFakeSbx(t, 0, "", "")
	ctx := context.Background()
	require.NoError(t, Enable(ctx, c))
	require.NoError(t, Disable(ctx, c))
	args, _ := os.ReadFile(argFile)
	lines := string(args)
	require.Contains(t, lines, "settings set feature.ssh true")
	require.Contains(t, lines, "settings set feature.ssh false")
}

func TestEnabledParsesFeatureFlag(t *testing.T) {
	c, _ := newFakeSbx(t, 0, `{"key":"feature.ssh","value":{"enabled":true,"variant":""},"type":"json","source":"override","description":"flag"}`, "")
	on, err := Enabled(context.Background(), c)
	require.NoError(t, err)
	require.True(t, on)
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./ssh/ -run 'TestEnable' -v`
Expected: FAIL — `undefined: Enable` / `undefined: Disable` / `undefined: Enabled`.

- [ ] **Step 3: Write the minimal implementation**

Add to `ssh/ssh.go`:

```go
// Enable turns on the SSH endpoint (settings set feature.ssh true). Fire-and-forget:
// the daemon hot-reloads within ~5s. Requires platform.allowExperimentalFeatures
// (default true), which Enable does not modify.
func Enable(ctx context.Context, c *client.Client) error {
	return settings.Set(ctx, c, featureKey, "true")
}

// Disable turns off the SSH endpoint (settings set feature.ssh false — explicit, so
// the result is deterministic regardless of the default).
func Disable(ctx context.Context, c *client.Client) error {
	return settings.Set(ctx, c, featureKey, "false")
}

// Enabled reports whether the SSH endpoint feature flag is on. feature.ssh is a
// structured flag; its value is {"enabled":bool,…}.
func Enabled(ctx context.Context, c *client.Client) (bool, error) {
	s, err := settings.Get(ctx, c, featureKey)
	if err != nil {
		return false, err
	}
	var f struct {
		Enabled bool `json:"enabled"`
	}
	if err := json.Unmarshal(s.Value, &f); err != nil {
		return false, fmt.Errorf("ssh: parse %s value %s: %w: %w", featureKey, s.Value, client.ErrUnexpectedFormat, err)
	}
	return f.Enabled, nil
}
```

- [ ] **Step 4: Run the test to verify it passes**

Run: `gofmt -w ssh/ && go test ./ssh/ -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add ssh/ssh.go ssh/ssh_test.go
git commit -m "feat(ssh): Enable/Disable/Enabled feature-flag helpers"
```

---

### Task 6: `ssh` — `Setup` + `WithAlias`/`WithRegenerate` options

**Files:**
- Create: `ssh/setup.go`
- Test: `ssh/setup_test.go`

**Interfaces:**
- Consumes: `client.Client.Runner()`, `newFakeSbx` (Task 4).
- Produces:
  - `type Option func(*setupConfig)`
  - `func WithAlias(alias string) Option`
  - `func WithRegenerate() Option`
  - `func Setup(ctx context.Context, c *client.Client, opts ...Option) error`

- [ ] **Step 1: Write the failing test**

Create `ssh/setup_test.go`:

```go
package ssh

import (
	"context"
	"os"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSetupArgs(t *testing.T) {
	c, argFile := newFakeSbx(t, 0, "", "")
	ctx := context.Background()

	require.NoError(t, Setup(ctx, c))
	require.NoError(t, Setup(ctx, c, WithAlias("work"), WithRegenerate()))

	args, _ := os.ReadFile(argFile)
	lines := string(args)
	require.Contains(t, lines, "ssh setup\n")               // no options -> bare
	require.Contains(t, lines, "ssh setup --alias work --regenerate")
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./ssh/ -run 'TestSetupArgs' -v`
Expected: FAIL — `undefined: Setup` / `undefined: WithAlias` / `undefined: WithRegenerate`.

- [ ] **Step 3: Write the minimal implementation**

Create `ssh/setup.go`:

```go
package ssh

import (
	"context"

	"github.com/squall-chua/sbx-go-sdk/client"
)

type setupConfig struct {
	alias      string
	regenerate bool
}

// Option configures Setup.
type Option func(*setupConfig)

// WithAlias sets the ssh_config Host alias to write (default "sbx" upstream).
func WithAlias(alias string) Option { return func(c *setupConfig) { c.alias = alias } }

// WithRegenerate rotates the managed SSH key.
func WithRegenerate() Option { return func(c *setupConfig) { c.regenerate = true } }

// Setup provisions the local SSH client for the sandbox endpoint (`sbx ssh setup`):
// it writes an ~/.ssh/config alias + a dedicated managed key. Idempotent and
// non-interactive; safe to re-run.
func Setup(ctx context.Context, c *client.Client, opts ...Option) error {
	var cfg setupConfig
	for _, o := range opts {
		o(&cfg)
	}
	args := []string{"ssh", "setup"}
	if cfg.alias != "" {
		args = append(args, "--alias", cfg.alias)
	}
	if cfg.regenerate {
		args = append(args, "--regenerate")
	}
	r, err := c.Runner()
	if err != nil {
		return err
	}
	_, err = r.Capture(ctx, nil, args...)
	return err
}
```

- [ ] **Step 4: Run the test to verify it passes**

Run: `gofmt -w ssh/ && go test ./ssh/ -v`
Expected: PASS (all ssh tests).

- [ ] **Step 5: Commit**

```bash
git add ssh/setup.go ssh/setup_test.go
git commit -m "feat(ssh): Setup wrapper for sbx ssh setup"
```

---

### Task 7: Integration contract test (read-only `settings`)

**Files:**
- Create: `internal/integration/settings_test.go`

**Interfaces:**
- Consumes: `settings.List`/`settings.Get` (Tasks 2), `client.New`.
- Produces: `TestContract_SettingsListGet` (build tag `integration`).

- [ ] **Step 1: Write the failing test**

Create `internal/integration/settings_test.go`:

```go
//go:build integration

package integration

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/squall-chua/sbx-go-sdk/client"
	"github.com/squall-chua/sbx-go-sdk/settings"
	"github.com/stretchr/testify/require"
)

// TestContract_SettingsListGet is a read-only drift detector for the `sbx settings
// … --json` shape. It asserts structure, not exact default values (which drift
// across sbx versions). It never mutates settings.
func TestContract_SettingsListGet(t *testing.T) {
	ctx := context.Background()
	c, err := client.New(ctx)
	require.NoError(t, err)

	all, err := settings.List(ctx, c)
	require.NoError(t, err)
	require.NotEmpty(t, all)
	for _, s := range all {
		require.NotEmpty(t, s.Key)
		require.NotEmpty(t, s.Type)
		require.NotEmpty(t, s.Source)
	}

	// ssh.port is a known int-typed setting; assert structure, not the value.
	p, err := settings.Get(ctx, c, "ssh.port")
	require.NoError(t, err)
	require.Equal(t, "ssh.port", p.Key)
	var port int
	require.NoError(t, json.Unmarshal(p.Value, &port))
	require.Positive(t, port)
}
```

- [ ] **Step 2: Run the test to verify it passes against the live daemon**

Run: `go test -tags integration -run TestContract_SettingsListGet ./internal/integration -v`
Expected: PASS (requires a running v0.34.0 `sandboxd`; the SDK's `client.New` needs no daemon, but `settings list` reads live settings). If `sbx` is not installed/daemon absent, this is the only task that needs the live environment.

- [ ] **Step 3: Commit**

```bash
git add internal/integration/settings_test.go
git commit -m "test(integration): read-only settings --json contract"
```

---

### Task 8: Documentation (README + skill doc)

**Files:**
- Modify: `README.md` (feature list + a usage subsection; and the Known-deviations note about fire-and-forget)
- Modify: `skills/sbx-go-sdk/SKILL.md` (API surface + gotcha)

**Interfaces:**
- Consumes: the public APIs from Tasks 1-6.
- Produces: docs only (no code).

- [ ] **Step 1: Add a `settings` + `ssh` usage subsection to `README.md`**

Insert a new subsection in the numbered "how-to" area (after the last numbered example, before "## Version alignment"). Use this content verbatim:

````markdown
### 8. Settings & SSH

`settings` wraps `sbx settings … --json`; `ssh` manages the experimental SSH endpoint
(composed over `settings`). Both shell out to the `sbx` binary.

```go
// Read a setting (typed JSON value).
s, _ := settings.Get(ctx, c, "ssh.port")
port, _ := strconv.Atoi(s.Text())          // or s.Bool() for bool settings

// Write / clear an override (fire-and-forget; daemon hot-reloads within ~5s).
_ = settings.Set(ctx, c, "kit.allowedSources", `["docker.io/","ghcr.io/"]`)
_ = settings.Unset(ctx, c, "kit.allowedSources")

// Enable and connect to the SSH endpoint.
_ = ssh.Enable(ctx, c)                       // settings set feature.ssh true
_ = ssh.Setup(ctx, c)                        // provisions ~/.ssh/config alias + key
tgt, _ := ssh.TargetFor(ctx, c, "mybox")     // {User:"mybox", Host:"127.0.0.1", Port:2222}
fmt.Println(tgt.Command())                   // ssh mybox@127.0.0.1 -p 2222
```

> `Set`/`Enable`/etc. are fire-and-forget — they write `settings.json` and return; the
> daemon reloads within ~5s. `ssh.Enable` toggles only `feature.ssh`; SSH also requires
> `platform.allowExperimentalFeatures` (default `true`).
````

- [ ] **Step 2: Add both packages to the `skills/sbx-go-sdk/SKILL.md` surface + a gotcha**

Under the package/API list section, add a line describing the two packages, and add this bullet to the "Gotchas" list:

```markdown
- **`settings`/`ssh` mutations are fire-and-forget shell-outs** — `settings.Set`,
  `ssh.Enable/Disable/Setup` write host state (`settings.json`, `~/.ssh/config`) and
  return before the daemon's ~5s hot-reload. `settings.Get/List` and `ssh.Port/Enabled`
  read via `--json`. `ssh.Enable` sets only `feature.ssh` (also needs
  `platform.allowExperimentalFeatures`, default true).
```

- [ ] **Step 3: Verify docs build/format and the whole module still passes**

Run: `gofmt -l . && go build ./... && go test ./...`
Expected: no `gofmt` output; build OK; all unit tests PASS.

- [ ] **Step 4: Commit**

```bash
git add README.md skills/sbx-go-sdk/SKILL.md
git commit -m "docs: document settings and ssh packages"
```

---

## Self-Review

**1. Spec coverage:**
- `settings` List/Get/Set/Unset → Tasks 2-3. Setting + Bool/Text → Task 1. ✅
- `ssh` Enable/Disable/Enabled → Task 5; Setup + options → Task 6; Port/Target/TargetFor/Args/Command → Task 4. ✅
- Errors = raw `*client.CLIError`, `--json` parse wrapped with `ErrUnexpectedFormat`, no sentinels → Tasks 2-5 (`TestGetErrorPropagatesCLIError`). ✅
- Fire-and-forget + ~5s lag documented → doc comments (Tasks 3, 5) + README/SKILL (Task 8). ✅
- `Enable` single-purpose; documents `platform.allowExperimentalFeatures` dependency → Task 5 comment + Task 8. ✅
- `Target` pure builder, `Host="127.0.0.1"`, `Args()`/`Command()` (no `String()`) → Task 4. ✅
- Integration read-only + structural assertions → Task 7. ✅
- `ssh` composes over `settings` (imports it, no duplicated shell-out) → Task 4-5 imports. ✅

**2. Placeholder scan:** No TBD/TODO; every code step shows complete code and exact run/expected lines. ✅

**3. Type consistency:** `Setting{Key,Value,Type,Source,Description}` used identically in Tasks 1-2 and 7. `settings.Get`/`settings.Set` signatures consumed unchanged by `ssh` in Tasks 4-5. `Target{User,Host,Port}` consistent across Task 4 methods and tests. `Option`/`setupConfig` defined and used only in Task 6. `newFakeSbx(t, exit, stdout, stderr)` identical in `settings_test.go` (Task 2) and `ssh_test.go` (Task 4). ✅

Note: `newFakeSbx` is intentionally re-declared per test package (matching the repo's existing per-package `recordingClient` convention in `policy_test.go`); it is not shared infrastructure.
