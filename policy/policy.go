// Package policy manages sandbox network/egress policies. Rule management is
// engine-layer (no working daemon REST path in v0.33.0), so mutations and listing
// shell out to `sbx policy`; only Log uses REST (GET /network/log).
package policy

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	"github.com/squall-chua/sbx-go-sdk/client"
	"github.com/squall-chua/sbx-go-sdk/internal/coltable"
)

// scopeArgs appends "--sandbox NAME" when scope is non-empty (global otherwise).
// NOTE: `sbx policy` uses "--sandbox NAME"; `sbx secret` uses "-g"/bare positional
// (see secret.scopeArg). The encodings differ per CLI on purpose — do not unify.
func scopeArgs(scope string) []string {
	if scope == "" {
		return nil
	}
	return []string{"--sandbox", scope}
}

func run(ctx context.Context, c *client.Client, args ...string) error {
	r, err := c.Runner()
	if err != nil {
		return err
	}
	_, err = r.Capture(ctx, nil, args...)
	return err
}

// SetDefault sets the baseline network policy: "allow-all", "balanced", or "deny-all".
func SetDefault(ctx context.Context, c *client.Client, name string) error {
	return run(ctx, c, "policy", "set-default", name)
}

// Allow adds an allow rule for the given hosts within scope ("" = global).
func Allow(ctx context.Context, c *client.Client, scope string, hosts ...string) error {
	args := append([]string{"policy", "allow", "network"}, scopeArgs(scope)...)
	return run(ctx, c, append(args, hosts...)...)
}

// Deny adds a deny rule for the given hosts within scope ("" = global).
func Deny(ctx context.Context, c *client.Client, scope string, hosts ...string) error {
	args := append([]string{"policy", "deny", "network"}, scopeArgs(scope)...)
	return run(ctx, c, append(args, hosts...)...)
}

// RemoveRule removes the network rule for resource within scope ("" = global).
// sbx requires a selector, so resource is mandatory.
func RemoveRule(ctx context.Context, c *client.Client, scope, resource string) error {
	args := append([]string{"policy", "rm", "network"}, scopeArgs(scope)...)
	args = append(args, "--resource", resource)
	return run(ctx, c, args...)
}

// Reset clears all policies back to defaults.
func Reset(ctx context.Context, c *client.Client) error {
	return run(ctx, c, "policy", "reset")
}

// capture runs an sbx subcommand and returns its stdout text.
func capture(ctx context.Context, c *client.Client, args ...string) (string, error) {
	r, err := c.Runner()
	if err != nil {
		return "", err
	}
	return r.Capture(ctx, nil, args...)
}

// policyHeader is the column header of `sbx policy ls`, in order. Drift from this
// set is reported as client.ErrUnexpectedFormat.
var policyHeader = []string{"PROVENANCE", "APPLIES_TO", "POLICY/RULE", "TYPE", "DECISION", "RESOURCES"}

// PolicyRule is one rule from `sbx policy ls`, modelling exactly its columns.
type PolicyRule struct {
	Provenance string   // "local", or a remote-governance source
	AppliesTo  string   // "all" or a sandbox name
	Rule       string   // POLICY/RULE — rule name, or ID when unnamed
	Type       string   // "network"
	Decision   string   // "allow" | "deny"
	Resources  []string // hosts, gathered across continuation rows
}

// List returns the parsed `sbx policy ls [SCOPE]` rules. scope "" lists global+all.
// A format change in the CLI's table yields client.ErrUnexpectedFormat — use
// ListRaw to fall back to the unparsed text.
func List(ctx context.Context, c *client.Client, scope string) ([]PolicyRule, error) {
	raw, err := ListRaw(ctx, c, scope)
	if err != nil {
		return nil, err
	}
	return parsePolicyList(raw)
}

// ListRaw returns the raw `sbx policy ls [SCOPE]` text.
func ListRaw(ctx context.Context, c *client.Client, scope string) (string, error) {
	args := []string{"policy", "ls"}
	if scope != "" {
		args = append(args, scope)
	}
	return capture(ctx, c, args...)
}

// parsePolicyList maps the policy table to rules. A row with a non-blank PROVENANCE
// starts a new rule; a continuation row (blank before RESOURCES) appends its host
// to the current rule. A missing header means an empty listing (not an error).
func parsePolicyList(raw string) ([]PolicyRule, error) {
	rows, err := coltable.Parse(raw, policyHeader)
	if errors.Is(err, coltable.ErrNoHeader) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("policy list: %w: %w", client.ErrUnexpectedFormat, err)
	}
	var out []PolicyRule
	for _, r := range rows {
		if r["PROVENANCE"] != "" {
			out = append(out, PolicyRule{
				Provenance: r["PROVENANCE"],
				AppliesTo:  r["APPLIES_TO"],
				Rule:       r["POLICY/RULE"],
				Type:       r["TYPE"],
				Decision:   r["DECISION"],
			})
		}
		if res := r["RESOURCES"]; res != "" && len(out) > 0 {
			out[len(out)-1].Resources = append(out[len(out)-1].Resources, res)
		}
	}
	return out, nil
}

// Profiles returns the raw `sbx policy profile ls` text.
func Profiles(ctx context.Context, c *client.Client) (string, error) {
	return capture(ctx, c, "policy", "profile", "ls")
}

// LogEntry is one allowed/blocked host record from the proxy.
type LogEntry struct {
	Host       string `json:"host"`
	VMName     string `json:"vm_name"`
	ProxyType  string `json:"proxy_type"`
	Rule       string `json:"rule"`
	LastSeen   string `json:"last_seen"`
	Since      string `json:"since"`
	CountSince int    `json:"count_since"`
}

// PolicyLog is the /network/log response.
type PolicyLog struct {
	BlockedHosts []LogEntry `json:"blocked_hosts"`
	AllowedHosts []LogEntry `json:"allowed_hosts"`
}

// Log returns the proxy's allowed/blocked-host log (REST GET /network/log).
func Log(ctx context.Context, c *client.Client) (*PolicyLog, error) {
	var pl PolicyLog
	if err := c.Transport().DoJSON(ctx, http.MethodGet, "/network/log", nil, &pl); err != nil {
		return nil, client.MapError("policy-log", err)
	}
	return &pl, nil
}
