# Parse `sbx` table output into typed list APIs

> **Partly superseded by [ADR 0003](0003-reclaim-rest-paths-for-cp-unpublish-policy-list.md)
> (policy only; secrets unchanged).** The premise below is no longer true for policy: `sbx policy
> ls` gained `--json` in v0.35.0 and `GET /policy/network/rules` works from v0.37.0, so
> `policy.List` reads REST. `sbx secret ls` still has no `--json` and no REST path, so everything
> here still describes `secret.List` — which remains the only consumer of `internal/coltable`.
> The `policy.Profiles` decision in Consequences also still stands.

`sbx policy ls` and `sbx secret ls` have no daemon REST path and no `--json` flag, so the SDK
shelled them out and returned the raw, human-rendered table text — pushing the parsing burden
onto every caller. This extends the coupling ADR 0001 already worried about (shell-out flags
and REST structs pinned to a tested `sbx` range) to a more volatile surface: the rendered
table layout.

**Decision:** the SDK parses the rendered tables into typed values — `policy.List` →
`[]PolicyRule`, `secret.List` → `*Secrets{Stored, Custom}` — while keeping a `ListRaw` escape
hatch returning the unparsed text. Parsing is **header-anchored and strict**: a small
`internal/coltable` helper skips any preamble, validates the exact column header, and
computes column offsets dynamically from that header. Column *width* changes are absorbed
silently; a column-set/name/order change returns `client.ErrUnexpectedFormat` (the caller
falls back to `ListRaw`) rather than silently returning misaligned data. A live
`//go:build integration` contract test asserts the real headers, the table-format sibling of
the version contract test from ADR 0001. Types model exactly the visible columns, so they
stay a subset of any eventual upstream JSON.

## Considered Options

- **Keep returning raw text** — rejected: leaves every caller to parse a banner-prefixed,
  whitespace-aligned, multi-section table by hand.
- **Wait for upstream `--json`** — rejected: no such flag exists today, and the parse burden
  is real now. The header-anchored parser is isolated precisely so the eventual `--json`
  swap is confined to the flag + parser body, not the public types or call sites.
- **Best-effort lenient parsing** (map known columns, ignore the rest) — rejected: it would
  silently return partial or misaligned data when the format drifts, the opposite of the
  loud-on-drift behaviour chosen.

## Consequences

- A new failure mode, `client.ErrUnexpectedFormat`, signals table-format drift; callers that
  want resilience over structure use `ListRaw`.
- The drift alarm depends on the live contract test actually being run (it is opt-in, like the
  rest of `internal/integration`), so table drift is caught in that suite, not by default CI.
- `policy.Profiles` stays raw text — remote governance produces no rows in tested
  environments, so its layout is unverified and out of scope until real data exists.

## Ledger — where the SDK reads `sbx` output

Every site that depends on the shape of human-rendered output, so drift has one place to check.

| Site | What it reads | Why not something else |
|---|---|---|
| `secret.List` | the rendered table, via `internal/coltable` | no `--json`, no REST path |
| `kit.Validate` | a leading `INVALID:` on stderr, mapped to `client.ErrKitRejected` | `sbx kit validate` exits 1 for both a refused artifact and a missing path, so the exit code cannot tell them apart; only the prefix can. Verified 2026-07-27 against sbx v0.37.0. Note the marker covers a source refused by `kit.allowedSources` as well as a malformed `spec.yaml` — hence the sentinel is named for rejection, not invalidity. |
