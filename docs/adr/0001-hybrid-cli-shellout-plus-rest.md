# Hybrid transport: shell out to `sbx` for provisioning, REST for runtime ops

The `sandboxd` daemon's REST API has no client path for sandbox **creation** (and none for
`template save`): the `sbx` CLI orchestrates these client-side (`runCreate` /
`createWithCleanup`), and the unused `POST /sandbox` handler answers `"not implemented"`.
Reverse-engineering exactly what the SDK calls (`go tool nm` over the binary) confirms the REST
client only ships builders for list/inspect/start/stop/delete, exec/inspect-exec/resize,
images, ports, sync-credentials, and daemon ops — not create.

**Decision:** the SDK is a hybrid. It **shells out to the `sbx` binary** for the
orchestration-heavy operations (`Create`, agent `Run`, `template save`) and uses the **daemon
REST API** directly for everything with a real client path (list/inspect/start/stop/remove,
exec, ports, images, cp/attach via their hijack endpoints, secrets, daemon lifecycle).

## Considered Options

- **Reimplement orchestration** over the engine Connect-RPC/`docker.sock` layer — rejected:
  large, fragile, and re-tracks a moving internal target (effectively rebuilding the CLI).
- **Depend on the REST `POST /sandbox` handler** — rejected: it is unused by the CLI,
  apparently incomplete, and returned `"not implemented"`.
- **Shell out for everything** — rejected: loses typed structs, streaming control, and
  structured errors for the runtime operations that *do* have a clean REST path.

## Consequences

- The SDK requires the `sbx` binary on the host for `Create`/`Run`/`template save` (it already
  requires it for daemon start). Binary discovery: PATH, with a `WithBinaryPath` override.
- Two internal drivers: `internal/api` (REST) and an `sbx`-binary driver. Resource methods route
  to whichever fits.
- Version coupling: shell-out flags and REST structs are pinned to a tested `sbx`/daemon range
  (v0.32.0 / api 0.10.0); a contract test warns on drift. See the design spec's open questions.
