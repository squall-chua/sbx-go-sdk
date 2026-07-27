# sbx-go-sdk

A Go SDK for automating Docker Sandboxes (`sbx`) — isolated environments for AI coding
agents — by talking to the local `sandboxd` daemon. This glossary fixes the domain language
so SDK names match `sbx`'s own mental model.

## Language

**Sandbox**:
An isolated environment (a micro-VM) provisioned for an agent, with one or more host
workspaces mounted. The central resource the SDK manages.
_Avoid_: container, VM, box

**Agent**:
The AI coding tool that runs inside a sandbox (claude, codex, copilot, cursor,
docker-agent, droid, gemini, kiro, opencode, or shell). A sandbox is created *for* an agent.
_Avoid_: assistant, bot, tool

**Workspace**:
A host directory mounted into a sandbox. May be read-only (`:ro`). A sandbox can have
several; the first is primary.
_Avoid_: mount, volume, folder

**Create**:
Provision a sandbox for an agent *without* attaching to it. Matches `sbx create`.
_Avoid_: run (see below), provision, new

**Run**:
Launch and *interactively attach* to the agent in a sandbox, creating the sandbox first if
needed. Matches `sbx run`. In this SDK, **Run does NOT mean "create + start"** — that
docker/go-sdk meaning is deliberately rejected here.
_Avoid_: start, exec, attach (Run is specifically the agent session)

**Exec**:
Run an arbitrary command inside a sandbox (not the agent). Matches `sbx exec`.
_Avoid_: run, command, shell

**Start / Stop**:
Bring a sandbox's micro-VM up or down without removing it. Distinct from Run (which is about
the agent, not the VM lifecycle). Matches `sbx daemon`-managed sandbox states
(`running` / `stopped`).
_Avoid_: pause, resume, suspend (for VM up/down use Start/Stop only)

**Attach Session**:
A live bidirectional stream to a process in a sandbox (the agent via Run, or a command via
interactive Exec): stdin/stdout/stderr plus TTY resize. Backed by a hijacked connection.
_Avoid_: connection, stream, pipe

**Template**:
A saved sandbox image that new sandboxes can be created from. Matches `sbx template`.
_Avoid_: image, snapshot, base

**Kit**:
A declarative YAML artifact (`spec.yaml` plus an optional `files/` directory) that
contributes configuration to a sandbox. Two kinds: a **mixin** kit adds credentials, network
policy, environment variables, startup commands and files to an existing sandbox; a
**sandbox** kit instead supplies the base image the sandbox is built from. Installed from a
directory, ZIP, git repository, or OCI reference. EXPERIMENTAL upstream.
_Avoid_: plugin, extension, bundle, addon

**Skills Store**:
The daemon-managed directory of agent skill folders, seeded from the host by `sbx skills
import` and mounted into sandboxes that have not opted out. Survives sandbox deletion;
cleared by `sbx reset`. EXPERIMENTAL upstream.
_Avoid_: skill cache, shared skills, skill registry

**Daemon (`sandboxd`)**:
The local background process the SDK talks to over a unix socket. Same binary as the `sbx`
CLI. Owns all sandboxes.
_Avoid_: server, engine, service

**Scope**:
Whether a policy or secret is global or bound to a single sandbox. The SDK uses `""` for
global; `sbx` spells the same idea `(global)` / `-g` / `--sandbox NAME` depending on the
subcommand.
_Avoid_: namespace, level, context

**Secret**:
A stored credential the daemon injects into sandboxes, listed (masked) by `sbx secret ls`.
Two kinds: *service/registry* secrets (`sbx secret set`) and *custom* secrets (below).
_Avoid_: credential, key, token

**Custom Secret**:
A proxy-injected secret that swaps a placeholder env var for the real value on outbound
requests to a target host (`sbx secret set-custom`). EXPERIMENTAL upstream.
_Avoid_: injected secret, env secret

**Policy Rule**:
A single allow/deny entry shown by `sbx policy ls` — a source, scope, decision, resource
type, and the resources it covers.
_Avoid_: rule, ACL, firewall rule

**Source**:
Where a policy rule comes from: `local` (set on this host), `org` (remote governance), or
`kit` (contributed by an installed kit). Shown as the `SOURCE` column of `sbx policy ls`.
_Avoid_: provenance, origin, owner

**Authorization**:
The outcome of evaluating the whole policy against one access request (`sbx policy check`) —
allowed or denied, with the deciding rule and, for a denial, whether a deny rule matched or
nothing matched at all (an *implicit* deny). Distinct from a Policy Rule's own `decision`.
_Avoid_: decision, verdict, check result

**Setting**:
A persistent daemon configuration key managed by `sbx settings` (e.g. `feature.ssh`,
`ssh.autoCreate`, `kit.allowedSources`). Each has a typed value and a source (`default` vs an
override). The daemon hot-reloads changes from `settings.json`.
_Avoid_: config, option, preference, flag

**SSH Endpoint**:
The native SSH access to sandboxes exposed by the daemon (gated by the `feature.ssh`
setting). Sandbox name is the SSH username; provisioned for a normal `ssh` client via
`sbx ssh setup`. EXPERIMENTAL upstream.
_Avoid_: shell, remote, tunnel
