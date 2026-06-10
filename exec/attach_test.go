package exec

import (
	"context"
	"io"
	"testing"

	"github.com/squall-chua/sbx-go-sdk/sandbox"
	"github.com/stretchr/testify/require"
)

func TestExecInteractive_StreamsAndWaits(t *testing.T) {
	c, _ := attachStub(t)
	sb := sandbox.NewForTest(c, "s1")
	sess, err := ExecInteractive(context.Background(), sb, []string{"echo", "hi"}, WithTTY())
	require.NoError(t, err)
	defer sess.Close()
	out, _ := io.ReadAll(sess.Stdout())
	require.Contains(t, string(out), "hello") // stub streams a stdcopy "hello\n" frame
	code, err := sess.Wait(context.Background())
	require.NoError(t, err)
	require.Equal(t, 0, code)
}
