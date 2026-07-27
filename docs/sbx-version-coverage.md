# sbx feature coverage by release

What each `sbx` release added, and whether this SDK covers it. The SDK debuted
against v0.32.0 (daemon API `0.10.0`).

**Maintainers:** reconcile this table against the upstream release notes on every
version sync. `TestContract_VersionAlignment` points here when it detects drift.
The v0.35.0 sync shipped while silently missing five v0.35.0 features — this
table exists so that cannot happen quietly again.

Status values: **covered** · **gap** (upstream feature the SDK does not expose)
· **deferred** (planned, with a named spec) · **n/a** (needs no SDK surface).

| Release | Daemon API |
|---|---|
| v0.32.0 | `0.10.0` (SDK debut) |
| v0.33.0 | `0.12.0` |
| v0.34.0 | `0.16.0` |
| v0.35.0 | `0.22.0` |
| v0.36.0 | never released |
| v0.37.0 | `0.24.0` |

| Feature | sbx | SDK | Status |
|---|---|---|---|
| Sandbox create / run / exec / cp / ports / templates | v0.32.0 | `sandbox`, `exec`, `template` | covered |
| Policy allow / deny / rm / reset / log | v0.32.0 | `policy` | covered |
| `sbx diagnose` | v0.32.0 | — | gap — never wrapped |
| `sbx login` / `sbx logout` | v0.32.0 | — | gap — never wrapped |
| Stable per-sandbox `id` | v0.33.0 | `Sandbox.ID()` | covered |
| `mount_policy_denied` | v0.33.0 | `Sandbox.MountPolicyDenied()` | covered |
| `sbx cp -L` follows source symlinks | v0.33.0 | `WithFollowSymlinks` | covered |
| `secret set-custom --host` wildcards | v0.33.0 | `secret.SetCustom` | covered — pattern passes through |
| Experimental SSH endpoint | v0.34.0 | `ssh` | covered |
| `sbx setup` (credential import) | v0.34.0 | `secret.Import` / `ImportAll` | covered (v0.37.0 sync) — `--force` stays opt-in via `WithOverwriteExisting`, unlike `skillstore.Import` which always forces; skill replacement is recoverable (the CLI backs up the folder first), a credential overwrite is not |
| `policy set-default` renamed `policy init` | v0.34.0 | `policy.SetDefault` | covered — calls `policy init` |
| Kit source allowlist (`kit.allowedSources`) | v0.34.0 | `settings.Set` | covered — generic settings |
| OCI v2 kit artifact streaming | v0.34.0 | — | deferred — kit spec |
| Published ports restored on restart | v0.34.0 | `sandbox.Ports` | n/a — daemon-side |
| `sbx policy check network` | v0.35.0 | `policy.Check` | covered (v0.37.0 sync) — `sbx policy check --help` lists only the `network` subcommand, so there is no sibling to add a row for |
| `sbx policy inspect` | v0.35.0 | `policy.InspectRaw` | covered (v0.37.0 sync) |
| `policy ls --wide/--source/--decision/--include-inactive` | v0.35.0 | — | gap — fields are on `PolicyRule`; filter client-side |
| `sbx secret import` | v0.35.0 | `secret.Import` / `ImportAll` | covered (v0.37.0 sync) |
| `sbx rm --force` for an active session | v0.35.0 | `Remove(WithForce())` | covered (v0.37.0 sync) |
| `sbx inspect` (kits, auth mode, active sessions) | v0.35.0 | — | gap — not in `api.SandboxInfo` |
| `sbx kit add` recreates container, applies kit policy | v0.35.0 | — | deferred — kit spec |
| `GET /health` removed; `/daemon/health` is liveness | v0.35.0 | `Client.Health` | covered |
| `credential_sources` on `SandboxInfo` | v0.35.0 | `api.SandboxInfo` | covered |
| SOCKS5 upstream proxy, `DOCKER_SANDBOXES_PROXY` | v0.35.0 | — | n/a — env var |
| `DOCKER_SANDBOXES_NO_PROXY` | v0.35.0 | — | n/a — env var |
| virtiofs cache default on, `..._ENABLE_VIRTIOFS_CACHE` | v0.35.0 | — | n/a — env var |
| Shared agent skills store, `sbx skills import` | v0.37.0 | `skillstore.Import` | covered |
| `--no-share-skills` on create | v0.37.0 | — | gap — flag is gated off by a remote feature flag |
| `-p/--publish` on create and run | v0.37.0 | `sandbox.WithPublish` | covered |
| `sbx setup ssh` replaces `sbx ssh setup` | v0.37.0 | `ssh.Setup` | covered |
| `feature.ssh` defaults to enabled | v0.37.0 | `ssh.Enabled` | covered |
| `POST /version` removed | v0.37.0 | `Client.CheckVersion` | covered — deprecated, see ADR 0004 |
| `GET /sandbox/{n}/files` now works | v0.37.0 | `Sandbox.CopyFrom` | covered — REST, see ADR 0003 |
| `POST /sandbox/{n}/ports/unpublish` | v0.37.0 | `Sandbox.UnpublishPort` | covered — REST, see ADR 0003 |
| `GET /policy/network/rules` now works | v0.37.0 | `policy.List` | covered — REST, `type=all` |
| `GET /policy/network/profiles` | v0.37.0 | `policy.Profiles` | gap — raw text; no data to model, see ADR 0002 |
| `POST /daemon/reset` | v0.37.0 | `Client.Reset` | covered |
| `DOCKER_SANDBOXES_PROXY=system` | v0.37.0 | — | n/a — env var |
| Governance org support messages | v0.37.0 | `policy.Check` → `Authorization.Governance` | covered — fields decoded (`Active`, `Organization`, `OrganizationUnavailable`, `LastSyncedStatus`, `LastSyncedMessage`); live behaviour unverified, no governed org on this host |
| `sbx secret set --oauth` | v0.37.0 | — | gap — interactive, `openai`/global only |

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
