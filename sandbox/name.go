package sandbox

import (
	"path/filepath"
	"strconv"
	"strings"
)

// maxNameLen is sbx's sandbox-name limit ("sandbox name cannot exceed 63
// characters", enforced since v0.43.0).
const maxNameLen = 63

// allowed name charset: letters, digits, '.', '-'. sbx v0.51.0 rejects '+'
// ("sandbox name cannot contain plus signs"), so it becomes '-'.
func sanitize(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9',
			r == '.', r == '-':
			b.WriteRune(r)
		case r == ' ' || r == '_' || r == '/' || r == '+':
			b.WriteByte('-')
		}
	}
	return strings.Trim(b.String(), "-")
}

// fit cuts s so that s+suffix stays within maxNameLen, without leaving a
// trailing hyphen. sanitize keeps names ASCII, so bytes are characters.
func fit(s, suffix string) string {
	if n := maxNameLen - len(suffix); len(s) > n {
		s = strings.TrimRight(s[:n], "-")
	}
	return s + suffix
}

// generateName derives "<agent>-<basename(workspace)>", sanitized, cut to sbx's
// 63-character limit, and collision-resolved against existing names by
// appending "-N" (inside the limit).
func generateName(agent, primaryWorkspace string, existing map[string]bool) string {
	base := sanitize(agent + "-" + filepath.Base(primaryWorkspace))
	if cand := fit(base, ""); !existing[cand] {
		return cand
	}
	for i := 2; ; i++ {
		cand := fit(base, "-"+strconv.Itoa(i))
		if !existing[cand] {
			return cand
		}
	}
}
