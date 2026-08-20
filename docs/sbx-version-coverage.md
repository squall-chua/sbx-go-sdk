# sbx feature coverage by release

What each `sbx` release added, and whether this SDK covers it. The SDK debuted
against v0.32.0 (daemon API `0.10.0`).

**Maintainers:** reconcile this table against the upstream release notes on every
version sync. `TestContract_VersionAlignment` points here when it detects drift.
The v0.35.0 sync shipped while silently missing five v0.35.0 features — this
table exists so that cannot happen quietly again.

Status values: **covered** · **partly covered** (some sub-feature is a gap) ·
**gap** (upstream feature the SDK does not expose) · **deferred** (planned,
with a named spec) · **n/a** (needs no SDK surface).

| Release | Daemon API |
|---|---|
| v0.32.0 | `0.10.0` (SDK debut) |
| v0.33.0 | `0.12.0` |
| v0.34.0 | `0.16.0` |
| v0.35.0 | `0.22.0` |
| v0.36.0 | never released |
| v0.37.0 | `0.24.0` |
| v0.38.0 | `0.26.0` |
| v0.39.0 | `0.26.0` (unchanged) |

| Feature | sbx | SDK | Status |
|---|---|---|---|
| Sandbox create / run / exec / cp / ports / templates | v0.32.0 | `sandbox`, `exec`, `template` | covered |
| Policy allow / deny / rm / reset / log | v0.32.0 | `policy` | covered |
| `sbx diagnose` | v0.32.0 | `Client.Diagnose` | covered (v0.38.0 sync) — shells out to `diagnose -o json`, whose JSON output arrived in v0.38.0. `--upload` stays unwrapped on purpose: shipping host diagnostics to Docker support should be an explicit act, not a library side effect |
| `sbx login` / `sbx logout` | v0.32.0 | `Client.Login`, `Client.Logout` | covered (v0.38.0 sync) for the scriptable paths — `login --username U --password-stdin` (token via stdin, never argv) and `logout --yes`. Bare `sbx login` opens a browser for OAuth and stays unwrapped. Neither is exercised live: one needs real Docker credentials, the other would sign the maintainer out and stop every running sandbox |
| Stable per-sandbox `id` | v0.33.0 | `Sandbox.ID()` | covered |
| `mount_policy_denied` | v0.33.0 | `Sandbox.MountPolicyDenied()` | covered |
| `sbx cp -L` follows source symlinks | v0.33.0 | `WithFollowSymlinks` | covered |
| `secret set-custom --host` wildcards | v0.33.0 | `secret.SetCustom` | covered — pattern passes through |
| Experimental SSH endpoint | v0.34.0 | `ssh` | covered |
| `sbx setup` (interactive host-config wizard) | v0.34.0 | `Client.DetectSetup`, `DetectSetupRaw` | covered for the detection half — the "no non-interactive flag" reading missed that the command *degrades*: with no terminal it prints a read-only report (prerequisites, agent secrets, skills, governance, host MCP servers), changes nothing, and exits 0. The SDK forces that path with an empty stdin. Acting on the findings stays with `secret.ImportAll`, `skillstore.Import`, `policy.SetDefault` and the `mcp` package |
| `policy set-default` renamed `policy init` | v0.34.0 | `policy.SetDefault` | covered — calls `policy init` |
| Kit source allowlist (`kit.allowedSources`) | v0.34.0 | `settings.Set` | covered — generic settings |
| OCI v2 kit artifact streaming | v0.34.0 | `kit.Push`, `kit.Pull` | covered — format follows the kit's `schemaVersion`; unverified against a live registry |
| `sbx kit inspect` / `validate` / `pack` | v0.34.0 | `kit.Inspect`, `kit.Validate`, `kit.Pack` | covered |
| `sbx create --kit` / `sbx run --kit` | v0.34.0 | `sandbox.WithKit` | covered — emitted by both create and run |
| Published ports restored on restart | v0.34.0 | `sandbox.Ports` | n/a — daemon-side |
| `sbx policy check network` | v0.35.0 | `policy.Check` | covered (v0.37.0 sync) — `sbx policy check --help` lists only the `network` subcommand, so there is no sibling to add a row for |
| `sbx policy inspect` | v0.35.0 | `policy.InspectRaw` | covered (v0.37.0 sync) |
| `policy ls --wide/--source/--decision/--include-inactive` | v0.35.0 | `policy.PolicyRule` | covered for data, client-side for filtering — `--wide` adds no field `PolicyRule` lacks (`ID`, `Name`, `PolicyID`, `Scope`, `AppliesTo`, `ResourceType`, `Decision`, `Resources`, `Origin`, `Status`, `Editable` are all exported), and `--source`/`--decision` are one `slices.DeleteFunc` on the returned slice, so no SDK option is warranted. Whether `GET /policy/network/rules` can return inactive rules is **unresolvable from the client**: the endpoint ignores unknown query parameters (a deliberately bogus one returns a byte-identical 200 response), so `include_inactive=true` proves nothing, and every rule on a host without remote governance reports `status: active`. Settling it needs a daemon with an inactive rule. Re-probed at v0.38.0: still byte-identical with and without `include_inactive=true`, so this stays open. |
| `sbx secret import` | v0.35.0 | `secret.Import` / `ImportAll` | covered (v0.37.0 sync) — `--force` stays opt-in via `WithOverwriteExisting`, unlike `skillstore.Import` which always forces; skill replacement is recoverable (the CLI backs up the folder first), a credential overwrite is not |
| `sbx rm --force` for an active session | v0.35.0 | `Remove(WithForce())` | covered (v0.37.0 sync) |
| `sbx inspect` (kits, auth mode, active sessions) | v0.35.0 | `Sandbox.Summary`, `Sandbox.Kits` | covered (v0.38.0 sync) — `sbx inspect --json` carries auth mode, session count, injected secrets and MCP-gateway state, none of which `api.SandboxInfo` has. `Sandbox.Kits` still reads the `com.docker.sandbox.kits` label and stays the REST-only path |
| `sbx kit add` recreates container, applies kit policy | v0.35.0 | `Sandbox.AddKit` | covered — applies `environment.variables`, `caps.network`, `commands.install`, `agentContext`; the CLI refuses kits declaring `credentials`, `publishedPorts`, `volumes`, `commands.startup` or `commands.initFiles` |
| `GET /health` removed; `/daemon/health` is liveness | v0.35.0 | `Client.Health` | covered |
| `credential_sources` on `SandboxInfo` | v0.35.0 | `api.SandboxInfo` | covered |
| SOCKS5 upstream proxy, `DOCKER_SANDBOXES_PROXY` | v0.35.0 | — | n/a — env var |
| `DOCKER_SANDBOXES_NO_PROXY` | v0.35.0 | — | n/a — env var |
| virtiofs cache default on, `..._ENABLE_VIRTIOFS_CACHE` | v0.35.0 | — | n/a — env var |
| Shared agent skills store, `sbx skills import` | v0.37.0 | `skillstore.Import` | covered |
| `--no-share-skills` on create | v0.37.0 | `sandbox.WithoutSharedSkills` | covered (v0.38.0 sync) — the earlier "gated off" reading was wrong. `feature.shareSkills` only controls whether cobra *shows* the flag in `--help`; the flag is registered and parses on both `create` and `run` regardless (verified with the feature off, against a bogus flag as the control). Whether it changes behaviour still depends on the feature being active |
| `-p/--publish` on create and run | v0.37.0 | `sandbox.WithPublish` | covered |
| `sbx setup ssh` replaces `sbx ssh setup` | v0.37.0 | `ssh.Setup` | covered |
| `feature.ssh` defaults to enabled | v0.37.0 | `ssh.Enabled` | covered |
| `POST /version` removed | v0.37.0 | `Client.CheckVersion` | covered — deprecated, see ADR 0004 |
| `GET /sandbox/{n}/files` now works | v0.37.0 | `Sandbox.CopyFrom` | covered — REST, see ADR 0003 |
| `POST /sandbox/{n}/ports/unpublish` | v0.37.0 | `Sandbox.UnpublishPort` | covered — REST, see ADR 0003 |
| `GET /policy/network/rules` now works | v0.37.0 | `policy.List` | covered — REST, `type=all` |
| `GET /policy/network/profiles` | v0.37.0 | `policy.ProfileNames` | covered — "no data to model" conflated an empty response with an unknown shape. DWARF settles it: `sandboxapi.PolicyProfilesListResponse` holds a lone `[]string`, so profiles are names and nothing more. `ProfileNames` is REST and typed; the older `Profiles` keeps its `(string, error)` signature and is deprecated rather than changed, because `sbx-swarm-node` calls it — a typed `Profiles` broke that build, caught by the go.work check below |
| `POST /daemon/reset` | v0.37.0 | `Client.Reset` | covered |
| `DOCKER_SANDBOXES_PROXY=system` | v0.37.0 | — | n/a — env var |
| Governance org support messages | v0.37.0 | `policy.Check` → `Authorization.Governance` | covered — fields decoded (`Active`, `Organization`, `OrganizationUnavailable`, `LastSyncedStatus`, `LastSyncedMessage`); live behaviour unverified, no governed org on this host |
| `sbx secret set --oauth` | v0.37.0 | `secret.SetOAuth` | covered — "needs a browser" was the wrong conclusion. With no TTY the command prints the authorization URL (on **stderr**) and blocks on a loopback callback, so the SDK hands the URL to an `onURL` callback and blocks until consent lands or the context ends. URL emission, blocking and cancellation are verified; the success path needs a human at a browser. `openai`/global only |
| MCP gateway: `sbx mcp add/ls/rm/inspect/load` | v0.38.0 | `mcp.AddRemote`, `AddLocal`, `List`, `Remove`, `Inspect`, `Load` | covered |
| `sbx mcp auth <server>` (authorize / reauthorize) | v0.38.0 | `mcp.Authorize` | covered — same degradation as `secret set --oauth`, except the URL goes to **stdout**. Shared implementation in `internal/oauthflow`. A merely expired credential is refreshed without consent, so `onURL` may never fire on a successful call |
| `sbx mcp auth status` / `auth rm` | v0.38.0 | `mcp.AuthStatus`, `mcp.AuthRemove` | covered — both return `[]mcp.AuthResult`. The shape was settled by registering a real OAuth server with `--skip_auth`, which records the registration without running the browser flow: `[{"server_name":"…","status":"unauthorized"}]`. The remaining fields (`credential_id`, `authorization_url`, `error`) are `omitempty` and appear only once a credential exists; `Status`'s full value set is undocumented upstream |
| `GET /mcp/gateway-mode` | v0.38.0 | `Client.MCPGatewayMode` | covered — REST; reports local vs hosted gateway and why |
| `--static-mcp` on create and run | v0.38.0 | `sandbox.WithStaticMCP` | covered — emitted as repeated flags by both create and run |
| `--deny-network` on create and run | v0.38.0 | `sandbox.WithDenyNetwork` | covered |
| `sbx daemon restart` | v0.38.0 | `Client.RestartDaemon` | covered |
| Secrets default to global scope; `-g` and positional sandbox deprecated | v0.38.0 | `secret` package | covered — the SDK now emits `--sandbox NAME` or no scope argument at all. The old spellings still work but print a deprecation warning into the output the SDK parses |
| Registry credentials default to host-only scope | v0.38.0 | `secret.SetRegistry` + `WithHostOnly`, `secret.HostOnlyScope` | covered — global scope now emits `--all-sandboxes`, since a bare `secret set --registry` means host-only |
| `settings list --all` exposes feature flags | v0.38.0 | `settings.ListAll` | covered — before this they were readable only one at a time via `Get` |
| `requires_restart` / `feature_flag` on a setting | v0.38.0 | `settings.Setting` | covered |
| Kit spec v2 (`permissions`, `setup`, `agentInstructions`) | v0.38.0 | `kit` package | n/a — authoring-side schema. `schemaVersion: "2"` now has its own decoder, so a v2 spec must use the new key names, but at v0.38.0 `kit inspect --json` still reported the normalized v1 shape and `kit.Info` was unchanged. The integration fixture was migrated. v0.39.0 flipped the output to the v2 shape too — see the v0.39.0 section |
| `sbx cp` copy-out destination escape (CVE-2026-17106) | v0.38.0 | `internal/untar` | n/a — the SDK's `Sandbox.CopyFrom` extracts the REST tar itself through `os.Root` with explicit symlink and hardlink target checks, so it never had the CLI's escape. Upgrading sbx fixes the CLI path |
| `sbx inspect` shows custom secrets | v0.38.0 | `Summary.Secrets` | covered — a sandbox-scoped custom secret lists as `{name, source:"custom"}`, verified live |
| Structured create/run progress output | v0.38.0 | — | n/a — `sandbox.Create` owns the name and never parses create output |
| `sbx diagnose -o json` / `--upload` | v0.38.0 | `Client.Diagnose` | partly covered — `-o json` is what `Diagnose` parses; `--upload` is deliberately not wrapped. Distinct from `Client.Diagnostics`, which is the daemon's own `/daemon/diagnostics` report |

## v0.39.0

The daemon REST API did not move: `api_version` stayed `0.26.0`, and the
`sandboxapi.New<Op>Request` symbol set is identical to v0.38.0. Every change
below is CLI-side, apart from one additive wire field.

| Feature | sbx | SDK | Status |
|---|---|---|---|
| `kit inspect --json` reports the flat kit spec v2 shape | v0.39.0 | `kit.Info` | covered — **breaking**. The `manifest` wrapper is gone, so `kit.Manifest` was deleted and `info.Manifest.Name` became `info.Name`. **Nothing was lost; several fields moved**, and the full relocation map is in `kit.Info`'s doc comment. The non-obvious half: `Template` → `Sandbox.image`, `Binary` → `Sandbox.entrypoint`, `RunOptions` → `Sandbox.command.default`, `InteractiveOptions` → `Sandbox.command.interactive`, `AIFilename` → `AgentInstructions.filename`, `PublishedPorts` → `Ports`. `Resources.memoryMB` also changed shape, to a unit string like `"2048m"`. A v1 kit is normalized up to the same shape and gains a deprecation entry in `Warnings` per legacy key. The clean break was chosen over half-filling the old fields: a compile error tells a downstream consumer what happened, a silently empty `Caps` does not |
| `mcp ls` is grouped by gateway, not a table | v0.39.0 | `mcp.List` | covered — **breaking**. The NAME/TYPE/URL-COMMAND header is gone, so the old `coltable` parse returned an empty slice and a nil error: a silent wrong answer, the worst failure mode available. `mcp.Server.Target` is dropped because the listing no longer carries the URL or command at all; `Transport` and `Status` replace it, and `mcp.Inspect` remains the way to read an endpoint. Still no `--json` or `--format` flag on `mcp ls` |
| `diagnose -o json` exits non-zero when a check fails | v0.39.0 | `Client.Diagnose` | covered — the report is returned with a nil error however the CLI exited, since a failing check is the diagnosis, not a failure to diagnose. The exit code surfaces only when nothing decodes. Not strictly new upstream behaviour, but it never bit until this host had a genuinely failing check |
| `SandboxInfo.stopped_at` | v0.39.0 | `api.SandboxInfo.StoppedAt` | covered — typed from DWARF and left `omitempty`. The daemon does not emit the key yet, verified against a stopped sandbox, so nothing reads it today. It is what `prune --filter since=DURATION` sorts on |
| `SandboxCreateRequest.UsbDevices` | v0.39.0 | — | n/a — creation shells out and `sbx create` has no USB flag. Pairs with the new `feature.sandbox-usb` flag |
| `sbx env` (`.sbxenv.yaml` declarative environments) | v0.39.0 | — | gap — `env create/run/exec/rm`. Deep-merges several files with docker-compose `-f` semantics, provisions sandbox-scoped secrets, and `env rm` removes what it created |
| Kit signing: `kit sign` / `verify` / `provenance`, `push --sign` | v0.39.0 | `kit.Sign`, `Verify`, `Provenance`, `WithSign` | covered — a failed verification is `client.ErrSignatureInvalid`, following the `ErrKitRejected` pattern, while a misuse (keyless with no accepted identity) stays a plain `*CLIError` so the two can never be confused. Key-based signing of a local directory is verified live end to end, including that editing the spec afterwards invalidates the signature. Keyless needs an OIDC provider and the OCI paths need a registry, so both are shape-only — as is `Provenance`, which only exists on a pushed kit. Governed by `kit.requireSignature`, `kit.trustedSigners` and `kit.ignoreTransparencyLog`, already reachable through `settings.Set` |
| `create` / `run --env` and `--env-file` | v0.39.0 | `sandbox.WithEnv`, `WithEnvFile` | covered — closes the long-standing "the CLI cannot pass `Environment`" entry below, which is why this is the highest-value row of the release. All three forms are verified live inside a real sandbox: `KEY=VALUE`, a bare `KEY` inherited from the calling process, and a file. Files are emitted before variables, matching the CLI's documented precedence |
| `secret set` / `set-custom` external resolvers | v0.39.0 | `secret.FromRef`, `FromCommand`, `WithRefresh`, `WithoutVerify`, `WithResolverErrors` | covered — options rather than new functions, so `SetToken` and `SetCustom` keep their signatures (`SetCustom` gained a variadic tail). Exactly one value source is allowed: a literal, `FromRef`, or `FromCommand`; every other combination is refused before the CLI is invoked. `--command` is verified live; `--ref` is shape-only, since this host has neither the `op` binary nor AWS credentials. `-t/--token` is deliberately not wrapped — it puts the secret in the argument vector, which is exactly what `SetToken`'s stdin path exists to avoid |
| `sbx prune` | v0.39.0 | `sandbox.Prune` | covered — returns the names removed, or with `WithDryRun` the names it would remove; `WithStoppedLongerThan` sets `--filter since=`. Always passes `--force`, because the CLI refuses outright on a non-interactive stdin rather than prompting — and that flag also removes a stopped sandbox still in use, which the doc comment calls out. A dry-run table missing its header is `ErrUnexpectedFormat`, never an empty list |
| `skills ls` | v0.39.0 | `skillstore.List` | covered — reports the store path and its folder names. A missing path line is `ErrUnexpectedFormat`, not an empty store, because without it a skill name cannot be told from a stray message |
| `setup ssh remove` | v0.39.0 | `ssh.RemoveSetup` | covered — undoes what `ssh.Setup` wrote for this app instance. Distinct from `ssh.Disable`, which turns off the `feature.ssh` setting |
| `template save -o` | v0.39.0 | `sandbox.WithExport` | covered — the template is registered either way; `-o` only adds the tar export |
| `daemon status --json` | v0.39.0 | `Client.DaemonLogPath` | covered — the log path is the only field `--json` adds that the SDK lacked. It is its own shell-out so that `DaemonStatus` stays pure REST and keeps working with no CLI binary |
| `reset --preserve-secrets` | v0.39.0 | — | gap, deliberately unresolved. `POST /daemon/reset` takes no body — there is no `ResetRequest` type in DWARF — so the flag is CLI-side only, and `Client.Reset` is REST. Which of the two spellings preserves secrets cannot be settled without running a real reset, which wipes every sandbox and secret on the host. Settle it on a throwaway host, then decide whether `Reset` grows an option or gains a CLI sibling |
| `secret import [SERVICE]` and `--all` | — | `secret.Import` / `ImportAll` | covered, and **not new in v0.39.0** — the v0.37.0 recon simply never recorded the positional or the flag. The SDK has emitted both since that sync. Listed here only to retract the claim |
| `sbx ssh` hidden from the root help | v0.39.0 | `ssh` package | n/a — cobra visibility only. `sbx ssh setup` and `sbx setup ssh` both still work, so `ssh.Setup` is unaffected |
| Feature flags: 9 → 23 | v0.39.0 | `settings.ListAll` | covered — generic. New are `feature.sandbox-usb`, `feature.network-user-prompts`, `update.channel`, and the `feature.sbx-api` family (`sbx-api` plus ten `sbx-api-*` sub-flags). That family looks like a gated public REST API and is worth probing next sync: if it exposes creation over REST, `sandbox.Create` could stop shelling out |
| Non-flag settings: 18 → 23 | v0.39.0 | `settings.Set` / `Get` | covered — generic. New are `claude.remoteControl`, `kit.ignoreTransparencyLog`, `kit.requireSignature`, `kit.trustedSigners`, `platform.images.registryMirror` |

## Create-request fields the daemon accepts but the CLI cannot pass

`SandboxCreateRequest` carries `SecretsScope`, `PullPolicy`,
`RootFilesystemSize`, `DindVolumeSize`, `EnableVirtiofsCache`, `Display`,
`AgentOptions`, `BindingsPath`, `CredentialValues` and `UsbDevices`. Sandbox
creation shells out to `sbx create`, which exposes none of them, so they are
unreachable until creation moves to REST. Recorded here so the list is not
rediscovered each sync.

`Environment` left this list in v0.39.0: `create` and `run` gained `--env` and
`--env-file`. The SDK does not emit them yet — see the gap row above.

## Verifying downstream source compatibility

The v0.37.0 sync's public-signature guarantee is proven against
`sbx-swarm-node`, a real downstream consumer. Building that repo directly does
**not** prove it: it pins a released `sbx-go-sdk` version with no `replace`
directive, so `go build` there resolves the module from the module cache and
never touches this branch's code. To actually exercise this branch's code
against that consumer, use a throwaway `go.work` that adds both module
directories, then run `GOWORK=<path-to-that-file> go build ./...` (and any
targeted `go vet`) from the downstream repo — that forces the downstream
module to resolve `sbx-go-sdk` to the local working tree instead of the cache.
Do this again on the next sync rather than trusting a plain build in the
downstream repo.

**The v0.38.0 sync earned that warning.** Typing `policy.Profiles` as
`([]string, error)` — a clean, house-style change — broke
`sbx-swarm-node/internal/sandbox/sdkbackend.go:623` at compile time. The
go.work build is what caught it; nothing in this repo's own tests could have.
The fix follows the ADR 0004 pattern: `Profiles` keeps its `(string, error)`
signature and is deprecated, and the typed call is the new `ProfileNames`.
Every other v0.38.0 change is additive.

Note the go.work file needs a full `go` version (`go 1.25.0`, not `go 1.25`),
or the build fails before it compiles anything.

**The v0.39.0 sync ran this check and it failed, as designed.** The `kit.Info`
break is intentional this time, not an accident like `policy.Profiles` was, but
the check is still what turned "downstream might be affected" into an exact
list. One file, one function, five lines:
`sbx-swarm-node/internal/sandbox/sdkbackend.go`'s `inspectKit`, which read
`Kind`, `Resources`, `RunOptions`, `Template` and `Volumes` off `info.Manifest`.

The fix is mechanical — every fact it needs is still reported, three of them
now inside the `sandbox` block — and was verified to build, vet and pass that
package's tests against this branch through the same go.work:

```go
var sb struct {
    Image     string          `json:"image"`
    Resources json.RawMessage `json:"resources"`
    Command   struct {
        Default []string `json:"default"`
    } `json:"command"`
}
if len(info.Sandbox) > 0 {
    if err := json.Unmarshal(info.Sandbox, &sb); err != nil {
        return KitInfo{}, fmt.Errorf("kit %q: decoding sandbox block: %w", ref, err)
    }
}
return KitInfo{
    Kind:          info.Kind,
    HasResources:  hasResources(sb.Resources),
    HasRunOptions: len(sb.Command.Default) > 0,
    HasTemplate:   sb.Image != "",
    HasVolumes:    hasVolumes(info.Volumes),
}, nil
```

That patch lives in the downstream repo and is **not** applied there yet. Land
it before or alongside the release that carries this `kit.Info`.

**The check also caught a bug in this repo**, which is the stronger argument for
running it. Reading real downstream code is what surfaced that `Info` had no
field for a v1 kit's `publishedPorts` — the CLI renames them to `ports` on
output, so they were being decoded into nothing and silently dropped. No test
here covered a v1 kit with ports, because the fixture is a v2 mixin. Probe a
kind `sandbox` kit and a v1 kit next sync, not just the fixture.
