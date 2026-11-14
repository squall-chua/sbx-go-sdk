package policy

import (
	"context"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRuleSiblings_Argv(t *testing.T) {
	argFile := filepath.Join(t.TempDir(), "args.txt")
	c := recordingClient(t, argFile)
	ctx := context.Background()

	require.NoError(t, AllowRule(ctx, c, "sb", []string{"api.example.com", "cdn.example.com"},
		WithMethods("GET", "HEAD"), WithPath("/v1/**"), WithQuery("team=x", "env=prod")))
	require.NoError(t, DenyRule(ctx, c, "", []string{"media.example.com"}, WithProtocols("udp")))
	require.NoError(t, AllowRule(ctx, c, "", []string{"**"}, WithProtocols("tcp", "udp")))
	require.NoError(t, RemoveRuleWith(ctx, c, "sb", "api.example.com",
		WithMethods("GET", "HEAD"), WithPath("/v1/**"), WithQuery("team=x")))
	require.NoError(t, AllowRule(ctx, c, "", []string{"plain.example.com"}))

	data, err := os.ReadFile(argFile)
	require.NoError(t, err)
	require.Equal(t, []string{
		"policy allow network --sandbox sb api.example.com,cdn.example.com --method GET,HEAD --path /v1/** --query team=x --query env=prod",
		"policy deny network media.example.com --protocol udp",
		"policy allow network ** --protocol tcp --protocol udp",
		"policy rm network --sandbox sb --resource api.example.com --force --method GET,HEAD --path /v1/** --query team=x",
		"policy allow network plain.example.com",
	}, strings.Split(strings.TrimSpace(string(data)), "\n"))
}

// rm has no --protocol, and removing by resource drops every protocol's rule,
// so a protocol option would widen the removal silently.
func TestRemoveRuleWith_RejectsProtocol(t *testing.T) {
	argFile := filepath.Join(t.TempDir(), "args.txt")
	c := recordingClient(t, argFile)
	err := RemoveRuleWith(context.Background(), c, "", "x.example.com", WithProtocols("udp"))
	require.Error(t, err)
	_, statErr := os.Stat(argFile)
	require.True(t, os.IsNotExist(statErr), "the CLI must not run")
}

func TestList_DecodesHTTPAndProtocolFields(t *testing.T) {
	c := stubPolicyClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Rows as sbx v0.51.0 returned them for an HTTP rule and a UDP rule.
		w.Write([]byte(`{"rules":[
			{"actions":["http:request:get","http:request:head"],"applies_to":"sandbox:sb","decision":"allow",
			 "editable":true,"http_targets":[{"host":"api.example.com","path":"/v1/**",
			 "query":[{"name":"team","value_pattern":"x"}],"query_match":"subset"}],"id":"r1",
			 "methods":["GET","HEAD"],"origin":"scoped","resource_type":"http","resources":["api.example.com"],
			 "scope":"sandbox:sb","status":"active"},
			{"actions":["net:connect:udp"],"applies_to":"sandbox:sb","decision":"allow","id":"r2",
			 "resource_type":"network","resources":["udp.example.com"],"scope":"sandbox:sb","status":"active"}
		]}`))
	}))
	rules, err := List(context.Background(), c, "sb")
	require.NoError(t, err)
	require.Len(t, rules, 2)
	require.Equal(t, []string{"http:request:get", "http:request:head"}, rules[0].Actions)
	require.Equal(t, []string{"GET", "HEAD"}, rules[0].Methods)
	require.Equal(t, []HTTPTarget{{
		Host: "api.example.com", Path: "/v1/**",
		Query:      []QueryParam{{Name: "team", ValuePattern: "x"}},
		QueryMatch: "subset",
	}}, rules[0].HTTPTargets)
	require.Equal(t, []string{"net:connect:udp"}, rules[1].Actions)
	require.Empty(t, rules[1].HTTPTargets)
}
