//go:build integration

package integration

import (
	"context"
	"slices"
	"testing"

	"github.com/squall-chua/sbx-go-sdk/client"
	"github.com/squall-chua/sbx-go-sdk/policy"
	"github.com/squall-chua/sbx-go-sdk/sandbox"
	"github.com/stretchr/testify/require"
)

// TestPolicy_ScopedRules adds, reads back and removes rules in a throwaway
// sandbox's scope, so the host's global policy is never touched. Removing the
// sandbox purges its scoped rules, so a failed assertion leaks nothing.
func TestPolicy_ScopedRules(t *testing.T) {
	ctx := context.Background()
	c, err := client.New(ctx, client.WithAutoStart())
	require.NoError(t, err)
	sb, err := sandbox.Create(ctx, c, sandbox.WithAgent("shell"), sandbox.WithWorkspace(t.TempDir()))
	require.NoError(t, err)
	t.Cleanup(func() { sb.Remove(ctx, sandbox.WithForce()) })
	scope := sb.Name()

	find := func(host, resourceType string) *policy.PolicyRule {
		t.Helper()
		rules, err := policy.List(ctx, c, scope)
		require.NoError(t, err)
		for i, r := range rules {
			if r.AppliesTo == "sandbox:"+scope && r.ResourceType == resourceType && slices.Contains(r.Resources, host) {
				return &rules[i]
			}
		}
		return nil
	}

	// Several hosts in one call: the CLI rejects a second positional host.
	require.NoError(t, policy.Deny(ctx, c, scope, "a.sdk-test.example", "b.sdk-test.example"))
	require.NotNil(t, find("a.sdk-test.example", "network"))
	require.NotNil(t, find("b.sdk-test.example", "network"))
	require.NoError(t, policy.RemoveRule(ctx, c, scope, "a.sdk-test.example"))
	require.NoError(t, policy.RemoveRule(ctx, c, scope, "b.sdk-test.example"))
	require.Nil(t, find("a.sdk-test.example", "network"))

	require.NoError(t, policy.AllowRule(ctx, c, scope, []string{"udp.sdk-test.example"}, policy.WithProtocols("udp")))
	r := find("udp.sdk-test.example", "network")
	require.NotNil(t, r)
	require.Equal(t, []string{"net:connect:udp"}, r.Actions)
	require.NoError(t, policy.RemoveRule(ctx, c, scope, "udp.sdk-test.example"))

	http := []policy.RuleOption{policy.WithMethods("GET", "HEAD"), policy.WithPath("/v1/**"), policy.WithQuery("team=x")}
	require.NoError(t, policy.AllowRule(ctx, c, scope, []string{"api.sdk-test.example"}, http...))
	r = find("api.sdk-test.example", "http")
	require.NotNil(t, r)
	require.Equal(t, []string{"GET", "HEAD"}, r.Methods)
	require.Len(t, r.HTTPTargets, 1)
	require.Equal(t, "/v1/**", r.HTTPTargets[0].Path)
	require.Equal(t, []policy.QueryParam{{Name: "team", ValuePattern: "x"}}, r.HTTPTargets[0].Query)

	// The HTTP check honours the rule's method and path.
	url := "https://api.sdk-test.example/v1/items?team=x"
	auth, err := policy.CheckHTTP(ctx, c, url, policy.WithCheckSandbox(scope))
	require.NoError(t, err)
	require.True(t, auth.Allowed, "%+v", auth)
	require.Equal(t, "GET", auth.Method)
	require.Equal(t, r.ID, auth.Rule)
	auth, err = policy.CheckHTTP(ctx, c, url, policy.WithCheckSandbox(scope), policy.WithCheckMethod("POST"))
	require.NoError(t, err)
	require.False(t, auth.Allowed, "%+v", auth)
	_, err = policy.CheckHTTP(ctx, c, "api.sdk-test.example", policy.WithCheckSandbox(scope))
	require.Error(t, err, "the daemon rejects a URL that is not absolute")

	// Removal must repeat every HTTP constraint.
	require.Error(t, policy.RemoveRuleWith(ctx, c, scope, "api.sdk-test.example", policy.WithMethods("GET", "HEAD")))
	require.NoError(t, policy.RemoveRuleWith(ctx, c, scope, "api.sdk-test.example", http...))
	require.Nil(t, find("api.sdk-test.example", "http"))
}
