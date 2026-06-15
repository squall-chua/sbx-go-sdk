// Package coltable parses the fixed-width, whitespace-aligned tables the sbx CLI
// prints (e.g. `sbx policy ls`, `sbx secret ls`). It is generic: it finds and
// validates a header row, then slices each data line at the header's column
// offsets. It does not know about table-specific quirks (continuation rows,
// multiple sections) — callers handle those over the returned rows.
package coltable

import (
	"errors"
	"regexp"
	"slices"
	"strings"
)

var (
	// ErrNoHeader means no header-like row was found — treat as an empty listing.
	ErrNoHeader = errors.New("coltable: no header row found")
	// ErrHeaderMismatch means a header-like row was found but its columns do not
	// match the expected set — the CLI's table format has drifted.
	ErrHeaderMismatch = errors.New("coltable: header does not match expected columns")
)

// gutter matches the run of two or more spaces that separates table columns.
var gutter = regexp.MustCompile(`\s{2,}`)

// Parse locates the first header-like row, validates it equals want (same columns,
// same order), and returns each following non-blank line sliced into a map keyed by
// the want column names. Every field is trimmed of surrounding whitespace.
func Parse(raw string, want []string) ([]map[string]string, error) {
	lines := strings.Split(strings.ReplaceAll(raw, "\r\n", "\n"), "\n")
	hi := -1
	for i, ln := range lines {
		if isHeaderLike(ln) {
			hi = i
			break
		}
	}
	if hi == -1 {
		return nil, ErrNoHeader
	}
	offsets, err := headerOffsets(lines[hi], want)
	if err != nil {
		return nil, err
	}
	var rows []map[string]string
	for _, ln := range lines[hi+1:] {
		if strings.TrimSpace(ln) == "" {
			continue
		}
		rows = append(rows, sliceRow(ln, want, offsets))
	}
	return rows, nil
}

// isHeaderLike reports whether ln looks like a column-header row: at least two
// columns, with a first column that is an all-uppercase token (the convention the
// sbx tables use: PROVENANCE, SCOPE, ...). This distinguishes the header from
// banner lines, prose ("No secrets found ..."), single-column section labels
// ("CUSTOM SECRETS"), and lower-cased data rows.
func isHeaderLike(ln string) bool {
	cols := splitCols(ln)
	if len(cols) < 2 {
		return false
	}
	first := cols[0]
	if len(first) < 2 {
		return false
	}
	for _, r := range first {
		if !((r >= 'A' && r <= 'Z') || r == '_' || r == '/') {
			return false
		}
	}
	return true
}

// splitCols splits a trimmed line on runs of two or more spaces, dropping empties.
// Single spaces inside a column are preserved.
func splitCols(ln string) []string {
	ln = strings.TrimSpace(ln)
	if ln == "" {
		return nil
	}
	return gutter.Split(ln, -1)
}

// headerOffsets validates that line's columns equal want exactly, then returns the
// start byte-offset of each column (the index of each want token, in order).
func headerOffsets(line string, want []string) ([]int, error) {
	if !slices.Equal(splitCols(line), want) {
		return nil, ErrHeaderMismatch
	}
	offsets := make([]int, len(want))
	cur := 0
	for i, col := range want {
		idx := strings.Index(line[cur:], col)
		if idx < 0 {
			return nil, ErrHeaderMismatch
		}
		offsets[i] = cur + idx
		cur = offsets[i] + len(col)
	}
	return offsets, nil
}

// sliceRow cuts ln at the column offsets, trimming each field. The final column
// extends to end-of-line. Columns a short line doesn't reach yield "".
func sliceRow(ln string, want []string, offsets []int) map[string]string {
	m := make(map[string]string, len(want))
	for i, col := range want {
		start := offsets[i]
		if start > len(ln) {
			m[col] = ""
			continue
		}
		end := len(ln)
		if i+1 < len(want) && offsets[i+1] < end {
			end = offsets[i+1]
		}
		m[col] = strings.TrimSpace(ln[start:end])
	}
	return m
}
