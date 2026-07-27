# Version alignment is a development-time gate, not a runtime one

`client.WithStrictVersion()` compared the daemon's `api_version` to `TestedAPIVersion` with exact
string equality and refused to construct a client on any difference. But `api_version` bumps on
every `sbx` release — `0.10.0` → `0.12.0` → `0.16.0` → `0.22.0` → `0.24.0` across five releases —
so the gate is guaranteed to fire on every upgrade. At v0.37.0 it fired on a release whose wire
types are **byte-identical** to v0.35.0: maximum disruption, zero signal. A real downstream
consumer (`sbx-swarm-node`, which passes `WithStrictVersion()` at `sdkbackend.go:35`) was left
unable to construct its backend at all.

Two facts remove any basis for a smarter runtime check: upstream **deleted `POST /version`** in
v0.37.0 — the daemon's own compatibility oracle, now 404 on every verb — and `api_version` carries
no documented semver contract, so any compatible-range logic the SDK invented would be fiction.

**Decision:** drift detection belongs in `TestContract_VersionAlignment`, which fails loudly at
development time, for the maintainer, with the binary in hand and a remediation hint.
`WithStrictVersion` and `CheckVersion` are deprecated; callers who want a runtime check compare
`DaemonHealth().Version` / `.APIVersion` against the exported constants themselves — which the
README already recommended in prose while the constructor option said otherwise.

## Consequences

- **The SDK no longer refuses an untested daemon.** A genuinely drifted daemon may now misbehave at
  runtime rather than failing fast at construction. This is the accepted cost of the decision, not
  an oversight: the alternative demonstrably converts a maintainer's TODO into a downstream outage
  the operator cannot fix.
- `CheckVersion` can never succeed, since its endpoint is gone. It stays exported but returns an
  error naming the removal, rather than surfacing a bare 404.
- `WithStrictVersion` keeps its current behaviour, so nothing breaks at compile time. It is marked
  deprecated and points at the `DaemonHealth` comparison.
- The contract test is opt-in (`//go:build integration`), so drift is caught only when that suite
  runs — the same caveat ADR 0002 records for table-format drift.

## Considered Options

- **Add a version range or minimum** (`WithMinAPIVersion`) — rejected: `api_version` has no
  documented ordering or compatibility semantics, and upstream removed the endpoint that used to
  answer the compatibility question. The SDK would be asserting a contract nobody offers.
- **Do nothing structural** (bump the pins, tell downstream to upgrade) — rejected: it leaves a
  constructor option that the SDK's own README warns against, and the next `api_version` bump
  breaks every strict caller again.
- **Delete `WithStrictVersion` and `CheckVersion`** — rejected: a compile break for callers, for no
  gain over deprecation.
