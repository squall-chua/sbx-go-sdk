package sandbox

import (
	"path/filepath"
	"strconv"
	"strings"
)

// allowed name charset: letters, digits, '.', '+', '-'.
func sanitize(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9',
			r == '.', r == '+', r == '-':
			b.WriteRune(r)
		case r == ' ' || r == '_' || r == '/':
			b.WriteByte('-')
		}
	}
	return strings.Trim(b.String(), "-")
}

// generateName derives "<agent>-<basename(workspace)>", sanitized and collision-
// resolved against existing names by appending "-N".
func generateName(agent, primaryWorkspace string, existing map[string]bool) string {
	base := sanitize(agent + "-" + filepath.Base(primaryWorkspace))
	if !existing[base] {
		return base
	}
	for i := 2; ; i++ {
		cand := base + "-" + strconv.Itoa(i)
		if !existing[cand] {
			return cand
		}
	}
}
