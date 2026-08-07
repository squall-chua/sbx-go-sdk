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
| Kit spec v2 (`permissions`, `setup`, `agentInstructions`) | v0.38.0 | `kit` package | n/a — authoring-side schema. `schemaVersion: "2"` now has its own decoder, so a v2 spec must use the new key names, but `kit inspect --json` still reports the normalized v1 shape and `kit.Info` is unchanged. The integration fixture was migrated |
| `sbx cp` copy-out destination escape (CVE-2026-17106) | v0.38.0 | `internal/untar` | n/a — the SDK's `Sandbox.CopyFrom` extracts the REST tar itself through `os.Root` with explicit symlink and hardlink target checks, so it never had the CLI's escape. Upgrading sbx fixes the CLI path |
| `sbx inspect` shows custom secrets | v0.38.0 | `Summary.Secrets` | covered — a sandbox-scoped custom secret lists as `{name, source:"custom"}`, verified live |
| Structured create/run progress output | v0.38.0 | — | n/a — `sandbox.Create` owns the name and never parses create output |
| `sbx diagnose -o json` / `--upload` | v0.38.0 | `Client.Diagnose` | partly covered — `-o json` is what `Diagnose` parses; `--upload` is deliberately not wrapped. Distinct from `Client.Diagnostics`, which is the daemon's own `/daemon/diagnostics` report |

## Create-request fields the daemon accepts but the CLI cannot pass

`SandboxCreateRequest` carries `Environment`, `SecretsScope`, `PullPolicy`,
`RootFilesystemSize`, `DindVolumeSize`, `EnableVirtiofsCache`, `Display`,
`AgentOptions`, `BindingsPath` and `CredentialValues`. Sandbox creation shells
out to `sbx create`, which exposes none of them, so they are unreachable until
creation moves to REST. Recorded here so the list is not rediscovered each sync.

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
