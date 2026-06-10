package exec

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestProcessOptions_BuildBody(t *testing.T) {
	body := buildBody([]string{"echo", "hi"},
		WithEnv(map[string]string{"CI": "1"}),
		WithWorkdir("/work"),
		WithUser("dev"),
		WithTTY(),
	)
	b, _ := json.Marshal(body)
	var got map[string]any
	require.NoError(t, json.Unmarshal(b, &got))
	require.Equal(t, []any{"echo", "hi"}, got["cmd"])
	require.Equal(t, "/work", got["workdir"])
	require.Equal(t, "dev", got["user"])
	require.Equal(t, true, got["tty"])
	require.Equal(t, map[string]any{"CI": "1"}, got["env"])
}
