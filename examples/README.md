# Examples

Runnable, self-contained programs for the [sbx-go-sdk](../README.md). Each needs a reachable
`sandboxd` (the programs pass `client.WithAutoStart()`, so the SDK starts it if the `sbx`
binary is on `PATH`) and a current directory that can be mounted as a workspace.

| Program | Demonstrates |
| --- | --- |
| [`quickstart`](quickstart) | Connect → create → exec → remove — the smallest end-to-end flow. |
| [`exec`](exec) | All three exec modes: capture, live streaming (`WithMultiplexed`), detached + poll. |
| [`run-agent`](run-agent) | Interactive agent session attached to your terminal (`sandbox.Run`). |
| [`resources`](resources) | Ports, file copy in/out, template save, network-policy log. |

```bash
go run ./examples/quickstart
go run ./examples/exec
go run ./examples/run-agent shell    # or: claude, codex, gemini, …
go run ./examples/resources
```

Each program creates a disposable sandbox and removes it on exit (`defer sb.Remove(ctx)`), so
they leave no sandboxes behind. They do not call `client.Reset`, which would wipe all daemon
state.
