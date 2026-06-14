package exec

import (
	"context"

	"github.com/squall-chua/sbx-go-sdk/sandbox"
)

// Logs streams a file inside the sandbox continuously, like `tail -f`. It runs
// `tail -F <path>` under an interactive attach and returns the live AttachSession:
// read session.Stdout() for the stream and call session.Close when done. The -F
// flag follows across log rotation/truncation and waits for a not-yet-created
// file. Extra ProcessOption values (e.g. WithUser) are passed through.
//
// It streams new lines tail-style (last few lines, then follow). To replay a whole
// file or use different flags, call ExecInteractive directly with your own command.
func Logs(ctx context.Context, sb *sandbox.Sandbox, path string, opts ...ProcessOption) (*AttachSession, error) {
	return ExecInteractive(ctx, sb, []string{"tail", "-F", path}, opts...)
}
