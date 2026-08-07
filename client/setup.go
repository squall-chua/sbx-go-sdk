package client

import (
	"context"
	"fmt"
	"regexp"
	"strings"
)

// SetupItem is one detected row of the `sbx setup` report.
type SetupItem struct {
	// Section is the uppercase heading the row sits under, e.g. "PREREQUISITES",
	// "SECRETS", "SKILLS", "GOVERNANCE", "MCP".
	Section string
	// Name labels what was detected, e.g. "host", "agent secrets", "Local policy".
	Name string
	// Detail describes what was found, e.g. "sbx, Docker", "54 skill(s), 2 folders".
	Detail string
	// Status is the right-hand verdict, e.g. "Ready", "0 found", "11 conflict(s)".
	// Empty when the CLI prints only two columns.
	Status string
}

// SetupReport is what `sbx setup` detects about the host.
type SetupReport struct {
	Items []SetupItem
}

// Section returns the items under one heading, in the order printed.
func (r *SetupReport) Section(name string) []SetupItem {
	var out []SetupItem
	for _, it := range r.Items {
		if it.Section == name {
			out = append(out, it)
		}
	}
	return out
}

// setupSection matches a bare uppercase heading line ("PREREQUISITES", "MCP").
var setupSection = regexp.MustCompile(`^[A-Z][A-Z ]*$`)

// gutter splits the report's columns: runs of two or more spaces.
var setupGutter = regexp.MustCompile(`\s{2,}`)

// DetectSetup reports what `sbx setup` finds already configured on the host:
// prerequisites, agent secrets, importable skills, the governance policy, and
// host MCP servers.
//
// `sbx setup` is a TTY wizard, but with no terminal it degrades to exactly this
// read-only detection pass and exits 0 — it changes nothing, and this call keeps
// it that way by giving the child an empty stdin. Acting on what it finds is the
// interactive half, which stays unwrapped; reach for secret.ImportAll,
// skillstore.Import, policy.SetDefault and the mcp package instead.
//
// The MCP row counts servers detected in the *host's* agent configuration that
// setup would offer to import, which is a different set from mcp.List.
//
// This parses a human report — `sbx setup` has no --json flag and no REST path
// (ADR 0002). A layout change yields ErrUnexpectedFormat; DetectSetupRaw returns
// the text unparsed.
func (c *Client) DetectSetup(ctx context.Context) (*SetupReport, error) {
	raw, err := c.DetectSetupRaw(ctx)
	if err != nil {
		return nil, err
	}
	return parseSetupReport(raw)
}

// DetectSetupRaw returns the raw `sbx setup` detection text.
func (c *Client) DetectSetupRaw(ctx context.Context) (string, error) {
	r, err := c.runnerOrErr()
	if err != nil {
		return "", err
	}
	// Empty stdin, not the caller's: a terminal on stdin is what turns `sbx setup`
	// from a detection pass into a wizard that writes to the host.
	return r.CaptureStdin(ctx, strings.NewReader(""), nil, "setup")
}

func parseSetupReport(raw string) (*SetupReport, error) {
	var rep SetupReport
	section := ""
	for _, line := range strings.Split(strings.ReplaceAll(raw, "\r\n", "\n"), "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "(") {
			continue
		}
		// An unindented line is either a heading or the report's own title.
		if !strings.HasPrefix(line, " ") {
			if setupSection.MatchString(trimmed) {
				section = trimmed
			}
			continue
		}
		cols := setupGutter.Split(trimmed, 3)
		item := SetupItem{Section: section, Name: cols[0]}
		if len(cols) > 1 {
			item.Detail = cols[1]
		}
		if len(cols) > 2 {
			item.Status = cols[2]
		}
		rep.Items = append(rep.Items, item)
	}
	if len(rep.Items) == 0 {
		return nil, fmt.Errorf("setup: %w: no detected rows in output", ErrUnexpectedFormat)
	}
	return &rep, nil
}
