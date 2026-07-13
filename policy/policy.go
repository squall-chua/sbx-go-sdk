// Package policy manages sandbox network/egress policies. Rule management is
// engine-layer (no working daemon REST path in v0.35.0), so mutations and listing
// shell out to `sbx policy`; only Log uses REST (GET /network/log).
package policy

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/squall-chua/sbx-go-sdk/client"
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
// Shells out to `sbx policy init` (the old `policy set-default` name was deprecated
// to an alias in sbx v0.34.0).
func SetDefault(ctx context.Context, c *client.Client, name string) error {
	return run(ctx, c, "policy", "init", name)
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

// PolicyRule is one rule from `sbx policy ls --json`, modelling the daemon's
// filtered rule response. sbx v0.35.0 replaced the flat per-rule table with a
// per-policy summary table, so List reads the stable --json rule stream instead.
type PolicyRule struct {
	ID           string   `json:"id"`
	Name         string   `json:"name"`
	PolicyID     string   `json:"policy_id"`
	Scope        string   `json:"scope"`         // "global" or a sandbox scope
	AppliesTo    string   `json:"applies_to"`    // "all" or a sandbox name
	ResourceType string   `json:"resource_type"` // "network", "filesystem read", …
	Decision     string   `json:"decision"`      // "allow" | "deny"
	Resources    []string `json:"resources"`
	Origin       string   `json:"origin"` // "local" or a remote-governance source
	Status       string   `json:"status"` // "active" | "inactive"
	Editable     bool     `json:"editable"`
}

// List returns the parsed `sbx policy ls [SCOPE] --json` rules. scope "" lists all
// policies; a sandbox name filters to rules that apply to it. A JSON-shape change
// yields client.ErrUnexpectedFormat — use ListRaw for the raw human table.
func List(ctx context.Context, c *client.Client, scope string) ([]PolicyRule, error) {
	args := []string{"policy", "ls"}
	if scope != "" {
		args = append(args, scope)
	}
	args = append(args, "--json")
	raw, err := capture(ctx, c, args...)
	if err != nil {
		return nil, err
	}
	var resp struct {
		Rules []PolicyRule `json:"rules"`
	}
	if err := json.Unmarshal([]byte(raw), &resp); err != nil {
		return nil, fmt.Errorf("policy list: %w: %w", client.ErrUnexpectedFormat, err)
	}
	return resp.Rules, nil
}

// ListRaw returns the raw `sbx policy ls [SCOPE]` human table text.
func ListRaw(ctx context.Context, c *client.Client, scope string) (string, error) {
	args := []string{"policy", "ls"}
	if scope != "" {
		args = append(args, scope)
	}
	return capture(ctx, c, args...)
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
