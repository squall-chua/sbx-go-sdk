# Type kit strings, pass kit structs through

`sbx kit inspect --json` returns fully structured data, so the SDK could mirror the whole schema in
Go types. `kit.Info` deliberately does not. Strings and string slices are typed; every struct-valued
field is `json.RawMessage`.

The schema is large and unstable. DWARF from `sbx` v0.37.0 gives
`github.com/docker/sbx-kits-contrib/spec` two central types: `SpecFile`, what a kit author writes,
with 24 fields of which **eight are named `Legacy*`** and two (`credentials`, `volumes`) accept
either a list or a legacy map through custom unmarshalers; and `Artifact`, what `inspect` reports,
with 14. Mirroring `Artifact` fully means about 17 Go types, because `Credentials` alone reaches
`ApiKey`, `ApiKeyInject`, `OAuth` and five OAuth sub-types. Upstream marks the whole command
EXPERIMENTAL: "this command may change or be removed in future releases."

This repo has been bitten twice by wrapping surfaces that then moved — `sbx ssh setup` became
`sbx setup ssh`, and `POST /version` was deleted outright. The `InspectRaw` / `Profiles` precedent
exists for exactly this situation: pass output through when there is no stable contract.

But passing *everything* through would discard the part that is stable and that every caller wants —
a kit's identity. And a hand-picked middle ground failed on its first attempt: an earlier draft of
`Manifest` selected six fields it judged useful and silently dropped `template`, which real output
demonstrably contains for `kind: sandbox` kits. Judgement per field is how data goes missing.

**Decision:** one mechanical rule decides every field. **A `string` or `[]string` is typed; a struct
is `json.RawMessage`.** No field is omitted. `Manifest` carries all 15 of its DWARF fields under the
same rule, so `kit.Info` is a lossless view of the JSON — typed where the shape is a scalar and
cannot churn, raw where the shape is a struct and will.

## Considered Options

- **Mirror the full schema in Go types** — rejected: about 17 types tracking an EXPERIMENTAL schema
  that already carries eight legacy fields and two dual-shape unmarshalers. Every upstream change
  becomes an SDK change, and a missed one becomes silently dropped data.
- **Return raw JSON bytes only**, per `InspectRaw` — rejected: it makes the caller unmarshal even to
  read a kit's name and version, the one part of the schema with no churn risk. `InspectRaw` exists
  because `policy inspect` has *no* structured output; `kit inspect` has `--json`.
- **Hand-pick the useful fields** — rejected on evidence. The first attempt at this dropped
  `template` while its author believed the struct complete. A rule cannot make that mistake; a
  judgement call already did.

## Consequences

- Reading a policy block costs a second `json.Unmarshal` into a shape the caller defines. That is
  the deliberate cost: callers who need `caps.network` opt into a shape that may change, instead of
  the SDK guaranteeing one it cannot.
- `Manifest` is 15 fields, four of them raw, and nine are only meaningful for `kind: sandbox` kits.
  A mixin user sees a lot of empty struct. Completeness was chosen over tidiness.
- Adding an upstream field is a one-line diff, and the rule says which line without discussion.
- The rule will look unfinished to a future maintainer, whose instinct will be to type the raw
  fields. This ADR is the answer to that instinct. If upstream drops the EXPERIMENTAL marker and
  commits to the schema, revisit it then — that is a breaking API change and needs its own decision.
- `kit.Info` is named `Info`, not `Artifact` as upstream names it, because it is a report and not
  the artifact: `inspect --json` omits the `files/` payload that `kit pack` writes into the ZIP.
