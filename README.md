# sbx-go-sdk

A Go SDK to automate Docker Sandboxes (`sbx`). See
[docs/superpowers/specs/2026-06-10-sbx-go-sdk-design.md](docs/superpowers/specs/2026-06-10-sbx-go-sdk-design.md).

```go
ctx := context.Background()
c, _ := client.New(ctx, client.WithAutoStart())
sb, _ := sandbox.Create(ctx, c, sandbox.WithAgent("claude"), sandbox.WithWorkspace("."))
defer sb.Remove(ctx)
code, out, _ := exec.Exec(ctx, sb, []string{"claude", "-p", "summarise the repo"})
```

Plan 1 covers: daemon lifecycle, sandbox create/list/inspect/start/stop/remove, exec
(capture/interactive/detached). Plan 2 adds cp/ports/template/policy/secret.
