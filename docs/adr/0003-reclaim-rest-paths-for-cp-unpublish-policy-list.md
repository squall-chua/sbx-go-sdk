# Reclaim REST paths for cp-from, port unpublish, and policy list

Three operations the SDK shelled out to the `sbx` binary for gained working daemon REST paths by
v0.37.0: `GET /sandbox/{name}/files` (404 through v0.35.0), `POST /sandbox/{name}/ports/unpublish`,
and `GET /policy/network/rules` (`501 not implemented` at v0.32.0, when ADR 0001 was written).

**Decision:** move all three to REST, keeping their **public signatures unchanged** so downstream
consumers keep compiling. This fulfils ADR 0001's original intent for `cp` — that ADR already
listed cp as a REST operation; the endpoint simply did not work yet.

## Consequences

- **`policy.List` must always send `type=all`.** The endpoint defaults to network-only, so
  omitting the parameter silently drops filesystem rules — no error, no drift signal, just a
  shorter list. Verified live: `type=all` returns 14 rules (12 network + `filesystem:read` +
  `filesystem:write`), no parameter returns 12. The parameter is load-bearing, not decorative.
  This is precisely the silent-partial-data failure ADR 0002 chose strict parsing to avoid.

- **The tar extractor must validate symlink targets itself.** `os.Root` confines writes and blocks
  traversal *through* a symlink that escapes the root, but still permits *creating* one. Sandbox
  contents are agent-authored, so this is a trust boundary: an agent that writes
  `ln -sf ../../../../../../etc/hostname` had that link faithfully reproduced on the host by an
  `os.Root`-based extractor, exit code 0. `sbx cp` rejects a relative escaping target (exit 1) and
  permits an absolute one such as `-> /etc/passwd` (exit 0); the extractor matches both, including
  that asymmetry. Do not remove the check on the assumption that `os.Root` covers it.

- **`CopyFrom` auto-starts a stopped sandbox**, unlike `exec`, which returns
  `client.ErrSandboxNotRunning` unless given `WithAutoStart()`. `sbx cp` has always started the VM
  ("Sandbox … started successfully", exit 0) while `GET /files` answers `409 sandbox is not
  running`, so parity requires it. The asymmetry between two neighbouring methods on the same
  handle is deliberate: `exec` runs arbitrary code and deserves an explicit opt-in; copying a file
  is passive.

- Behaviour was confirmed by a differential test against `sbx cp` across 15 cases — files,
  directories, mode bits (0755 and 0640), file and directory symlinks with and without `-L`, a
  `:ro` workspace, a missing path, and hostile symlinks — comparing path, type, mode, size, link
  target, ownership, and contents. Tar ownership (`root/root`) is ignored; extracted files are
  owned by the calling user, matching `sbx cp`. One intentional divergence: a failed extraction
  into a non-existent destination leaves nothing behind (staging directory discarded), where
  `sbx cp` leaves a partial tree.

## Considered Options

- **Keep shelling out** — rejected: the endpoints now work, and each shell-out costs a subprocess
  plus loses structured errors. `policy.List`'s CLI path also depends on `--json` output shape.
- **Change `UnpublishPort` to a typed `Port` signature** — rejected: it reads better, but it breaks
  the downstream build (`sbx-swarm-node` builds a spec string at `sdkbackend.go:292`) to save the
  SDK a ~25-line spec parser. Source compatibility is a constraint here, not a preference; every
  break must buy the *caller* something.
- **Migrate `CopyTo` as well** — rejected as out of scope: `PUT /files` exists, but the shell-out
  works and rewriting it buys nothing.
