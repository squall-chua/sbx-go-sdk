package client_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/squall-chua/sbx-go-sdk/client"
)

// Compile-time proof that Client.Runner()'s return type is the externally-nameable
// client.Runner, closing the internal/cli.Runner leak.
var _ = func(c *client.Client) *client.Runner {
	r, _ := c.Runner()
	return r
}

func TestRunnerIsExternallyNameable(t *testing.T) {
	// Inject a fake binary via WithBinaryPath so the test is hermetic and does not
	// require a real sbx on PATH (matching the other shell-out unit tests).
	bin := filepath.Join(t.TempDir(), "sbx")
	if err := os.WriteFile(bin, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	c, err := client.New(context.Background(), client.WithBinaryPath(bin))
	if err != nil {
		t.Fatal(err)
	}
	var r *client.Runner
	if r, err = c.Runner(); err != nil {
		t.Fatal(err)
	}
	if r.Bin() != bin {
		t.Fatalf("runner Bin() = %q, want %q", r.Bin(), bin)
	}
}
