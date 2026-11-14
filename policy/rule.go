package policy

import (
	"context"
	"errors"
	"strings"

	"github.com/squall-chua/sbx-go-sdk/client"
)

// RuleOption narrows a rule that AllowRule or DenyRule adds, or picks out the
// rule that RemoveRuleWith removes.
type RuleOption func(*ruleConfig)

type ruleConfig struct {
	protocols []string
	methods   []string
	path      string
	query     []string
}

// WithProtocols sets the transports a rule covers (`--protocol`, "tcp" or
// "udp", added in sbx v0.45.0). Unset, an allow covers tcp and a deny covers
// both. The CLI saves a UDP rule even while UDP egress is off, and only warns
// (enable feature.udp-egress); the SDK drops that warning. RemoveRuleWith
// rejects this option.
func WithProtocols(protocols ...string) RuleOption {
	return func(c *ruleConfig) { c.protocols = append(c.protocols, protocols...) }
}

// WithMethods makes the rule an HTTP rule for these methods (`--method`), or
// "ANY" for every method. WithPath and WithQuery need it.
func WithMethods(methods ...string) RuleOption {
	return func(c *ruleConfig) { c.methods = append(c.methods, methods...) }
}

// WithPath restricts an HTTP rule to a path glob (`--path`, e.g. "/v1/**").
// The CLI defaults to "/**".
func WithPath(glob string) RuleOption { return func(c *ruleConfig) { c.path = glob } }

// WithQuery requires query parameters on an HTTP rule (`--query`, each a
// "name=value" pattern, added in sbx v0.50.0).
func WithQuery(pairs ...string) RuleOption {
	return func(c *ruleConfig) { c.query = append(c.query, pairs...) }
}

func (c *ruleConfig) args() []string {
	var args []string
	for _, p := range c.protocols {
		args = append(args, "--protocol", p)
	}
	if len(c.methods) > 0 {
		args = append(args, "--method", strings.Join(c.methods, ","))
	}
	if c.path != "" {
		args = append(args, "--path", c.path)
	}
	for _, q := range c.query {
		args = append(args, "--query", q)
	}
	return args
}

func newRuleConfig(opts []RuleOption) *ruleConfig {
	var c ruleConfig
	for _, o := range opts {
		o(&c)
	}
	return &c
}

// AllowRule is Allow with rule options: a protocol, or HTTP method, path and
// query scoping. The CLI makes one rule per host.
//
// It adds a rule rather than editing one. The CLI refuses an allow when the
// same scope already holds a deny for the same resource, protocols and HTTP
// constraints; remove that rule first.
func AllowRule(ctx context.Context, c *client.Client, scope string, hosts []string, opts ...RuleOption) error {
	return addRule(ctx, c, "allow", scope, hosts, opts)
}

// DenyRule is Deny with rule options, as for AllowRule.
func DenyRule(ctx context.Context, c *client.Client, scope string, hosts []string, opts ...RuleOption) error {
	return addRule(ctx, c, "deny", scope, hosts, opts)
}

func addRule(ctx context.Context, c *client.Client, verb, scope string, hosts []string, opts []RuleOption) error {
	args := append([]string{"policy", verb, "network"}, scopeArgs(scope)...)
	args = append(args, strings.Join(hosts, ","))
	return run(ctx, c, append(args, newRuleConfig(opts).args()...)...)
}

// RemoveRuleWith is RemoveRule for an HTTP rule. Pass the same WithMethods,
// WithPath and WithQuery the rule was added with: the CLI matches them
// exactly, and anything less is "rule not found". PolicyRule.Methods and
// HTTPTargets hold them.
//
// A network rule comes off by resource alone, whatever its protocols. `sbx
// policy rm` has no --protocol, so removing by resource drops a host's tcp and
// udp rules together. WithProtocols is rejected rather than ignored, because
// the removal would be wider than it reads.
func RemoveRuleWith(ctx context.Context, c *client.Client, scope, resource string, opts ...RuleOption) error {
	cfg := newRuleConfig(opts)
	if len(cfg.protocols) > 0 {
		return errors.New("policy rm: WithProtocols is not supported; sbx policy rm has no --protocol")
	}
	args := append([]string{"policy", "rm", "network"}, scopeArgs(scope)...)
	args = append(args, "--resource", resource, "--force")
	return run(ctx, c, append(args, cfg.args()...)...)
}
