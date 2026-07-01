package settings

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSettingBool(t *testing.T) {
	s := Setting{Key: "feature.x", Value: json.RawMessage(`false`), Type: "bool"}
	b, err := s.Bool()
	require.NoError(t, err)
	require.False(t, b)

	_, err = Setting{Key: "ssh.port", Value: json.RawMessage(`2222`)}.Bool()
	require.Error(t, err) // not a bool
}

func TestSettingText(t *testing.T) {
	require.Equal(t, "shell", Setting{Value: json.RawMessage(`"shell"`)}.Text())                 // string -> unquoted
	require.Equal(t, "2222", Setting{Value: json.RawMessage(`2222`)}.Text())                     // number -> as-is
	require.Equal(t, `["docker.io/"]`, Setting{Value: json.RawMessage(`["docker.io/"]`)}.Text()) // array -> raw JSON
}
