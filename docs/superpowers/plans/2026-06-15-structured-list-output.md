# Structured output for `policy ls` and `secret ls` — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace the raw-text returns of `policy.List` and `secret.List` with typed values (`[]PolicyRule`, `*Secrets`), keeping a `ListRaw` escape hatch, and isolate the table parsing so a future upstream `--json` is a parser-internal swap.

**Architecture:** A generic, independently-tested `internal/coltable` helper finds + validates the column header of an `sbx` fixed-width table and slices each row into a `map[string]string`. The `policy` and `secret` packages map those rows to typed structs and handle their own quirks (policy's multi-resource continuation rows; secret's two sections). Header drift fails loud via `client.ErrUnexpectedFormat`; a missing header means an empty listing. A live `//go:build integration` contract test guards the real format.

**Tech Stack:** Go 1.25, stdlib (`strings`, `regexp`, `slices`, `errors`, `fmt`), `stretchr/testify`. Reference spec: [docs/superpowers/specs/2026-06-15-structured-list-output-design.md](../specs/2026-06-15-structured-list-output-design.md). ADR: [docs/adr/0002-parse-sbx-table-output.md](../../adr/0002-parse-sbx-table-output.md).

---

## File structure

- **Create** `internal/coltable/coltable.go` — generic fixed-width table parser (find/validate header, slice rows).
- **Create** `internal/coltable/coltable_test.go` — isolated unit tests.
- **Modify** `client/errors.go` — add `ErrUnexpectedFormat` sentinel.
- **Modify** `policy/policy.go` — `PolicyRule` type; `List` now returns `[]PolicyRule`; add `ListRaw`; `parsePolicyList`.
- **Modify** `policy/policy_test.go` — update the existing list test; add `parsePolicyList` tests.
- **Modify** `secret/secret.go` — `Stored`/`Custom`/`Secrets` types; `List` now returns `*Secrets`; add `ListRaw`; `parseSecretList`.
- **Modify** `secret/secret_test.go` — update the existing list test; add `parseSecretList` tests.
- **Create** `internal/integration/list_format_contract_test.go` — live drift alarm (`//go:build integration`).
- **Modify** `README.md` and `skills/sbx-go-sdk/SKILL.md` — update "returns raw text" notes.

---

## Task 1: Add the `ErrUnexpectedFormat` sentinel

**Files:**
- Modify: `client/errors.go:13-21`

- [ ] **Step 1: Add the sentinel**

In `client/errors.go`, add `ErrUnexpectedFormat` to the existing `var (...)` sentinel block (currently lines 13-21):

```go
// Sentinels callers branch on.
var (
	ErrSandboxNotFound     = errors.New("sandbox not found")
	ErrSandboxExists       = errors.New("sandbox already exists")
	ErrSandboxNotRunning   = errors.New("sandbox not running")
	ErrExecNotFound        = errors.New("exec not found")
	ErrIncompatibleVersion = errors.New("incompatible sbx/daemon version")
	ErrDaemonNotRunning    = errors.New("sandboxd not running")
	ErrBinaryNotFound      = cli.ErrBinaryNotFound
	ErrUnexpectedFormat    = errors.New("unexpected sbx output format")
)
```

- [ ] **Step 2: Verify it builds**

Run: `go build ./...`
Expected: builds with no errors.

- [ ] **Step 3: Commit**

```bash
git add client/errors.go
git commit -m "feat(client): add ErrUnexpectedFormat sentinel for table-format drift"
```

---

## Task 2: `internal/coltable` generic table parser

**Files:**
- Create: `internal/coltable/coltable.go`
- Test: `internal/coltable/coltable_test.go`

- [ ] **Step 1: Write the failing tests**

Create `internal/coltable/coltable_test.go`. The fixtures are hand-aligned: columns are separated by 2+ spaces and each value fits inside its column. (`NAME` at col 0, `VALUE` at col 8, `NOTE` at col 16.)

```go
package coltable

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestParse_BasicWithPreamble(t *testing.T) {
	raw := "starting some banner\n" +
		"NAME    VALUE   NOTE\n" +
		"alpha   1       first\n" +
		"beta    2       second\n"

	rows, err := Parse(raw, []string{"NAME", "VALUE", "NOTE"})
	require.NoError(t, err)
	require.Len(t, rows, 2)
	require.Equal(t, map[string]string{"NAME": "alpha", "VALUE": "1", "NOTE": "first"}, rows[0])
	require.Equal(t, map[string]string{"NAME": "beta", "VALUE": "2", "NOTE": "second"}, rows[1])
}

func TestParse_ShortLineLeavesTrailingColumnsEmpty(t *testing.T) {
	// A continuation-style line that only reaches the last column: leading columns
	// are blank, the final column carries the value.
	raw := "NAME    VALUE   NOTE\n" +
		"alpha   1       first\n" +
		"                extra\n"

	rows, err := Parse(raw, []string{"NAME", "VALUE", "NOTE"})
	require.NoError(t, err)
	require.Len(t, rows, 2)
	require.Equal(t, "", rows[1]["NAME"])
	require.Equal(t, "", rows[1]["VALUE"])
	require.Equal(t, "extra", rows[1]["NOTE"])
}

func TestParse_BlankLinesSkipped(t *testing.T) {
	raw := "NAME    VALUE   NOTE\n" +
		"alpha   1       first\n" +
		"\n" +
		"beta    2       second\n"

	rows, err := Parse(raw, []string{"NAME", "VALUE", "NOTE"})
	require.NoError(t, err)
	require.Len(t, rows, 2)
}

func TestParse_NoHeader(t *testing.T) {
	rows, err := Parse(`No secrets found for scope "x".`, []string{"NAME", "VALUE", "NOTE"})
	require.ErrorIs(t, err, ErrNoHeader)
	require.Nil(t, rows)
}

func TestParse_EmptyInput(t *testing.T) {
	rows, err := Parse("", []string{"NAME", "VALUE"})
	require.ErrorIs(t, err, ErrNoHeader)
	require.Nil(t, rows)
}

func TestParse_HeaderMismatchRenamed(t *testing.T) {
	// Header-like row (uppercase first token, >=2 columns) but a renamed column.
	raw := "NAME    AMOUNT  NOTE\n" +
		"alpha   1       first\n"

	rows, err := Parse(raw, []string{"NAME", "VALUE", "NOTE"})
	require.ErrorIs(t, err, ErrHeaderMismatch)
	require.Nil(t, rows)
}

func TestParse_HeaderMismatchExtraColumn(t *testing.T) {
	raw := "NAME    VALUE   NOTE    EXTRA\n" +
		"alpha   1       first   x\n"

	_, err := Parse(raw, []string{"NAME", "VALUE", "NOTE"})
	require.ErrorIs(t, err, ErrHeaderMismatch)
}

func TestParse_SingleColumnLabelIsNotHeader(t *testing.T) {
	// A lone uppercase label like a section title must not be taken as a header.
	rows, err := Parse("CUSTOM SECRETS\n", []string{"NAME", "VALUE"})
	require.ErrorIs(t, err, ErrNoHeader)
	require.Nil(t, rows)
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/coltable/ -v`
Expected: FAIL — `undefined: Parse`, `undefined: ErrNoHeader`, `undefined: ErrHeaderMismatch`.

- [ ] **Step 3: Implement `coltable.go`**

Create `internal/coltable/coltable.go`:

```go
// Package coltable parses the fixed-width, whitespace-aligned tables the sbx CLI
// prints (e.g. `sbx policy ls`, `sbx secret ls`). It is generic: it finds and
// validates a header row, then slices each data line at the header's column
// offsets. It does not know about table-specific quirks (continuation rows,
// multiple sections) — callers handle those over the returned rows.
package coltable

import (
	"errors"
	"regexp"
	"slices"
	"strings"
)

var (
	// ErrNoHeader means no header-like row was found — treat as an empty listing.
	ErrNoHeader = errors.New("coltable: no header row found")
	// ErrHeaderMismatch means a header-like row was found but its columns do not
	// match the expected set — the CLI's table format has drifted.
	ErrHeaderMismatch = errors.New("coltable: header does not match expected columns")
)

// gutter matches the run of two or more spaces that separates table columns.
var gutter = regexp.MustCompile(`\s{2,}`)

// Parse locates the first header-like row, validates it equals want (same columns,
// same order), and returns each following non-blank line sliced into a map keyed by
// the want column names. Every field is trimmed of surrounding whitespace.
func Parse(raw string, want []string) ([]map[string]string, error) {
	lines := strings.Split(strings.ReplaceAll(raw, "\r\n", "\n"), "\n")
	hi := -1
	for i, ln := range lines {
		if isHeaderLike(ln) {
			hi = i
			break
		}
	}
	if hi == -1 {
		return nil, ErrNoHeader
	}
	offsets, err := headerOffsets(lines[hi], want)
	if err != nil {
		return nil, err
	}
	var rows []map[string]string
	for _, ln := range lines[hi+1:] {
		if strings.TrimSpace(ln) == "" {
			continue
		}
		rows = append(rows, sliceRow(ln, want, offsets))
	}
	return rows, nil
}

// isHeaderLike reports whether ln looks like a column-header row: at least two
// columns, with a first column that is an all-uppercase token (the convention the
// sbx tables use: PROVENANCE, SCOPE, ...). This distinguishes the header from
// banner lines, prose ("No secrets found ..."), single-column section labels
// ("CUSTOM SECRETS"), and lower-cased data rows.
func isHeaderLike(ln string) bool {
	cols := splitCols(ln)
	if len(cols) < 2 {
		return false
	}
	first := cols[0]
	if len(first) < 2 {
		return false
	}
	for _, r := range first {
		if !((r >= 'A' && r <= 'Z') || r == '_' || r == '/') {
			return false
		}
	}
	return true
}

// splitCols splits a trimmed line on runs of two or more spaces, dropping empties.
// Single spaces inside a column are preserved.
func splitCols(ln string) []string {
	ln = strings.TrimSpace(ln)
	if ln == "" {
		return nil
	}
	return gutter.Split(ln, -1)
}

// headerOffsets validates that line's columns equal want exactly, then returns the
// start byte-offset of each column (the index of each want token, in order).
func headerOffsets(line string, want []string) ([]int, error) {
	if !slices.Equal(splitCols(line), want) {
		return nil, ErrHeaderMismatch
	}
	offsets := make([]int, len(want))
	cur := 0
	for i, col := range want {
		idx := strings.Index(line[cur:], col)
		if idx < 0 {
			return nil, ErrHeaderMismatch
		}
		offsets[i] = cur + idx
		cur = offsets[i] + len(col)
	}
	return offsets, nil
}

// sliceRow cuts ln at the column offsets, trimming each field. The final column
// extends to end-of-line. Columns a short line doesn't reach yield "".
func sliceRow(ln string, want []string, offsets []int) map[string]string {
	m := make(map[string]string, len(want))
	for i, col := range want {
		start := offsets[i]
		if start > len(ln) {
			m[col] = ""
			continue
		}
		end := len(ln)
		if i+1 < len(want) && offsets[i+1] < end {
			end = offsets[i+1]
		}
		m[col] = strings.TrimSpace(ln[start:end])
	}
	return m
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/coltable/ -v`
Expected: PASS (all 8 tests).

- [ ] **Step 5: Commit**

```bash
gofmt -w internal/coltable/
git add internal/coltable/
git commit -m "feat(coltable): generic fixed-width sbx table parser"
```

---

## Task 3: `policy.List` returns `[]PolicyRule`

**Files:**
- Modify: `policy/policy.go` (imports; replace `List` at lines 69-77; add types + parser)
- Test: `policy/policy_test.go` (update `TestPolicyListProfilesAndLog`; add parse tests)

- [ ] **Step 1: Write the failing tests**

In `policy/policy_test.go`, **replace** the body of `TestPolicyListProfilesAndLog` (lines 48-81) so the raw-text assertion uses the new `ListRaw`, and `List` now returns parsed rules (the fake `sbx` prints `POLICY-TEXT`, which has no header → empty):

```go
func TestPolicyListProfilesAndLog(t *testing.T) {
	// List/Profiles: capturing runner returns the fake sbx stdout.
	argFile := filepath.Join(t.TempDir(), "args.txt")
	// fake sbx prints a banner to stdout so ListRaw returns non-empty text.
	sock := filepath.Join(t.TempDir(), "d.sock")
	l, err := net.Listen("unix", sock)
	require.NoError(t, err)
	srv := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/network/log", r.URL.Path)
		w.Write([]byte(`{"blocked_hosts":[],"allowed_hosts":[{"host":"api.github.com:443","vm_name":"s1","proxy_type":"forward","rule":"domain-allowed","last_seen":"2026-06-10T11:29:10Z","since":"2026-06-10T11:29:10Z","count_since":2}]}`))
	})}
	go srv.Serve(l)
	t.Cleanup(func() { srv.Close() })
	bin := filepath.Join(t.TempDir(), "sbx")
	require.NoError(t, os.WriteFile(bin, []byte("#!/bin/sh\nprintf '%s\\n' \"$*\" >> "+argFile+"\necho POLICY-TEXT\nexit 0\n"), 0o755))
	c, err := client.New(context.Background(), client.WithSocketPath(sock), client.WithBinaryPath(bin))
	require.NoError(t, err)
	ctx := context.Background()

	raw, err := ListRaw(ctx, c, "s1")
	require.NoError(t, err)
	require.Contains(t, raw, "POLICY-TEXT")
	data, _ := os.ReadFile(argFile)
	require.Contains(t, string(data), "policy ls s1")

	// "POLICY-TEXT" has no recognizable header → empty rule list, no error.
	rules, err := List(ctx, c, "s1")
	require.NoError(t, err)
	require.Empty(t, rules)

	prof, err := Profiles(ctx, c)
	require.NoError(t, err)
	require.Contains(t, prof, "POLICY-TEXT")

	logs, err := Log(ctx, c)
	require.NoError(t, err)
	require.Len(t, logs.AllowedHosts, 1)
	require.Equal(t, "api.github.com:443", logs.AllowedHosts[0].Host)
}
```

Then **add** a new white-box parse test at the end of `policy/policy_test.go`. The data rows are hand-aligned to the header (column starts `PROVENANCE`@0, `APPLIES_TO`@12, `POLICY/RULE`@24, `TYPE`@37, `DECISION`@46, `RESOURCES`@56); the **continuation row's indent is computed from the header** with `strings.Repeat` so it can't drift from the `RESOURCES` offset. Add `"strings"` to the test file's imports.

```go
func TestParsePolicyList(t *testing.T) {
	hdr := "PROVENANCE  APPLIES_TO  POLICY/RULE  TYPE     DECISION  RESOURCES"
	ri := strings.Index(hdr, "RESOURCES") // RESOURCES column offset (56)
	raw := "Starting sandboxd daemon...\n" +
		"Daemon started (PID: 17849, socket: /x/sandboxd.sock)\n" +
		hdr + "\n" +
		"local       all         default-ai   network  allow     a.example.com:443\n" +
		strings.Repeat(" ", ri) + "b.example.com:443\n" +
		"\n" +
		"local       web         block-bad    network  deny      evil.example.com:443\n"

	rules, err := parsePolicyList(raw)
	require.NoError(t, err)
	require.Len(t, rules, 2)

	require.Equal(t, PolicyRule{
		Provenance: "local",
		AppliesTo:  "all",
		Rule:       "default-ai",
		Type:       "network",
		Decision:   "allow",
		Resources:  []string{"a.example.com:443", "b.example.com:443"},
	}, rules[0])

	require.Equal(t, "block-bad", rules[1].Rule)
	require.Equal(t, "deny", rules[1].Decision)
	require.Equal(t, []string{"evil.example.com:443"}, rules[1].Resources)
}

func TestParsePolicyList_Empty(t *testing.T) {
	rules, err := parsePolicyList("No policies found.\n")
	require.NoError(t, err)
	require.Empty(t, rules)
}

func TestParsePolicyList_Drift(t *testing.T) {
	raw := "PROVENANCE  APPLIES_TO  RULE  TYPE  DECISION  RESOURCES\n" +
		"local       all         x     net   allow     a:443\n"
	_, err := parsePolicyList(raw)
	require.ErrorIs(t, err, client.ErrUnexpectedFormat)
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./policy/ -v`
Expected: FAIL — `undefined: ListRaw`, `undefined: parsePolicyList`, `undefined: PolicyRule`, and `List` signature mismatch (`require.Empty` on a string / `rules.Rule`).

- [ ] **Step 3: Implement the changes in `policy/policy.go`**

First update the imports block (currently `context`, `net/http`, `client`) to add `errors`, `fmt`, and `coltable`:

```go
import (
	"context"
	"errors"
	"fmt"
	"net/http"

	"github.com/squall-chua/sbx-go-sdk/client"
	"github.com/squall-chua/sbx-go-sdk/internal/coltable"
)
```

Then **replace** the existing `List` function (lines 69-77) with the typed `List`, a new `ListRaw`, the `PolicyRule` type, the expected header, and `parsePolicyList`:

```go
// policyHeader is the column header of `sbx policy ls`, in order. Drift from this
// set is reported as client.ErrUnexpectedFormat.
var policyHeader = []string{"PROVENANCE", "APPLIES_TO", "POLICY/RULE", "TYPE", "DECISION", "RESOURCES"}

// PolicyRule is one rule from `sbx policy ls`, modelling exactly its columns.
type PolicyRule struct {
	Provenance string   // "local", or a remote-governance source
	AppliesTo  string   // "all" or a sandbox name
	Rule       string   // POLICY/RULE — rule name, or ID when unnamed
	Type       string   // "network"
	Decision   string   // "allow" | "deny"
	Resources  []string // hosts, gathered across continuation rows
}

// List returns the parsed `sbx policy ls [SCOPE]` rules. scope "" lists global+all.
// A format change in the CLI's table yields client.ErrUnexpectedFormat — use
// ListRaw to fall back to the unparsed text.
func List(ctx context.Context, c *client.Client, scope string) ([]PolicyRule, error) {
	raw, err := ListRaw(ctx, c, scope)
	if err != nil {
		return nil, err
	}
	return parsePolicyList(raw)
}

// ListRaw returns the raw `sbx policy ls [SCOPE]` text.
func ListRaw(ctx context.Context, c *client.Client, scope string) (string, error) {
	args := []string{"policy", "ls"}
	if scope != "" {
		args = append(args, scope)
	}
	return capture(ctx, c, args...)
}

// parsePolicyList maps the policy table to rules. A row with a non-blank PROVENANCE
// starts a new rule; a continuation row (blank before RESOURCES) appends its host
// to the current rule. A missing header means an empty listing (not an error).
func parsePolicyList(raw string) ([]PolicyRule, error) {
	rows, err := coltable.Parse(raw, policyHeader)
	if errors.Is(err, coltable.ErrNoHeader) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("policy list: %w: %w", client.ErrUnexpectedFormat, err)
	}
	var out []PolicyRule
	for _, r := range rows {
		if r["PROVENANCE"] != "" {
			out = append(out, PolicyRule{
				Provenance: r["PROVENANCE"],
				AppliesTo:  r["APPLIES_TO"],
				Rule:       r["POLICY/RULE"],
				Type:       r["TYPE"],
				Decision:   r["DECISION"],
			})
		}
		if res := r["RESOURCES"]; res != "" && len(out) > 0 {
			out[len(out)-1].Resources = append(out[len(out)-1].Resources, res)
		}
	}
	return out, nil
}
```

Leave `capture`, `Profiles`, and everything else unchanged.

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./policy/ -v`
Expected: PASS (mutations, list/profiles/log, and the three parse tests).

- [ ] **Step 5: Commit**

```bash
gofmt -w policy/
git add policy/policy.go policy/policy_test.go
git commit -m "feat(policy): List returns []PolicyRule; add ListRaw"
```

---

## Task 4: `secret.List` returns `*Secrets`

**Files:**
- Modify: `secret/secret.go` (imports; replace `List` at lines 46-57; add types + parser)
- Test: `secret/secret_test.go` (update `TestSecretOps`; add parse tests)

- [ ] **Step 1: Write the failing tests**

In `secret/secret_test.go`, in `TestSecretOps`, **replace** the `List` call (lines 36-38) with `ListRaw`:

```go
	txt, err := ListRaw(ctx, c, "")
	require.NoError(t, err)
	require.Contains(t, txt, "SECRET-TEXT")
```

Then **add** white-box parse tests at the end of `secret/secret_test.go`. The fixture mirrors real `sbx secret ls`: a standard table (`SCOPE`@0 `TYPE`@12 `NAME`@21 `SECRET`@29), a blank line, the `CUSTOM SECRETS` section label, then the custom table (`SCOPE`@0 `TARGET`@10 `ENV`@20 `PLACEHOLDER`@29 `SECRET`@42).

```go
func TestParseSecretList(t *testing.T) {
	raw := "SCOPE       TYPE     NAME    SECRET\n" +
		"my-sandbox  service  openai  testte**\n" +
		"\n" +
		"CUSTOM SECRETS\n" +
		"SCOPE     TARGET    ENV      PLACEHOLDER  SECRET\n" +
		"(global)  api.x.io  API_KEY  ph-123       sk-***\n"

	got, err := parseSecretList(raw)
	require.NoError(t, err)

	require.Equal(t, []Stored{{
		Scope: "my-sandbox", Type: "service", Name: "openai", ValueMasked: "testte**",
	}}, got.Stored)

	require.Equal(t, []Custom{{
		Scope: "", Target: "api.x.io", Env: "API_KEY", Placeholder: "ph-123", ValueMasked: "sk-***",
	}}, got.Custom)
}

func TestParseSecretList_Empty(t *testing.T) {
	got, err := parseSecretList(`No secrets found for scope "zzz".` + "\n")
	require.NoError(t, err)
	require.Empty(t, got.Stored)
	require.Empty(t, got.Custom)
}

func TestParseSecretList_CustomOnly(t *testing.T) {
	raw := "CUSTOM SECRETS\n" +
		"SCOPE     TARGET    ENV      PLACEHOLDER  SECRET\n" +
		"(global)  api.x.io  API_KEY  ph-123       sk-***\n"

	got, err := parseSecretList(raw)
	require.NoError(t, err)
	require.Empty(t, got.Stored)
	require.Len(t, got.Custom, 1)
	require.Equal(t, "api.x.io", got.Custom[0].Target)
}

func TestParseSecretList_Drift(t *testing.T) {
	raw := "SCOPE       KIND     NAME    SECRET\n" +
		"my-sandbox  service  openai  testte**\n"
	_, err := parseSecretList(raw)
	require.ErrorIs(t, err, client.ErrUnexpectedFormat)
}
```

`secret_test.go` must import `client` — add `"github.com/squall-chua/sbx-go-sdk/client"` to its imports (it currently imports only `context`, `os`, `path/filepath`, `testing`, the testify require, and `client` is already imported via `recordingClient`; confirm the import line is present, add if missing).

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./secret/ -v`
Expected: FAIL — `undefined: ListRaw`, `undefined: parseSecretList`, `undefined: Stored`, `undefined: Custom`.

- [ ] **Step 3: Implement the changes in `secret/secret.go`**

Update the imports (currently `context`, `client`) to add `errors`, `fmt`, `strings`, `coltable`:

```go
import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/squall-chua/sbx-go-sdk/client"
	"github.com/squall-chua/sbx-go-sdk/internal/coltable"
)
```

Then **replace** the existing `List` function (lines 46-57) with the typed `List`, a new `ListRaw`, the types, expected headers, and `parseSecretList`:

```go
// Header columns of the two `sbx secret ls` tables, in order. Drift yields
// client.ErrUnexpectedFormat.
var (
	secretStdHeader    = []string{"SCOPE", "TYPE", "NAME", "SECRET"}
	secretCustomHeader = []string{"SCOPE", "TARGET", "ENV", "PLACEHOLDER", "SECRET"}
)

// Stored is a service or registry secret row (`sbx secret set`). Type is
// "service" or "registry".
type Stored struct {
	Scope       string // "" = global, else sandbox name
	Type        string // "service" | "registry"
	Name        string // service name or registry host
	ValueMasked string // masked display value — never the real secret
}

// Custom is a custom secret row (`sbx secret set-custom`).
type Custom struct {
	Scope       string // "" = global, else sandbox name
	Target      string // target host
	Env         string // env var injected into the sandbox
	Placeholder string
	ValueMasked string // masked display value
}

// Secrets is the parsed `sbx secret ls` output: the standard table (service +
// registry) and the custom-secrets table.
type Secrets struct {
	Stored []Stored
	Custom []Custom
}

// List returns the parsed `sbx secret ls [SCOPE]` output. A format change in the
// CLI's tables yields client.ErrUnexpectedFormat — use ListRaw to fall back.
func List(ctx context.Context, c *client.Client, scope string) (*Secrets, error) {
	raw, err := ListRaw(ctx, c, scope)
	if err != nil {
		return nil, err
	}
	return parseSecretList(raw)
}

// ListRaw returns the raw `sbx secret ls [SCOPE]` text.
func ListRaw(ctx context.Context, c *client.Client, scope string) (string, error) {
	args := []string{"secret", "ls"}
	if scope != "" {
		args = append(args, scope)
	}
	r, err := c.Runner()
	if err != nil {
		return "", err
	}
	return r.Capture(ctx, nil, args...)
}

// parseSecretList splits the output into the standard and custom sections and
// parses each. A missing header in a section means that section is empty.
func parseSecretList(raw string) (*Secrets, error) {
	std, custom := splitCustomSection(raw)
	out := &Secrets{}

	rows, err := coltable.Parse(std, secretStdHeader)
	if err != nil && !errors.Is(err, coltable.ErrNoHeader) {
		return nil, fmt.Errorf("secret list: %w: %w", client.ErrUnexpectedFormat, err)
	}
	for _, r := range rows {
		out.Stored = append(out.Stored, Stored{
			Scope:       normScope(r["SCOPE"]),
			Type:        r["TYPE"],
			Name:        r["NAME"],
			ValueMasked: r["SECRET"],
		})
	}

	crows, err := coltable.Parse(custom, secretCustomHeader)
	if err != nil && !errors.Is(err, coltable.ErrNoHeader) {
		return nil, fmt.Errorf("secret list: %w: %w", client.ErrUnexpectedFormat, err)
	}
	for _, r := range crows {
		out.Custom = append(out.Custom, Custom{
			Scope:       normScope(r["SCOPE"]),
			Target:      r["TARGET"],
			Env:         r["ENV"],
			Placeholder: r["PLACEHOLDER"],
			ValueMasked: r["SECRET"],
		})
	}
	return out, nil
}

// splitCustomSection splits raw at the "CUSTOM SECRETS" label line into the
// standard-table text and the custom-table text. With no label, everything is the
// standard section.
func splitCustomSection(raw string) (standard, custom string) {
	lines := strings.Split(strings.ReplaceAll(raw, "\r\n", "\n"), "\n")
	for i, ln := range lines {
		if strings.TrimSpace(ln) == "CUSTOM SECRETS" {
			return strings.Join(lines[:i], "\n"), strings.Join(lines[i+1:], "\n")
		}
	}
	return raw, ""
}

// normScope maps sbx's "(global)" to the SDK's "" global convention.
func normScope(s string) string {
	if s == "(global)" {
		return ""
	}
	return s
}
```

Leave `SetCustom`, `Remove`, `scopeArg`, and `CustomSecret` unchanged.

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./secret/ -v`
Expected: PASS (`TestSecretOps` and the four parse tests).

- [ ] **Step 5: Commit**

```bash
gofmt -w secret/
git add secret/secret.go secret/secret_test.go
git commit -m "feat(secret): List returns *Secrets; add ListRaw"
```

---

## Task 5: Live format contract test

**Files:**
- Create: `internal/integration/list_format_contract_test.go`

This is the runtime drift alarm: against a real `sbx`, `policy.List` / `secret.List` must not return `ErrUnexpectedFormat`. It is opt-in (`//go:build integration`), like the rest of the suite, and is not run by default CI.

- [ ] **Step 1: Write the contract test**

Create `internal/integration/list_format_contract_test.go`:

```go
//go:build integration

package integration

import (
	"context"
	"testing"

	"github.com/squall-chua/sbx-go-sdk/client"
	"github.com/squall-chua/sbx-go-sdk/policy"
	"github.com/squall-chua/sbx-go-sdk/secret"
	"github.com/stretchr/testify/require"
)

// TestContract_ListFormat guards the sbx table layout that policy.List and
// secret.List parse. If the CLI renames/reorders columns, strict header
// validation surfaces client.ErrUnexpectedFormat and this test fails so a
// maintainer can re-sync the parser — the table-format sibling of
// TestContract_VersionAlignment.
func TestContract_ListFormat(t *testing.T) {
	ctx := context.Background()
	c, err := client.New(ctx, client.WithAutoStart())
	require.NoError(t, err)

	_, perr := policy.List(ctx, c, "")
	require.NotErrorIs(t, perr, client.ErrUnexpectedFormat, "sbx policy ls table format drifted")
	require.NoError(t, perr)

	_, serr := secret.List(ctx, c, "")
	require.NotErrorIs(t, serr, client.ErrUnexpectedFormat, "sbx secret ls table format drifted")
	require.NoError(t, serr)
}
```

- [ ] **Step 2: Verify it compiles under the integration tag**

Run: `go vet -tags integration ./internal/integration/`
Expected: no errors (compiles). Running it requires a live daemon; if `sbx` is installed locally, optionally run:

Run: `go test -tags integration ./internal/integration/ -run TestContract_ListFormat -v`
Expected: PASS (or, if the local daemon's format has genuinely drifted, a clear failure naming which table).

- [ ] **Step 3: Commit**

```bash
git add internal/integration/list_format_contract_test.go
git commit -m "test(integration): live contract test for policy/secret ls table format"
```

---

## Task 6: Update docs

**Files:**
- Modify: `README.md` (the `policy.List`/`policy.Profiles` lines ~472-473; the `secret.List` line ~492; the plain-text caveat ~613)
- Modify: `skills/sbx-go-sdk/SKILL.md` (the "return raw text" bullet ~73)

- [ ] **Step 1: Update README policy/secret usage lines**

In `README.md`, change the policy list lines (currently):

```go
txt, _ := policy.List(ctx, c, "")                    // raw text (no --json upstream)
prof, _ := policy.Profiles(ctx, c)                   // raw text
```

to:

```go
rules, _ := policy.List(ctx, c, "")                  // []policy.PolicyRule
raw, _ := policy.ListRaw(ctx, c, "")                 // unparsed text escape hatch
prof, _ := policy.Profiles(ctx, c)                   // raw text (no --json upstream)
```

And change the secret list line (currently):

```go
txt, _ := secret.List(ctx, c, "")          // raw text
```

to:

```go
secrets, _ := secret.List(ctx, c, "")      // *secret.Secrets{Stored, Custom}
raw, _ := secret.ListRaw(ctx, c, "")       // unparsed text escape hatch
```

- [ ] **Step 2: Update the README plain-text caveat (~line 613)**

Change the caveat bullet (currently):

```
- **`policy` / `secret` list output is plain text** — no `--json` upstream; `List`/`Profiles`
```

to:

```
- **`policy.List` / `secret.List` parse the CLI's table into typed values** — no `--json`
  upstream, so the SDK parses the rendered table and returns `client.ErrUnexpectedFormat` if
  its columns drift; use `policy.ListRaw` / `secret.ListRaw` for the unparsed text.
  `policy.Profiles` is still raw text.
```

(If the original bullet wraps differently, preserve the surrounding lines; only the `List`/`Profiles` sentence changes.)

- [ ] **Step 3: Update SKILL.md (~line 73)**

In `skills/sbx-go-sdk/SKILL.md`, change:

```
- **`policy.List` / `policy.Profiles` / `secret.List` return raw text** (no `--json` upstream).
  `policy.Log` is the only structured policy reader.
```

to:

```
- **`policy.List` → `[]PolicyRule`, `secret.List` → `*Secrets`** by parsing the CLI table (no
  `--json` upstream); drift returns `client.ErrUnexpectedFormat`. Use `policy.ListRaw` /
  `secret.ListRaw` for raw text; `policy.Profiles` stays raw text.
```

- [ ] **Step 4: Verify the whole build and suite**

Run: `gofmt -l . && go build ./... && go test ./...`
Expected: `gofmt -l` prints nothing; build clean; all hermetic tests PASS.

- [ ] **Step 5: Commit**

```bash
git add README.md skills/sbx-go-sdk/SKILL.md
git commit -m "docs: structured policy/secret ls output + ListRaw escape hatch"
```

---

## Self-review notes (for the implementer)

- **Spec coverage:** API change (Tasks 3, 4), `ListRaw` (3, 4), types modelling exact columns (3, 4), `internal/coltable` (2), empty-vs-drift via header presence (2, 3, 4), `ErrUnexpectedFormat` in `client` (1), hermetic golden tests (2, 3, 4), live contract test (5), docs (6). `policy.Profiles` deliberately unchanged.
- **Type consistency:** `PolicyRule{Provenance, AppliesTo, Rule, Type, Decision, Resources}`; `Stored{Scope, Type, Name, ValueMasked}`; `Custom{Scope, Target, Env, Placeholder, ValueMasked}`; `Secrets{Stored, Custom}`; `coltable.Parse(raw, want) ([]map[string]string, error)` with `ErrNoHeader` / `ErrHeaderMismatch`. These names are used identically across all tasks.
- **Fixture alignment:** the parse-test fixtures are hand-aligned (column starts given in each task). If a test fails on a field boundary, re-check the spacing against the stated offsets before changing the parser.
```
