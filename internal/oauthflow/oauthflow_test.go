package oauthflow

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/squall-chua/sbx-go-sdk/internal/cli"
	"github.com/stretchr/testify/require"
)

func runnerPrinting(t *testing.T, script string) *cli.Runner {
	t.Helper()
	bin := filepath.Join(t.TempDir(), "sbx")
	require.NoError(t, os.WriteFile(bin, []byte("#!/bin/sh\n"+script), 0o755))
	r, err := cli.NewRunner(bin)
	require.NoError(t, err)
	return r
}

// `sbx mcp auth NAME` prints its URL on stdout. Captured verbatim at v0.38.0.
func TestRun_CapturesURLFromStdout(t *testing.T) {
	const url = "https://mcp.notion.com/authorize?client_id=4NTTVH76e_wyHLW7&scope=default"
	r := runnerPrinting(t, `echo 'Open this URL to authorize MCP server "x":'
echo '`+url+`'
exit 0
`)

	var got []string
	var mu sync.Mutex
	err := Run(context.Background(), r, func(u string) {
		mu.Lock()
		defer mu.Unlock()
		got = append(got, u)
	}, "mcp", "auth", "x")

	require.NoError(t, err)
	require.Equal(t, []string{url}, got)
}

// `sbx secret set SERVICE --oauth` prints its URL on stderr instead.
func TestRun_CapturesURLFromStderr(t *testing.T) {
	const url = "https://auth.openai.com/oauth/authorize?client_id=app_EMoamEEZ73f0CkXaXp7hrann"
	r := runnerPrinting(t, `echo 'Open this URL to sign in to Codex OAuth:' >&2
echo '`+url+`' >&2
exit 0
`)

	var got string
	err := Run(context.Background(), r, func(u string) { got = u }, "secret", "set", "openai", "--oauth")

	require.NoError(t, err)
	require.Equal(t, url, got)
}

// The prompt line and the URL line both reach the scanner; only the URL fires,
// and a second URL later in the stream must not fire again.
func TestRun_FiresOnceOnly(t *testing.T) {
	r := runnerPrinting(t, `echo 'Open this URL to authorize:'
echo 'https://first.example/authorize'
echo 'https://second.example/authorize'
exit 0
`)

	var n int
	var first string
	err := Run(context.Background(), r, func(u string) {
		n++
		if first == "" {
			first = u
		}
	}, "mcp", "auth", "x")

	require.NoError(t, err)
	require.Equal(t, 1, n, "onURL must fire exactly once")
	require.Equal(t, "https://first.example/authorize", first)
}

// A URL the child leaves without a trailing newline must still be seen.
func TestRun_CapturesURLWithoutTrailingNewline(t *testing.T) {
	r := runnerPrinting(t, "printf 'https://no-newline.example/authorize'\nexit 0\n")

	var got string
	err := Run(context.Background(), r, func(u string) { got = u }, "mcp", "auth", "x")

	require.NoError(t, err)
	require.Equal(t, "https://no-newline.example/authorize", got)
}

// A declined or server-side-failed flow exits non-zero; the child's output has
// to survive into the error rather than being swallowed.
func TestRun_NonZeroExitCarriesOutput(t *testing.T) {
	r := runnerPrinting(t, `echo 'ERROR: 1 MCP server(s) failed authorization' >&2
exit 1
`)

	err := Run(context.Background(), r, func(string) {}, "mcp", "auth", "x")

	require.Error(t, err)
	require.Contains(t, err.Error(), "exit 1")
	require.Contains(t, err.Error(), "failed authorization")
}

// The real command blocks on a loopback callback forever if nobody consents,
// so cancelling must end it and say so. The fake traps the interrupt the way
// sbx itself does — a child that ignores it instead hits Runner's 10s
// kill backstop, which is a different path.
func TestRun_CancellationIsReportedAsAbandoned(t *testing.T) {
	r := runnerPrinting(t, `trap 'exit 130' INT
echo 'https://slow.example/authorize'
i=0
while [ $i -lt 300 ]; do sleep 0.1; i=$((i+1)); done
`)

	ctx, cancel := context.WithCancel(context.Background())
	urlSeen := make(chan struct{})
	var once sync.Once

	done := make(chan error, 1)
	go func() {
		done <- Run(ctx, r, func(string) { once.Do(func() { close(urlSeen) }) }, "mcp", "auth", "x")
	}()

	select {
	case <-urlSeen:
	case <-time.After(10 * time.Second):
		cancel()
		t.Fatal("onURL never fired")
	}
	cancel()

	select {
	case err := <-done:
		require.Error(t, err)
		require.ErrorIs(t, err, context.Canceled)
		require.Contains(t, err.Error(), "abandoned")
	case <-time.After(15 * time.Second):
		t.Fatal("Run did not return after cancellation")
	}
}

// Without onURL the caller can never reach the URL, so the flow is pointless —
// reject it before spawning anything.
func TestRun_RejectsNilCallback(t *testing.T) {
	argFile := filepath.Join(t.TempDir(), "ran.txt")
	r := runnerPrinting(t, "touch "+argFile+"\n")

	err := Run(context.Background(), r, nil, "mcp", "auth", "x")

	require.Error(t, err)
	require.NoFileExists(t, argFile, "the CLI must not be invoked")
}

// The child must not inherit the caller's stdin: the real command would
// otherwise sit reading a terminal the caller may be using for something else.
func TestRun_ChildStdinIsEmpty(t *testing.T) {
	r := runnerPrinting(t, `if [ -n "$(cat)" ]; then exit 3; fi
echo 'https://ok.example/authorize'
exit 0
`)

	err := Run(context.Background(), r, func(string) {}, "mcp", "auth", "x")
	require.NoError(t, err)
}
