//go:build integration

package integration

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/squall-chua/sbx-go-sdk/client"
	"github.com/squall-chua/sbx-go-sdk/sandbox"
	"github.com/squall-chua/sbx-go-sdk/template"
	"github.com/stretchr/testify/require"
)

func TestSmoke_PortsCpTemplate(t *testing.T) {
	ctx := context.Background()
	c, err := client.New(ctx, client.WithAutoStart())
	require.NoError(t, err)

	sb, err := sandbox.Create(ctx, c, sandbox.WithAgent("shell"), sandbox.WithWorkspace(t.TempDir()))
	require.NoError(t, err)
	t.Cleanup(func() { sb.Remove(ctx) })

	// ports: publish + list
	_, err = sb.PublishPort(ctx, sandbox.Port{SandboxPort: 8080, HostPort: 0, HostIP: "127.0.0.1", Protocol: "tcp"})
	require.NoError(t, err)
	ports, err := sb.Ports(ctx)
	require.NoError(t, err)
	require.NotEmpty(t, ports)

	// cp: host -> sandbox -> host round-trip
	dir := t.TempDir()
	src := filepath.Join(dir, "in.txt")
	require.NoError(t, os.WriteFile(src, []byte("sdk-cp-roundtrip"), 0o644))
	require.NoError(t, sb.CopyTo(ctx, src, "/tmp/in.txt"))
	dst := filepath.Join(dir, "out.txt")
	require.NoError(t, sb.CopyFrom(ctx, "/tmp/in.txt", dst))
	got, _ := os.ReadFile(dst)
	require.Equal(t, "sdk-cp-roundtrip", string(got))

	// templates: list (base images always present)
	imgs, err := template.List(ctx, c)
	require.NoError(t, err)
	require.NotEmpty(t, imgs)
}
