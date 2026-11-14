package kit

import (
	"context"
	"errors"

	"github.com/squall-chua/sbx-go-sdk/client"
)

// The kit builder is a shared sandbox named sbx-kit-builder that builds kit
// spec v3 source kits (a local directory or git reference). Its Docker Engine
// store is the kit build cache. It is an ordinary sandbox: while it exists it
// shows in `sbx ls` and in sandbox.List.

// BuilderStatusRaw returns `sbx kit builder status`: whether the builder
// exists and runs, its kit image and the kit registry's reachability. The
// CLI offers no JSON form, so the text comes back as printed. It does not
// create the builder.
func BuilderStatusRaw(ctx context.Context, c *client.Client) (string, error) {
	r, err := c.Runner()
	if err != nil {
		return "", err
	}
	return r.Capture(ctx, nil, "kit", "builder", "status")
}

// BuilderHistoryRaw runs `sbx kit builder history ARGS...` and returns its
// output. The CLI passes ARGS verbatim to `docker buildx history` inside the
// builder, so the subcommands are buildx's: ls, inspect, logs, rm, trace and
// export, each with buildx's own flags (for example "ls", "--format",
// "json"). args must name a subcommand.
//
// Every call creates and starts the builder sandbox if it does not exist,
// even `ls`. Remove it with RemoveBuilder.
//
// At sbx v0.51.0 `ls` listed no records even straight after a successful v3
// kit build in the builder; why is unexplained.
func BuilderHistoryRaw(ctx context.Context, c *client.Client, args ...string) (string, error) {
	if len(args) == 0 {
		return "", errors.New("kit: BuilderHistoryRaw needs a subcommand, such as \"ls\"")
	}
	r, err := c.Runner()
	if err != nil {
		return "", err
	}
	return r.Capture(ctx, nil, append([]string{"kit", "builder", "history"}, args...)...)
}

// RemoveBuilder removes the builder sandbox and with it the kit build cache
// (`sbx kit builder rm --force`). The next source-form kit build recreates
// it.
//
// --force is always sent, as without it the CLI refuses a non-interactive
// stdin. It also removes a builder that is in use, cutting off any build
// running in it. It succeeds when there is no builder, though the CLI still
// prints "Removed sandbox sbx-kit-builder".
func RemoveBuilder(ctx context.Context, c *client.Client) error {
	r, err := c.Runner()
	if err != nil {
		return err
	}
	_, err = r.Capture(ctx, nil, "kit", "builder", "rm", "--force")
	return err
}
