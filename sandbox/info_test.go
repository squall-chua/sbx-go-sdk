package sandbox_test

import (
	"context"
	"testing"

	"github.com/squall-chua/sbx-go-sdk/sandbox"
)

// Compile-time proof that Inspect's return type is the externally-nameable
// sandbox.Info, closing the internal/api.SandboxInfo leak: an external importer
// cannot import internal/api, so it must be able to name the type via sandbox.
var _ = func(s *sandbox.Sandbox, ctx context.Context) sandbox.Info {
	info, _ := s.Inspect(ctx)
	return info
}

func TestInfoIsExternallyNameable(t *testing.T) {
	var info sandbox.Info
	info.Name = "demo"
	info.Status = sandbox.StatusRunning
	if info.Name != "demo" || info.Status != sandbox.StatusRunning {
		t.Fatalf("unexpected info: %+v", info)
	}
}
