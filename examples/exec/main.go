// Command exec demonstrates the three exec modes against one sandbox:
// capture, live streaming, and detached-with-polling.
//
//	go run ./examples/exec
package main

import (
	"context"
	"fmt"
	"io"
	"log"
	"os"
	"time"

	"github.com/squall-chua/sbx-go-sdk/client"
	"github.com/squall-chua/sbx-go-sdk/exec"
	"github.com/squall-chua/sbx-go-sdk/sandbox"
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

	// 1. Capture: stdout comes back as an io.Reader; stderr is discarded.
	code, out, err := exec.Exec(ctx, sb,
		[]string{"sh", "-c", "echo $GREETING from $PWD"},
		exec.WithEnv(map[string]string{"GREETING": "hi"}),
		exec.WithWorkdir("/"),
	)
	if err != nil {
		log.Fatalf("capture: %v", err)
	}
	body, _ := io.ReadAll(out)
	fmt.Printf("capture [exit %d]: %s", code, body)

	// 2. Stream: route stdout and stderr live to our own writers.
	fmt.Println("--- streaming ---")
	if _, _, err := exec.Exec(ctx, sb,
		[]string{"sh", "-c", "for i in 1 2 3; do echo line $i; done; echo oops 1>&2"},
		exec.WithMultiplexed(os.Stdout, os.Stderr),
	); err != nil {
		log.Fatalf("stream: %v", err)
	}

	// 3. Detached: start in the background, then poll for completion.
	id, err := exec.ExecDetached(ctx, sb, []string{"sh", "-c", "sleep 1; exit 7"})
	if err != nil {
		log.Fatalf("detached: %v", err)
	}
	fmt.Printf("--- detached exec %s ---\n", id)
	for {
		st, err := exec.InspectExec(ctx, sb, id)
		if err != nil {
			log.Fatalf("inspect-exec: %v", err)
		}
		if !st.Running {
			fmt.Printf("detached finished: exit %d\n", st.ExitCode)
			break
		}
		time.Sleep(250 * time.Millisecond)
	}
}
