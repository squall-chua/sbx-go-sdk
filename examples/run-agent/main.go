// Command run-agent launches an interactive agent session, attaching the agent
// to this terminal and blocking until it exits. This is the SDK's `Run` —
// the agent session — not "create + start".
//
//	go run ./examples/run-agent [agent]
//
// agent defaults to "shell". A non-zero agent exit is reported, not fatal.
package main

import (
	"context"
	"fmt"
	"log"
	"os"

	"github.com/squall-chua/sbx-go-sdk/client"
	"github.com/squall-chua/sbx-go-sdk/sandbox"
)

func main() {
	ctx := context.Background()

	agent := "shell"
	if len(os.Args) > 1 {
		agent = os.Args[1]
	}

	c, err := client.New(ctx, client.WithAutoStart())
	if err != nil {
		log.Fatalf("connect: %v", err)
	}

	// Run provisions-if-missing, then attaches the agent to os.Stdin/out/err and
	// blocks. It returns the agent's exit code; only spawn/wait failures error.
	code, err := sandbox.Run(ctx, c,
		sandbox.WithAgent(agent),
		sandbox.WithWorkspace("."),
	)
	if err != nil {
		log.Fatalf("run: %v", err)
	}
	fmt.Printf("\nagent exited with code %d\n", code)
}
