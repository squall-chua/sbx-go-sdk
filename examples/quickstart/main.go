// Command quickstart connects to sandboxd, provisions a disposable sandbox over
// the current directory, runs one command inside it, and cleans up.
//
//	go run ./examples/quickstart
package main

import (
	"context"
	"fmt"
	"io"
	"log"

	"github.com/squall-chua/sbx-go-sdk/client"
	"github.com/squall-chua/sbx-go-sdk/exec"
	"github.com/squall-chua/sbx-go-sdk/sandbox"
)

func main() {
	ctx := context.Background()

	// Connect; start sandboxd if it isn't already running.
	c, err := client.New(ctx, client.WithAutoStart())
	if err != nil {
		log.Fatalf("connect: %v", err)
	}

	// Provision a sandbox for the "shell" agent over the working directory.
	sb, err := sandbox.Create(ctx, c,
		sandbox.WithAgent("shell"),
		sandbox.WithWorkspace("."),
	)
	if err != nil {
		log.Fatalf("create: %v", err)
	}
	defer sb.Remove(ctx) // disposable: tear it down on exit
	fmt.Printf("created sandbox %q (state=%s)\n", sb.Name(), sb.State())

	// Run a command. WithAutoStart brings the VM up if Create left it stopped.
	code, out, err := exec.Exec(ctx, sb,
		[]string{"sh", "-c", "echo hello from $(hostname); uname -a"},
		exec.WithAutoStart(),
	)
	if err != nil {
		log.Fatalf("exec: %v", err)
	}
	body, _ := io.ReadAll(out)
	fmt.Printf("[exit %d]\n%s", code, body)
}
