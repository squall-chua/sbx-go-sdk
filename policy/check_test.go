package policy

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCheck_AllowedRequestShape(t *testing.T) {
	var body map[string]any
	c := stubPolicyClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, http.MethodPost, r.Method)
		require.Equal(t, "/policy/network/check", r.URL.Path)
		raw, _ := io.ReadAll(r.Body)
		require.NoError(t, json.Unmarshal(raw, &body))
		w.Write([]byte(`{"action":"net:connect:tcp","allowed":true,"context":"global",
			"governance":{"active":false},"resource_type":"net:domain",
			"resource_value":"api.anthropic.com:443","target":"api.anthropic.com:443",
			"type":"network"}`))
	}))

	auth, err := Check(context.Background(), c, "api.anthropic.com:443")
	require.NoError(t, err)

	require.Equal(t, "network", body["type"], `type must be "network"`)
	require.Equal(t, "api.anthropic.com:443", body["target"])
	require.NotContains(t, body, "sandbox_id", "sandbox_id must be omitted when unset")

	require.True(t, auth.Allowed)
	require.Equal(t, "net:connect:tcp", auth.Action)
	require.Equal(t, "global", auth.Context)
	require.Equal(t, "net:domain", auth.ResourceType)
	require.Equal(t, "api.anthropic.com:443", auth.ResourceValue)
	require.False(t, auth.Governance.Active)
	require.Empty(t, auth.DenyKind)
}

func TestCheck_DeniedIncludesReasonAndDenyKind(t *testing.T) {
	c := stubPolicyClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"action":"net:connect:tcp","allowed":false,"context":"global",
			"deny_kind":"implicit","governance":{"active":false},
			"reason":"No matching allow rule (default deny)","resource_type":"net:domain",
			"resource_value":"evil.example.com:443","target":"evil.example.com:443",
			"type":"network"}`))
	}))

	auth, err := Check(context.Background(), c, "evil.example.com:443")
	require.NoError(t, err)
	require.False(t, auth.Allowed)
	require.Equal(t, "implicit", auth.DenyKind)
	require.Equal(t, "No matching allow rule (default deny)", auth.Reason)
}

func TestCheck_WithCheckSandboxSetsSandboxID(t *testing.T) {
	var body map[string]any
	c := stubPolicyClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		require.NoError(t, json.Unmarshal(raw, &body))
		w.Write([]byte(`{"action":"net:connect:tcp","allowed":true,"context":"sandbox:s1",
			"governance":{"active":false},"resource_type":"net:domain",
			"resource_value":"a:443","target":"a:443","type":"network"}`))
	}))

	auth, err := Check(context.Background(), c, "a:443", WithCheckSandbox("s1"))
	require.NoError(t, err)
	require.Equal(t, "s1", body["sandbox_id"])
	require.Equal(t, "sandbox:s1", auth.Context)
}

func TestCheck_EmptyTargetIsRejectedLocally(t *testing.T) {
	c := stubPolicyClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("no request expected for an empty target")
	}))
	_, err := Check(context.Background(), c, "")
	require.Error(t, err)
}

func TestCheck_UnknownSandboxSurfacesDaemonError(t *testing.T) {
	c := stubPolicyClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte(`{"message":"sandbox \"nope\" not found"}`))
	}))
	_, err := Check(context.Background(), c, "a:443", WithCheckSandbox("nope"))
	require.Error(t, err)
}

func TestCheck_GovernanceFieldsDecode(t *testing.T) {
	c := stubPolicyClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"action":"net:connect:tcp","allowed":true,"context":"global",
			"governance":{"active":true,"organization":"acme","organization_unavailable":false,
			"last_synced_status":"ok","last_synced_message":"synced"},
			"resource_type":"net:domain","resource_value":"a:443","target":"a:443","type":"network"}`))
	}))

	auth, err := Check(context.Background(), c, "a:443")
	require.NoError(t, err)
	require.True(t, auth.Governance.Active)
	require.Equal(t, "acme", auth.Governance.Organization)
	require.Equal(t, "ok", auth.Governance.LastSyncedStatus)
	require.Equal(t, "synced", auth.Governance.LastSyncedMessage)
}
