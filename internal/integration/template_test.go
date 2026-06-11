//go:build integration

package integration

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/squall-chua/sbx-go-sdk/client"
	"github.com/squall-chua/sbx-go-sdk/sandbox"
	"github.com/squall-chua/sbx-go-sdk/template"
	"github.com/stretchr/testify/require"
)

// TestSmoke_TemplateSaveRemoveLoad live-exercises the template surface end to end:
// SaveTemplate (shell-out) -> List -> Remove (REST DELETE) -> Load (REST POST raw
// tar). Remove and Load were implemented from registered-but-unverified daemon
// routes; this test is the live confirmation of their wire shapes.
func TestSmoke_TemplateSaveRemoveLoad(t *testing.T) {
	ctx := context.Background()
	c, err := client.New(ctx, client.WithAutoStart())
	require.NoError(t, err)

	// Unique workspace basename -> unique sandbox name (shell-<basename>). A fresh
	// test process otherwise reuses basename "001", colliding with a leaked network
	// from a prior failed run.
	ws := filepath.Join(t.TempDir(), "tmpl"+strconv.Itoa(os.Getpid()))
	require.NoError(t, os.Mkdir(ws, 0o755))
	sb, err := sandbox.Create(ctx, c, sandbox.WithAgent("shell"), sandbox.WithWorkspace(ws))
	require.NoError(t, err)
	t.Cleanup(func() {
		sb.Remove(ctx)
		_ = exec.Command("sbx", "rm", "--force", sb.Name()).Run()
	})

	const repo = "sdk-tmpl-smoke"
	const ver = "v1"
	const tag = repo + ":" + ver
	// Best-effort tag cleanup regardless of which path the test takes.
	t.Cleanup(func() { _ = exec.Command("sbx", "template", "rm", tag).Run() })

	// 1. SaveTemplate (shell-out) snapshots the sandbox into the image store.
	//    The daemon refuses to snapshot a running sandbox, so stop it first.
	require.NoError(t, sb.Stop(ctx))
	require.NoError(t, sb.SaveTemplate(ctx, tag))

	// 2. List shows the saved tag (verifies SaveTemplate + List).
	require.True(t, hasTemplate(t, ctx, c, repo, ver), "saved template should appear in List")

	// 3. Export the saved image to a tar via the CLI for the Load round-trip.
	tarPath := filepath.Join(t.TempDir(), "tmpl.tar")
	out, err := exec.Command("sbx", "template", "save", sb.Name(), tag, "--output", tarPath).CombinedOutput()
	require.NoErrorf(t, err, "save --output: %s", out)

	// 4. Remove (REST DELETE /docker/images/remove) — unverified wire shape.
	require.NoError(t, template.Remove(ctx, c, tag))
	require.False(t, hasTemplate(t, ctx, c, repo, ver), "Remove should delete the tag from the store")

	// 5. Load (REST POST /docker/images/load, raw tar body) — unverified wire shape.
	f, err := os.Open(tarPath)
	require.NoError(t, err)
	defer f.Close()
	require.NoError(t, template.Load(ctx, c, f))
	require.True(t, hasTemplate(t, ctx, c, repo, ver), "Load should restore the tag to the store")
}

// hasTemplate reports whether List returns an image whose repository contains
// repoSub and whose tag equals tag. The daemon may normalize a short tag like
// "sdk-tmpl-smoke:v1" to a fully-qualified repository, so repository is matched
// by substring.
func hasTemplate(t *testing.T, ctx context.Context, c *client.Client, repoSub, tag string) bool {
	t.Helper()
	imgs, err := template.List(ctx, c)
	require.NoError(t, err)
	for _, img := range imgs {
		if strings.Contains(img.Repository, repoSub) && img.Tag == tag {
			return true
		}
	}
	return false
}
