package client_test

import (
	"context"
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
	c, err := client.New(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	var r *client.Runner
	if r, err = c.Runner(); err != nil {
		t.Fatal(err)
	}
	if r.Bin() == "" {
		t.Fatal("runner Bin() is empty")
	}
}
