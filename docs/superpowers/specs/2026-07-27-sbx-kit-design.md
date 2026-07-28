# Design: `sbx kit` support

- **Targets:** `sbx` / `sandboxd` v0.37.0 (daemon API `0.24.0`)
- **Follows on from:** [v0.37.0 sync design](2026-07-27-sbx-v0.37.0-features-design.md), which deferred
  kit support to this spec
- **Companion decisions:** [ADR 0005](../../adr/0005-type-kit-strings-pass-structs-through.md) (new),
  [ADR 0002](../../adr/0002-parse-sbx-table-output.md) (gained a ledger entry),
  [ADR 0001](../../adr/0001-hybrid-cli-shellout-plus-rest.md) (shell-out default still applies)

## Background

`sbx kit` is six subcommands — `add`, `inspect`, `pack`, `pull`, `push`, `validate` — plus a
repeatable `--kit` flag on `sbx create`. Upstream marks the whole command EXPERIMENTAL, in its own
words: "this command may change or be removed in future releases."

The coverage table has carried two `deferred — kit spec` rows since v0.34.0 and one `gap` row since
v0.35.0. This spec closes all three.

Nothing about kits was written down in this repo before now, and the previous session's handoff
recorded the `spec.yaml` schema as "the single biggest unknown". It is no longer unknown. The schema
lives in `github.com/docker/sbx-kits-contrib/spec`, and DWARF gives its two central types:
`SpecFile` (24 fields, what a kit author writes) and `Artifact` (14 fields, what `kit inspect`
reports). They are **not** the same shape — see "Verified facts" below.

## Scope

In scope: all six subcommands, `--kit` on create, and reading a sandbox's kit list.

Out of scope: modelling `spec.yaml` for authoring (the SDK reads kits, it does not write them);
the `auth mode` and `active sessions` halves of coverage row 44; moving sandbox creation to REST.

## Verified facts this design rests on

Everything here was established by running commands against `sbx` v0.37.0 and `sandboxd` 0.24.0 on
2026-07-27, or by reading DWARF from `/usr/bin/sbx`. Nothing is inferred from documentation.

**A kit has two kinds.** `kind` must be `sandbox` or `mixin`. A `mixin` kit extends an existing
sandbox. A `sandbox` kit requires `sandbox.image` and supplies the base image instead — it extends
nothing. `CONTEXT.md`'s glossary described only the mixin case and has been corrected as part of
this work.

**Streams and exit codes** — this is what the SDK's error handling keys on:

| command | invalid artifact | warnings |
|---|---|---|
| `kit validate` | stderr `INVALID: <reason>`, exit 1 | stderr `WARN: <text>`, exit 0 |
| `kit inspect --json` | stderr `ERROR: <reason>`, exit 1 | inside the JSON, as `warnings` |
| `kit pack` | stderr `ERROR: pack: invalid artifact: <reason>`, exit 1 | not probed |

`validate` is the only subcommand that marks a validation failure differently from a load failure.
A missing path also exits 1, but prints `ERROR:`, not `INVALID:` — so the exit code alone cannot
tell the two apart, and the prefix can.

**`INVALID:` means more than "malformed".** It also fires when `kit.allowedSources` forbids the
reference's publisher, which is a settings refusal with nothing wrong with the kit:

```
$ sbx kit validate 'git+https://github.com/.../nope.git#dir=k'
INVALID: kit "git+https://..." cannot be installed — its source is not in your allowlist.
Your current kit.allowedSources:
  - docker.io/
```

**`validate` refuses OCI references** outright: `ERROR: OCI references are not supported for
validation; use a local directory or ZIP file`. `inspect` and `add` accept them. That refusal is an
`ERROR:`, not an `INVALID:`, so it does not collide with the sentinel.

**The input YAML shape is not the output JSON shape.** Flat `template` / `binary` / `runOptions`
keys are rejected on input (`use the 'sandbox:' block instead`), yet `kit inspect --json` emits all
three, derived from the `sandbox:` block. A probe kit setting every writable manifest field
confirmed 14 of the 15 JSON tags directly; only `build` is inferred, because it needs a build block.

**`inspect --json` omits the `files/` payload.** A probe kit containing `files/hello.txt` produced
JSON with no `files` key, while `kit pack` on the same directory wrote that file into the ZIP. The
report is a summary of the artifact, not the artifact.

**Relative local paths corrupt a sandbox's kit list.** The daemon records the kit list verbatim and
re-resolves the whole list on every later add, resolving a relative path against **its own** working
directory rather than the caller's:

| call | cwd | passed | recorded |
|---|---|---|---|
| `kit add kitprobe-b ./minkit3` | `/tmp/.../scratchpad` | `./minkit3` | `/home/mwchua/sbx-go-sdk/minkit3` ✗ |
| `kit add kitprobe-b "$SP/minkit2"` | `/tmp/.../scratchpad` | absolute | correct ✓ |
| `create --kit ./minkit2` | `/tmp/.../scratchpad` | `./minkit2` | `/home/mwchua/sbx-go-sdk/minkit2` ✗ |

The add reports success and the kit is applied, so the damage is invisible until a later, unrelated
add on the same sandbox dies with `re-resolve original kit 0 (...): path does not exist`. From then
on that sandbox cannot take any kit.

**`kit add` applies only part of a kit.** Refusals happen pre-flight and mutate nothing; four
refusals in a row left the sandbox usable and the next valid add still worked.

| spec field | `kit add` |
|---|---|
| `environment.variables` | applied |
| `caps.network` | applied |
| `commands.install` | applied |
| `agentContext` | applied |
| `commands.startup` | refused — "does not yet apply" |
| `commands.initFiles` | refused — "does not yet apply" |
| `credentials` (and `environment.proxyManaged`, which reports as `credentials`) | refused — "does not yet wire" |
| `publishedPorts` | refused — "does not yet publish" |
| `volumes` | refused — "does not yet pre-create" |

The binary carries two further refusals this design does not otherwise exercise: a sandbox using a
legacy git worktree, and a sandbox created before the kit-add recreate feature shipped.

**A sandbox's kit list is a label, not a `SandboxInfo` field.** `sandboxapi.SandboxInfo` has 14
fields in DWARF and none is a kit list; `internal/api/types_gen.go` matches it exactly, so coverage
row 44 is not stale generation. The list is `labels["com.docker.sandbox.kits"]`, a JSON string
array, returned by `GET /sandbox/{name}` — which the SDK already consumes:

```
after create --kit A:  ["…/minkit2"]
after kit add B:       ["…/minkit2","…/probe/p-install"]
```

## New package — `kit`

```go
// Package kit reads and packages kit artifacts (EXPERIMENTAL upstream).
package kit

func Inspect(ctx context.Context, c *client.Client, ref string) (Info, error)
func Validate(ctx context.Context, c *client.Client, ref string) error
func Pack(ctx context.Context, c *client.Client, dir, out string) error
func Push(ctx context.Context, c *client.Client, dir, ref string) error
func Pull(ctx context.Context, c *client.Client, ref, out string) error
```

All five shell out through `c.Runner().Capture`, per ADR 0001. No kit REST endpoints were found.

### `Info` and `Manifest`

One rule governs every field, and ADR 0005 records why: **a string or `[]string` is typed; a struct
is `json.RawMessage`.**

```go
// Info is what `sbx kit inspect --json` reports about a kit. It is a report,
// not the kit: the files/ payload is packed into the artifact but is not
// reported here.
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
```

`Manifest` carries all 15 DWARF fields. An earlier draft hand-picked six and silently dropped
`template`, which real output demonstrably contains — hence the rule, which cannot lose a field.

A JSON unmarshal failure in `Inspect` wraps `client.ErrUnexpectedFormat`, which already exists for
exactly this.

### `Validate`

```go
// Validate reports whether ref is a well-formed kit artifact
// (`sbx kit validate`). ref may be a directory, a ZIP file, or a git
// repository; unlike Inspect it may not be an OCI reference — the CLI
// refuses those for validation.
//
// A refusal returns an error wrapping client.ErrKitRejected. Warnings are
// not reported: a kit can be valid and still warn, and Inspect returns those
// structured, in Info.Warnings.
func Validate(ctx context.Context, c *client.Client, ref string) error
```

The OCI restriction is documented, not enforced. Pre-flighting it would mean re-implementing the
CLI's own judgement about what a reference looks like, giving a second place to be wrong when the
grammar shifts — the reasoning `WithPublish` already records for port specs.

Warnings are dropped deliberately. Scraping `WARN:` off stderr would be a second parsing site for
data `Inspect` already hands over cleanly. The doc comment is the only guard against a caller
running `Validate` in CI and never seeing "this field does nothing yet", so it must say so plainly.

### `Pack`, `Push`, `Pull`

`out` is a required argument on `Pack` and `Pull`, so the SDK always passes `-o`. Left to itself the
CLI derives a name from the kit and the artifact format and writes it into the calling process's
working directory — a terminal convenience that is a trap in a library.

## Changes in `sandbox`

```go
// sandbox/kit.go
func (s *Sandbox) AddKit(ctx context.Context, ref string) error
func (s *Sandbox) Kits(ctx context.Context) ([]string, error)

// sandbox/options.go
func WithKit(refs ...string) Option
```

`AddKit` and `WithKit` absolutise a reference when, and only when, `os.Stat` succeeds:

```go
// Local paths are made absolute before being passed on. The daemon records
// the kit list verbatim and re-resolves it on every later add, resolving a
// relative path against its own working directory rather than the caller's —
// silently recording a path that does not exist and breaking every
// subsequent add. Verified 2026-07-27 against sbx v0.37.0.
func absLocal(ref string) string {
	if _, err := os.Stat(ref); err != nil {
		return ref // OCI reference, git URL, or simply missing — the CLI's call
	}
	abs, err := filepath.Abs(ref)
	if err != nil {
		return ref
	}
	return abs
}
```

This is a filesystem question, not a grammar guess. Anything that does not stat passes through
untouched. It leaves one edge case: a caller with a local directory named `ghcr.io` gets it treated
as a path — which is what the CLI would do anyway.

`AddKit`'s doc comment carries the accept/refuse table verbatim, and notes that the sandbox's
container is recreated, that kit-owned volumes survive, and that the CLI refuses sandboxes predating
the recreate feature or using a legacy git worktree. None of that is enforced SDK-side: the CLI's
checks are pre-flight, mutate nothing, and its messages already name the remedy. Enforcing them here
would mean unmarshalling the `Caps`, `Credentials` and `Commands` blocks this design deliberately
leaves raw — re-typing the schema through the back door to duplicate a check against a list whose
every message says "does not *yet*".

`Kits` takes a context and calls `Inspect` first, then reads
`labels["com.docker.sandbox.kits"]` and unmarshals the JSON array. The refresh is not optional:
`Sandbox.info` is a snapshot taken at `Get` or `Create`, and `AddKit` recreates the container, so a
cached read would report the kit list as it was before the add. A missing label returns `nil, nil`;
a malformed value wraps `client.ErrUnexpectedFormat`. If upstream renames the key, `Kits` returns
nil rather than an error — a silent empty answer, which the doc comment names as a known risk.

`WithKit` follows `WithPublish` exactly: repeatable, appends, emits one `--kit` per reference, and
emits nothing when unset.

## Error handling

One new sentinel in `client/errors.go`:

```go
// ErrKitRejected reports that `sbx kit validate` refused an artifact. The CLI
// marks every refusal with an "INVALID:" line, whether the cause is a
// malformed spec.yaml or a source that kit.allowedSources forbids; the
// wrapped message carries which. Verified 2026-07-27 against sbx v0.37.0.
ErrKitRejected = errors.New("kit rejected by sbx")
```

The name matches the evidence. An earlier draft called it `ErrInvalidKit`, which would have told a
caller their kit was broken when the real answer was "your settings forbid that publisher".

Everything else surfaces the `cli.Error` unchanged; it already carries the exit code and stderr.

## Testing

**Unit, fake `sbx`** — the `recordingClient` shell-script pattern from `skillstore_test.go`.
Argument vectors: `Pack` and `Pull` always pass `-o`; `Push` puts the directory before the
reference; `WithKit` emits one `--kit` per reference and nothing when unset. Plus `absLocal` in
isolation: an existing relative path becomes absolute, an OCI-shaped string is untouched.

**Unit, error mapping** — a fake `sbx` exiting 1 with `INVALID:` on stderr must satisfy
`errors.Is(err, client.ErrKitRejected)`; one exiting 1 with only `ERROR:` must not. This is the test
that fails if upstream rewords its output.

**Integration, live `sbx`** (`//go:build integration`) — a fixture kit under
`internal/integration/testdata/`. `Validate` passes, `Inspect` returns the manifest, `Pack` writes a
ZIP into `t.TempDir()`. These need no daemon, no sandbox and no network.

One further live test earns its runtime:

```go
// AddKit must record an absolute path. A relative one is resolved by the
// daemon against its own working directory, silently recording a path that
// does not exist and breaking every later add on that sandbox.
func TestSmoke_AddKitRecordsAbsolutePath(t *testing.T)
```

It writes the fixture under `t.TempDir()`, changes into it, calls `sb.AddKit(ctx, "./fixture-kit")`
with a **relative** reference, and asserts `sb.Kits()` contains the correct absolute path. A unit
test can only prove we passed an absolute path, not that the daemon recorded the right one — which
is the thing that actually breaks. It exercises `Kits()` label parsing on real daemon output for
free.

The fixture must stay inside the four fields `kit add` accepts, or the CLI refuses it pre-flight and
the test proves nothing. That constraint belongs in a comment on the fixture.

## Docs

Three of these landed with this spec, since they record decisions rather than describe code:

- `CONTEXT.md` — Kit entry rewritten for the two kinds and the four source forms. **Applied.**
- `docs/adr/0005-type-kit-strings-pass-structs-through.md` — new. **Applied.**
- `docs/adr/0002-parse-sbx-table-output.md` — ledger of every site that reads `sbx` output, now
  naming the `INVALID:` prefix match. **Applied.**

The rest belong to the implementation:
- `docs/sbx-version-coverage.md` — rows 37 and 45 become covered; row 44 becomes partly covered,
  naming `Sandbox.Kits()` and leaving auth mode and active sessions as gaps. New rows for the six
  subcommands and `--kit` on create.
- `REVERSE_ENGINEERING.md` — where the schema came from (`sbx-kits-contrib/spec`, types `SpecFile`
  and `Artifact`), and that `SandboxInfo` carries no kit field.
- `README.md` limitations — `inspect --json` omits `files/`; `Push` and `Pull` are unverified
  against a live registry; relative kit paths are absolutised and why.

## Not verified

Stated so nobody mistakes silence for coverage:

- **`Push` and `Pull` against a real registry.** There is no OCI registry on this host. Both ship
  with argument-vector unit tests only.
  **Refined 2026-07-28** (`.superpowers/sdd/2026-07-27-sbx-kit-support/probe-findings.md`): still
  not verified end-to-end, but the blocking mechanism is now characterized and it is not what this
  design assumed. `Pull` (like `Inspect`) is blocked by `kit.allowedSources` instantly on this
  host. `Push` bypasses the allowlist entirely: against a forbidden-and-unreachable target it opens
  real connections to registry infrastructure and then retries indefinitely, with no output even
  under `--debug`. A reachable registry is still needed to observe either succeeding.
- **Whether `kit add` behaves differently on a stopped sandbox.** Only a running one was tested.
  **Resolved 2026-07-28** (`.superpowers/sdd/2026-07-27-sbx-kit-support/probe-findings.md`):
  `kit add` on a stopped sandbox auto-starts it ("Sandbox ... started successfully"), then runs the
  identical recreate flow. It never refuses and never leaves the sandbox stopped.
- **Git references.** `kit.allowedSources` refused them before they resolved.
  **Resolved 2026-07-28** (`.superpowers/sdd/2026-07-27-sbx-kit-support/probe-findings.md`):
  re-confirmed for `validate`, and extended to `inspect` — both refuse a forbidden git reference
  instantly, with no evidence of a clone attempt. `inspect --json`'s refusal is also plain text on
  stderr, not JSON.
- **Legacy `schemaVersion: "1"` kits.** Only `"2"` was exercised.
  **Resolved 2026-07-28** (`.superpowers/sdd/2026-07-27-sbx-kit-support/probe-findings.md`): a
  `"1"` kit validates and inspects fine, and `inspect --json`'s output shape is identical to a
  `"2"` kit's — only the `schemaVersion` value differs. `kit.Info`/`kit.Manifest` decode it
  unchanged.
- **Whether `create --kit` accepts git references at all.** Its help says "directory, ZIP, or OCI";
  `kit add`'s help adds git.
  **Resolved 2026-07-28** (`.superpowers/sdd/2026-07-27-sbx-kit-support/probe-findings.md`):
  `create --kit` does accept a git reference — it reaches the identical `resolve kits:` allowlist
  stage `kit add` reaches, refusing with the same message, not a format error. `--help` is
  incomplete, not authoritative. No sandbox is created when the reference is refused.
- **The `build` JSON tag** on `Manifest`, inferred rather than observed.
  **Resolved 2026-07-28** (`.superpowers/sdd/2026-07-27-sbx-kit-support/probe-findings.md`), with a
  correction to the method: DWARF carries no struct tags at all (confirmed empirically), so the tag
  could never have come from the DWARF pass that confirmed the other 14 fields. It is instead
  confirmed by a single occurrence of `json:"build,omitempty"` found via `strings` on the binary,
  corroborated by four sibling `BuildConfig` tags matching DWARF field names one-for-one. Real
  `inspect --json` output containing a `build` key remains unobtainable on this host:
  `sandbox.build` is refused pre-flight as "accepted in the schema but not yet implemented".

## Non-goals

- Typing `spec.yaml` for authoring kits.
- Wrapping `/sandbox/{name}/swap-container`, found in the binary. `kit add` does substantial work
  around it — re-resolving the recorded list, the accept/refuse check — and shelling out gets all of
  that for free.
- Fixing `sbx-swarm-node`, which still passes `WithStrictVersion()` at
  `internal/sandbox/sdkbackend.go:35`. That is a separate change in that repo.
