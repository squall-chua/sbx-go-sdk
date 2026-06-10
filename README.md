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

## Resources (Plan 2)

```go
// Interactive agent (terminal-inherit):
code, _ := sandbox.Run(ctx, c, sandbox.WithAgent("claude"), sandbox.WithWorkspace("."))

// Ports, files, templates:
sb.PublishPort(ctx, sandbox.Port{SandboxPort: 8080, HostIP: "127.0.0.1", Protocol: "tcp"})
ports, _ := sb.Ports(ctx)
sb.CopyTo(ctx, "./config.json", "/home/user/config.json")
sb.CopyFrom(ctx, "/home/user/out.log", "./out.log")
imgs, _ := template.List(ctx, c)
sb.SaveTemplate(ctx, "myimg:v1")

// Network policy + secrets:
policy.SetDefault(ctx, c, "balanced")
policy.Allow(ctx, c, "", "api.github.com")
log, _ := policy.Log(ctx, c)
secret.SetCustom(ctx, c, "", secret.CustomSecret{Host: "api.example.com", Env: "API_KEY", Value: "..."})
```

Verified deviations vs. the design spec: cp is shell-out (daemon `/files` GET is 501);
`policy`/`secret` list output is text. See the Plan 2 doc for details.
