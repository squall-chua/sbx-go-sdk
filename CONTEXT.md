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

**Daemon (`sandboxd`)**:
The local background process the SDK talks to over a unix socket. Same binary as the `sbx`
CLI. Owns all sandboxes.
_Avoid_: server, engine, service
