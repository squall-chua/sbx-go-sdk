package sandbox

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"

	"github.com/squall-chua/sbx-go-sdk/client"
	"github.com/squall-chua/sbx-go-sdk/internal/transport"
	"github.com/squall-chua/sbx-go-sdk/internal/untar"
)

// copyConfig accumulates cp options.
type copyConfig struct{ followSymlinks bool }

// CopyOption configures a copy.
type CopyOption func(*copyConfig)

// WithFollowSymlinks follows symlinks in the source path (`sbx cp -L`), in either
// direction (sandbox -> host support added in sbx v0.33.0).
func WithFollowSymlinks() CopyOption { return func(c *copyConfig) { c.followSymlinks = true } }

// CopyTo copies a host path into the sandbox (`sbx cp [-L] localPath name:sandboxPath`).
func (s *Sandbox) CopyTo(ctx context.Context, localPath, sandboxPath string, opts ...CopyOption) error {
	var cfg copyConfig
	for _, o := range opts {
		o(&cfg)
	}
	args := []string{"cp"}
	if cfg.followSymlinks {
		args = append(args, "-L")
	}
	args = append(args, localPath, s.info.Name+":"+sandboxPath)
	r, err := s.cli.Runner()
	if err != nil {
		return err
	}
	_, err = r.Capture(ctx, nil, args...)
	return err
}

// CopyFrom copies a sandbox path to the host over REST
// (GET /sandbox/{name}/files?path=…), extracting the returned tar stream.
//
// A stopped sandbox is started first. `sbx cp` has always done this, and the
// REST endpoint answers 409 when the sandbox is not running, so auto-starting
// is required for behavioural parity. This differs from exec, which requires
// an explicit WithAutoStart — see docs/adr/0003.
//
// Placement matches `sbx cp`: the tar is rooted at the requested path's base
// name, so when localPath does not exist the archive is staged beside it and
// renamed into place, and when localPath is an existing directory the source
// lands inside it.
//
// Missing ancestor directories of localPath are created and are not removed if
// the copy fails; only the destination itself is guaranteed absent on failure.
func (s *Sandbox) CopyFrom(ctx context.Context, sandboxPath, localPath string, opts ...CopyOption) error {
	var cfg copyConfig
	for _, o := range opts {
		o(&cfg)
	}
	if err := s.ensureRunningForCopy(ctx); err != nil {
		return err
	}

	q := url.Values{}
	q.Set("path", sandboxPath)
	if cfg.followSymlinks {
		q.Set("follow", "true")
	}
	route := "/sandbox/" + url.PathEscape(s.info.Name) + "/files?" + q.Encode()

	resp, err := s.cli.Transport().Do(ctx, http.MethodGet, route, nil, nil)
	if err != nil {
		return client.MapError("copy-from", err)
	}
	defer resp.Body.Close()

	// Transport.Do returns the raw *http.Response and does NOT error on non-2xx,
	// so the status must be checked here or a 404 body would be extracted as tar.
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		body, _ := io.ReadAll(resp.Body)
		return client.MapError("copy-from", &transport.HTTPStatusError{
			Status: resp.StatusCode,
			Body:   body,
		})
	}

	// Destination already a directory: extract straight in.
	if st, serr := os.Stat(localPath); serr == nil && st.IsDir() {
		return untar.Extract(resp.Body, localPath)
	}

	// Otherwise stage in a sibling directory so a failure leaves nothing behind.
	parent := filepath.Dir(localPath)
	if err := os.MkdirAll(parent, 0o755); err != nil {
		return err
	}
	staging, err := os.MkdirTemp(parent, ".sbx-cpfrom-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(staging)

	if err := untar.Extract(resp.Body, staging); err != nil {
		return err
	}
	base := path.Base(path.Clean(sandboxPath))
	if err := os.Rename(filepath.Join(staging, base), localPath); err != nil {
		return fmt.Errorf("copy-from: place %s: %w", base, err)
	}
	return nil
}

// ensureRunningForCopy starts the sandbox when it is not running. The /files
// endpoint returns 409 for a stopped sandbox, whereas `sbx cp` starts it.
func (s *Sandbox) ensureRunningForCopy(ctx context.Context) error {
	info, err := s.Inspect(ctx)
	if err != nil {
		return err
	}
	if info.Status == StatusRunning {
		return nil
	}
	return s.Start(ctx)
}
