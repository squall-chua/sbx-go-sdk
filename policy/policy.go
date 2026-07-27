// Package policy manages sandbox access policies. Listing and checking use the
// daemon REST API (GET /policy/network/rules, POST /policy/network/check,
// GET /network/log); rule mutation and inspection shell out to `sbx policy`,
// which has no REST equivalent for those operations.
package policy

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"

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
	Origin       string   `json:"origin"` // "local", "org" (remote governance), or "kit"
	Status       string   `json:"status"` // "active" | "inactive"
	Editable     bool     `json:"editable"`
}

// List returns the daemon's policy rules (REST GET /policy/network/rules).
// scope "" lists every policy; a sandbox name filters to rules that apply to it.
//
// The request always sends type=all. The endpoint otherwise returns network
// rules only, silently omitting filesystem rules with no error and no drift
// signal, so this is deliberately not configurable — see docs/adr/0003.
//
// A JSON-shape change yields client.ErrUnexpectedFormat; use ListRaw for the
// human-rendered table, which also shows non-network rules.
func List(ctx context.Context, c *client.Client, scope string) ([]PolicyRule, error) {
	q := url.Values{}
	q.Set("type", "all")
	if scope != "" {
		q.Set("sandbox", scope)
	}
	route := "/policy/network/rules?" + q.Encode()

	var resp struct {
		Rules []PolicyRule `json:"rules"`
	}
	if err := c.Transport().DoJSON(ctx, http.MethodGet, route, nil, &resp); err != nil {
		if isDecodeError(err) {
			return nil, fmt.Errorf("policy list: %w: %w", client.ErrUnexpectedFormat, err)
		}
		return nil, client.MapError("policy-list", err)
	}
	return resp.Rules, nil
}

// isDecodeError reports whether err came from JSON decoding rather than transport.
func isDecodeError(err error) bool {
	var se *json.SyntaxError
	var te *json.UnmarshalTypeError
	return errors.As(err, &se) || errors.As(err, &te)
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

// InspectRaw returns the human-rendered detail for a policy or rule
// (`sbx policy inspect <policy-or-rule>`). The selector may be a policy ID,
// policy name, rule ID, or rule name; use List to find them.
//
// The output is unparsed on purpose: `sbx policy inspect` has no --json flag and
// no REST path, so any parser here would be pinned to a human layout that is
// free to change. Callers that need structure should use List.
func InspectRaw(ctx context.Context, c *client.Client, selector string) (string, error) {
	if selector == "" {
		return "", errors.New("policy inspect: selector must not be empty")
	}
	return capture(ctx, c, "policy", "inspect", selector)
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
