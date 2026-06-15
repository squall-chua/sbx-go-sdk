# Structured output for `policy ls` and `secret ls`

## Goal

`policy.List` and `secret.List` currently shell out to `sbx` and return the raw, human-rendered
table text, forcing every caller to parse a banner-prefixed, whitespace-aligned, multi-section
table by hand. This change returns **typed values** instead, with a `ListRaw` escape hatch, and
isolates the parsing so a future upstream `--json` flag becomes a parser-internal swap rather
than a breaking change. See [ADR 0002](../../adr/0002-parse-sbx-table-output.md).

Scope: `policy ls` and `secret ls` only. `policy.Profiles` stays raw (remote-governance feature,
no rows to model/test). Module is v0.1.0 with no internal callers of these functions, so the
signature changes are acceptable breaking changes.

## Public API

```go
// policy
func List(ctx context.Context, c *client.Client, scope string) ([]PolicyRule, error) // was (string, error)
func ListRaw(ctx context.Context, c *client.Client, scope string) (string, error)     // new
func Profiles(ctx context.Context, c *client.Client) (string, error)                  // unchanged

// secret
func List(ctx context.Context, c *client.Client, scope string) (*Secrets, error)      // was (string, error)
func ListRaw(ctx context.Context, c *client.Client, scope string) (string, error)      // new
```

`List` is implemented as `raw := ListRaw(...)` followed by an isolated `parse*` call. When
upstream adds `--json`, only `List` (add the flag) and the `parse*` body (swap to
`json.Unmarshal`) change; return types and call sites do not. `ListRaw` keeps emitting today's
human text regardless.

## Types

Model **exactly the visible columns** — no speculative fields, splits, or `json` tags. This
keeps the types a subset of any eventual upstream JSON so the parser can absorb the swap.

```go
// policy — columns: PROVENANCE APPLIES_TO POLICY/RULE TYPE DECISION RESOURCES
type PolicyRule struct {
    Provenance string   // "local", or a remote-governance source
    AppliesTo  string   // "all" or a sandbox name
    Rule       string   // POLICY/RULE — rule name, or ID when unnamed (not split)
    Type       string   // "network"
    Decision   string   // "allow" | "deny"
    Resources  []string // hosts, gathered across continuation rows
}

// secret — standard table columns: SCOPE TYPE NAME SECRET
type Stored struct {
    Scope       string // "" = global, else sandbox name
    Type        string // "service" | "registry"
    Name        string // service name (openai) or registry host (ghcr.io)
    ValueMasked string // masked display value, e.g. "testte**" — never the real secret
}

// secret — CUSTOM SECRETS table columns: SCOPE TARGET ENV PLACEHOLDER SECRET
type Custom struct {
    Scope       string // "" = global, else sandbox name
    Target      string // target host
    Env         string // env var injected into the sandbox
    Placeholder string
    ValueMasked string // masked, e.g. "sk-cp-***...***5coY"
}

type Secrets struct {
    Stored []Stored // service + registry secrets (Type distinguishes)
    Custom []Custom // custom secrets
}
```

`Scope` is normalized: `sbx`'s `(global)` becomes `""`, matching the SDK's existing
`""`-means-global convention (see the **Scope** glossary entry).

## Parsing

A shared, independently-tested helper `internal/coltable` does the fixed-width work; the
`policy` and `secret` packages map rows to their structs and handle table-specific quirks.

`internal/coltable`:
1. **Skip preamble** — discard leading lines until the header (covers the daemon-start banner,
   blank lines).
2. **Validate header** — compare the header line's columns against the expected set, in order.
   Mismatch → return a sentinel so the caller can surface `client.ErrUnexpectedFormat`.
3. **Compute offsets dynamically** from the header's column start positions; slice each
   subsequent line at those offsets. Width changes in a given output are absorbed because
   offsets come from that same output's header.
4. Return each data line as its sliced fields keyed by column (`[]map[string]string`). It stays
   generic — it does **not** know about continuation rows; callers detect those themselves by
   checking which fields are blank.

`policy.parsePolicyList`:
- One `PolicyRule` per row with a non-blank `PROVENANCE`; continuation rows (blank before
  `RESOURCES`) append their resource to the current rule's `Resources`. Trailing blank
  resource lines are skipped.

`secret.parseSecretList`:
- Split on the `CUSTOM SECRETS` section header into the standard table and the custom table;
  parse each with its own expected header; normalize `Scope`.

### Empty vs. drift

- **Header present** → validate strictly and parse; zero data rows → empty result, `nil` error.
- **Header absent + zero exit** → empty result, `nil` error (a populated list always renders a
  header; the empty case prints e.g. `No secrets found for scope "X".`). No coupling to that
  wording.
- **Header present but wrong** (renamed/reordered/added columns) → `client.ErrUnexpectedFormat`.
- Non-zero exit is already surfaced as `cli.Error` by `Runner.Capture`, unchanged.

The one drift this misses at runtime — `sbx` dropping headers entirely — is caught by the live
contract test.

## Errors

Add to [client/errors.go](../../../client/errors.go), beside the existing sentinels:

```go
ErrUnexpectedFormat = errors.New("unexpected sbx output format")
```

`policy` and `secret` already import `client`, so they return `client.ErrUnexpectedFormat`
(joined with context). Callers `errors.Is` against it and fall back to `ListRaw`.

## Testing

- **Hermetic** (default `go test`): reuse the existing `recordingClient` pattern (a fake `sbx`
  shell script echoing canned output). Golden fixtures from the **real** captures:
  - `policy ls`: multi-resource continuation rows; daemon-banner preamble skipped; `allow`/`deny`.
  - `secret ls`: both sections (`Stored` service row + `Custom` row); `(global)`→`""`; masked
    values preserved verbatim.
  - Empty: `No secrets found ...` → empty `Secrets`, no error.
  - Drift: a mangled header → `client.ErrUnexpectedFormat`.
  - `internal/coltable`: offset slicing, width variation, header validation, continuation
    detection — tested in isolation.
- **Live contract** (`//go:build integration`, in [internal/integration/](../../../internal/integration/)):
  run real `sbx policy ls` / `secret ls` and assert the header rows equal the expected column
  sets — the table-format sibling of `version_contract_test.go`. Opt-in, like the rest of the
  integration suite.

## Docs

- [CONTEXT.md](../../../CONTEXT.md): glossary terms added (Scope, Secret, Custom Secret, Policy
  Rule, Provenance) — **done** during design.
- [ADR 0002](../../adr/0002-parse-sbx-table-output.md): recorded — **done**.
- README (~lines 472/492/613) and `skills/sbx-go-sdk/SKILL.md` (~line 73): update the "returns
  raw text" notes to the structured API + `ListRaw` escape hatch.

## Out of scope

- `policy.Profiles` (remains raw text).
- Any change to mutation commands (`policy allow/deny/...`, `secret set/rm`).
- Anticipating the upstream JSON schema (no speculative fields/tags).

## Decision log

| # | Decision | Rationale |
|---|----------|-----------|
| 1 | Strict header validation → `ErrUnexpectedFormat` + live contract test | Fail loud on display-format drift; never return silently-wrong structs. |
| 2 | Header *presence* is the empty signal | Avoids a second fragile coupling to the `No ... found` wording. |
| 3 | Model exactly the visible columns; no speculative fields/tags | Keeps types a subset of any future JSON, so the `--json` swap stays minor. |
| 4 | `secret.Secrets{Stored, Custom}` naming | Concise; distinct from the `secret.CustomSecret` input type. |
| 5 | Shared `internal/coltable` helper | One tested home for the fiddly, drift-sensitive offset logic. |
| 6 | `ErrUnexpectedFormat` in `client/errors.go` | Matches the existing exported-sentinel convention. |
```
