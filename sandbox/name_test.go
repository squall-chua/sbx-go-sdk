package sandbox

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestGenerateName_Default(t *testing.T) {
	existing := map[string]bool{}
	got := generateName("claude", "/home/u/myproj", existing)
	require.Equal(t, "claude-myproj", got)
}

func TestGenerateName_Sanitizes(t *testing.T) {
	got := generateName("claude", "/home/u/My Proj!", map[string]bool{})
	require.Equal(t, "claude-My-Proj", got) // spaces->-, invalid chars dropped
}

func TestGenerateName_CollisionSuffix(t *testing.T) {
	existing := map[string]bool{"claude-myproj": true, "claude-myproj-2": true}
	got := generateName("claude", "/x/myproj", existing)
	require.Equal(t, "claude-myproj-3", got)
}
