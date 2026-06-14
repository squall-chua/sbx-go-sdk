// Command exec demonstrates the exec surface against one sandbox: capture, live
// streaming, detached-with-polling, a resource-usage snapshot, and following a
// log file.
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

	// 4. Stats: the same resource snapshot the sbx TUI shows (CPU, memory, disk,
	// uptime), read from /proc and df. CPUPercent is sampled over a short window,
	// so this call blocks briefly.
	u, err := exec.Stats(ctx, sb)
	if err != nil {
		log.Fatalf("stats: %v", err)
	}
	fmt.Printf("--- stats ---\ncpu %.1f%% across %d cores, mem %d/%d MiB, disk %.0f/%.0f GiB, up %.0fs\n",
		u.CPUPercent, u.Cores, u.MemUsedKB/1024, u.MemTotalKB/1024,
		u.DiskUsedGB, u.DiskTotalGB, u.UptimeSeconds)

	// 5. Follow a log file (tail -F) like `docker logs -f`. The stream never ends
	// on its own, so we read for a brief window and then Close to stop it.
	if _, _, err := exec.Exec(ctx, sb,
		[]string{"sh", "-c", "printf 'one\\ntwo\\n' > /tmp/demo.log"}); err != nil {
		log.Fatalf("seed log: %v", err)
	}
	fmt.Println("--- following /tmp/demo.log ---")
	logs, err := exec.Logs(ctx, sb, "/tmp/demo.log")
	if err != nil {
		log.Fatalf("logs: %v", err)
	}
	go func() { time.Sleep(500 * time.Millisecond); logs.Close() }()
	io.Copy(os.Stdout, logs.Stdout())
}
