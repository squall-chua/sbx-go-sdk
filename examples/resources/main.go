// Command resources demonstrates the Plan-2 surface: publishing ports, copying
// files in and out, saving a template, and reading network policy.
//
//	go run ./examples/resources
package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"

	"github.com/squall-chua/sbx-go-sdk/client"
	"github.com/squall-chua/sbx-go-sdk/policy"
	"github.com/squall-chua/sbx-go-sdk/sandbox"
	"github.com/squall-chua/sbx-go-sdk/template"
)

func main() {
	ctx := context.Background()

	c, err := client.New(ctx, client.WithAutoStart())
	if err != nil {
		log.Fatalf("connect: %v", err)
	}

	sb, err := sandbox.Create(ctx, c,
		sandbox.WithAgent("shell"),
		sandbox.WithWorkspace("."),
	)
	if err != nil {
		log.Fatalf("create: %v", err)
	}
	defer sb.Remove(ctx)
	if err := sb.Start(ctx); err != nil {
		log.Fatalf("start: %v", err)
	}

	// Ports: publish a mapping (ephemeral host port), then list.
	if _, err := sb.PublishPort(ctx, sandbox.Port{
		SandboxPort: 8080,
		HostIP:      "127.0.0.1",
		Protocol:    "tcp",
	}); err != nil {
		log.Printf("publish-port: %v", err)
	}
	ports, _ := sb.Ports(ctx)
	fmt.Printf("published ports: %+v\n", ports)

	// Files: copy a host file in, then read it back out.
	tmp, _ := os.MkdirTemp("", "sbx-demo")
	defer os.RemoveAll(tmp)
	src := filepath.Join(tmp, "config.json")
	_ = os.WriteFile(src, []byte(`{"hello":"world"}`), 0o644)

	if err := sb.CopyTo(ctx, src, "/tmp/config.json"); err != nil {
		log.Printf("copy-to: %v", err)
	}
	if err := sb.CopyFrom(ctx, "/tmp/config.json", filepath.Join(tmp, "roundtrip.json")); err != nil {
		log.Printf("copy-from: %v", err)
	}

	// Templates: the daemon refuses to snapshot a running sandbox, so Stop first.
	if err := sb.Stop(ctx); err != nil {
		log.Printf("stop: %v", err)
	}
	if err := sb.SaveTemplate(ctx, "sbx-demo:v1"); err != nil {
		log.Printf("save-template: %v", err)
	} else {
		defer template.Remove(ctx, c, "sbx-demo:v1")
	}
	imgs, _ := template.List(ctx, c)
	fmt.Printf("template images: %d\n", len(imgs))

	// Policy: read the proxy's allowed/blocked-host log.
	if plog, err := policy.Log(ctx, c); err == nil {
		fmt.Printf("policy log: %d allowed, %d blocked\n",
			len(plog.AllowedHosts), len(plog.BlockedHosts))
	}
}
