// Package secret manages stored sandbox secrets via shell-out to `sbx secret`.
// For headless agent credentials, prefer exec.WithEnv; SetCustom is EXPERIMENTAL
// upstream.
package secret

import (
	"context"

	"github.com/squall-chua/sbx-go-sdk/client"
)

// CustomSecret describes a custom proxy-injected secret.
type CustomSecret struct {
	Host        string // target host whose outbound requests get the real secret
	Env         string // env var set (to the placeholder) inside the sandbox
	Value       string // the real secret
	Placeholder string // optional; supports a {rand} suffix
}

// scopeArg returns "-g" for global ("") or the sandbox name as a positional arg.
// NOTE: `sbx secret` uses "-g"/bare positional; `sbx policy` uses "--sandbox NAME"
// (see policy.scopeArgs). The encodings differ per CLI on purpose — do not unify.
func scopeArg(scope string) string {
	if scope == "" {
		return "-g"
	}
	return scope
}

// SetCustom creates/updates a custom secret in scope ("" = global). EXPERIMENTAL.
// The Value is passed as a `sbx secret set-custom --value` CLI argument, so it is
// briefly visible in host process listings.
func SetCustom(ctx context.Context, c *client.Client, scope string, s CustomSecret) error {
	args := []string{"secret", "set-custom", scopeArg(scope), "--host", s.Host, "--env", s.Env, "--value", s.Value}
	if s.Placeholder != "" {
		args = append(args, "--placeholder", s.Placeholder)
	}
	r, err := c.Runner()
	if err != nil {
		return err
	}
	_, err = r.Capture(ctx, nil, args...)
	return err
}

// List returns the raw `sbx secret ls [SCOPE]` text (no --json upstream).
func List(ctx context.Context, c *client.Client, scope string) (string, error) {
	args := []string{"secret", "ls"}
	if scope != "" {
		args = append(args, scope)
	}
	r, err := c.Runner()
	if err != nil {
		return "", err
	}
	return r.Capture(ctx, nil, args...)
}

// Remove deletes a secret (service) in scope ("" = global). Uses -f to skip the
// confirmation prompt (the CLI would otherwise block on non-TTY stdin).
func Remove(ctx context.Context, c *client.Client, scope, service string) error {
	args := []string{"secret", "rm", scopeArg(scope)}
	if service != "" {
		args = append(args, service)
	}
	args = append(args, "-f")
	r, err := c.Runner()
	if err != nil {
		return err
	}
	_, err = r.Capture(ctx, nil, args...)
	return err
}
