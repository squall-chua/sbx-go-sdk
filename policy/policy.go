// Package policy manages sandbox network/egress policies. Rule management is
// engine-layer (no working daemon REST path in v0.32.0), so mutations and listing
// shell out to `sbx policy`; only Log uses REST (GET /network/log).
package policy

import (
	"context"

	"github.com/squall-chua/sbx-go-sdk/client"
)

// scopeArgs appends "--sandbox NAME" when scope is non-empty (global otherwise).
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
func SetDefault(ctx context.Context, c *client.Client, policy string) error {
	return run(ctx, c, "policy", "set-default", policy)
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

// RemoveRule removes the network rule(s) for scope ("" = global).
func RemoveRule(ctx context.Context, c *client.Client, scope string) error {
	args := append([]string{"policy", "rm", "network"}, scopeArgs(scope)...)
	return run(ctx, c, args...)
}

// Reset clears all policies back to defaults.
func Reset(ctx context.Context, c *client.Client) error {
	return run(ctx, c, "policy", "reset")
}
