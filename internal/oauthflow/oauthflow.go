// Package oauthflow runs the sbx subcommands that perform a browser OAuth
// handshake, without requiring a browser or a terminal.
//
// Both such commands — `sbx mcp auth NAME` and `sbx secret set SERVICE --oauth`
// — behave the same way with no TTY: they print an authorization URL, then block
// on a loopback callback listener until the user completes consent. `mcp auth`
// prints the URL on stdout, `secret set --oauth` on stderr, so both streams are
// scanned.
//
// That shape is what makes them wrappable at all: the SDK hands the URL to the
// caller, who decides how to present it, and the call blocks until consent lands
// or the context ends.
package oauthflow

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"

	"github.com/squall-chua/sbx-go-sdk/internal/cli"
)

// Run executes `sbx args...`, calls onURL once with the first https:// URL the
// child prints on either stream, and blocks until the child exits.
//
// onURL runs on an internal goroutine while the child is still running — it must
// not block for long, and must be safe to call from another goroutine. Cancel
// ctx to abandon a flow the user never completes; the child is interrupted and
// the error names the cancellation.
//
// A non-zero exit becomes an error carrying the child's output, so a flow that
// the user declined or that timed out server-side is reported rather than
// silently succeeding.
//
// Scanning the output means the child's stdout and stderr are pipes rather than
// the inherited *os.File descriptors cli.Runner.Inherit normally passes through.
// A grandchild that inherits a pipe and outlives its parent therefore holds the
// copy open, and Run does not return until Runner's kill backstop fires (10s).
// That is the cost of reading the URL out; the sbx commands wrapped here leave
// no such grandchild.
func Run(ctx context.Context, r *cli.Runner, onURL func(string), args ...string) error {
	if onURL == nil {
		return errors.New("oauthflow: onURL must not be nil; it is the only way to reach the authorization URL")
	}

	var mu sync.Mutex
	var combined bytes.Buffer
	w := &urlScanner{onURL: onURL, mu: &mu, sink: &combined}

	code, err := r.Inherit(ctx, cli.Stdio{In: emptyReader{}, Out: w, Err: w}, nil, args...)
	w.flush()
	if err != nil {
		return err
	}
	if code != 0 {
		mu.Lock()
		out := strings.TrimSpace(combined.String())
		mu.Unlock()
		if ctxErr := ctx.Err(); ctxErr != nil {
			return fmt.Errorf("oauth flow abandoned: %w", ctxErr)
		}
		return fmt.Errorf("oauth flow failed (exit %d): %s", code, out)
	}
	return nil
}

// emptyReader stands in for stdin so the child never inherits the caller's
// terminal and cannot block reading from it.
type emptyReader struct{}

func (emptyReader) Read([]byte) (int, error) { return 0, io.EOF }

// urlScanner tees the child's output into sink and fires onURL for the first
// https:// URL it sees, line by line.
type urlScanner struct {
	onURL func(string)
	mu    *sync.Mutex
	sink  *bytes.Buffer

	pending []byte
	fired   bool
}

func (u *urlScanner) Write(p []byte) (int, error) {
	u.mu.Lock()
	u.sink.Write(p)
	u.mu.Unlock()

	u.pending = append(u.pending, p...)
	for {
		i := bytes.IndexByte(u.pending, '\n')
		if i < 0 {
			return len(p), nil
		}
		u.consider(string(u.pending[:i]))
		u.pending = u.pending[i+1:]
	}
}

// flush considers a trailing line the child left without a newline.
func (u *urlScanner) flush() { u.consider(string(u.pending)); u.pending = nil }

func (u *urlScanner) consider(line string) {
	if u.fired {
		return
	}
	i := strings.Index(line, "https://")
	if i < 0 {
		return
	}
	url := strings.TrimSpace(line[i:])
	if url == "" {
		return
	}
	u.fired = true
	u.onURL(url)
}
