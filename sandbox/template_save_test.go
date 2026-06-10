package sandbox

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSaveTemplate(t *testing.T) {
	argFile := filepath.Join(t.TempDir(), "args.txt")
	c := clientWithRecordingSbx(t, argFile)
	sb := NewForTest(c, "s1")
	require.NoError(t, sb.SaveTemplate(context.Background(), "myimg:v1"))
	data, _ := os.ReadFile(argFile)
	require.Contains(t, string(data), "template save s1 myimg:v1")
}
