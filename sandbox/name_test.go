package sandbox

import (
	"strings"
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

// sbx v0.51.0 rejects a name with a plus sign ("sandbox name cannot contain
// plus signs"), so a workspace like "c++" must not leak one into the name.
func TestGenerateName_NoPlusSign(t *testing.T) {
	require.Equal(t, "shell-c", generateName("shell", "/src/c++", nil))
	require.Equal(t, "shell-a-b", generateName("shell", "/src/a+b", nil))
}

// sbx rejects names over 63 characters ("sandbox name cannot exceed 63
// characters"). A long workspace basename must be cut, and a collision suffix
// must fit inside the limit rather than push past it.
func TestGenerateName_FitsTheLengthLimit(t *testing.T) {
	long := "/src/" + strings.Repeat("a", 80)
	base := generateName("shell", long, nil)
	require.Len(t, base, 63)
	require.True(t, strings.HasPrefix(base, "shell-aaa"))

	next := generateName("shell", long, map[string]bool{base: true})
	require.Len(t, next, 63)
	require.True(t, strings.HasSuffix(next, "-2"))
	require.NotEqual(t, base, next)

	// Cutting must not leave a trailing hyphen.
	cut := generateName("shell", "/src/"+strings.Repeat("a", 56)+"-bbbbbbbb", nil)
	require.False(t, strings.HasSuffix(cut, "-"), cut)
}
