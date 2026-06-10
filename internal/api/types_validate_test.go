//go:build integration

package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net"
	"net/http"
	"os"
	"testing"

	"github.com/stretchr/testify/require"
)

func liveGet(t *testing.T, path string) []byte {
	t.Helper()
	sock := os.Getenv("DOCKER_SANDBOXES_API")
	if sock == "" {
		home, _ := os.UserHomeDir()
		sock = home + "/.local/state/sandboxes/sandboxes/sandboxd/sandboxd.sock"
	}
	hc := &http.Client{Transport: &http.Transport{
		DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
			return (&net.Dialer{}).DialContext(ctx, "unix", sock)
		},
	}}
	resp, err := hc.Get("http://d" + path)
	require.NoError(t, err)
	defer resp.Body.Close()
	var buf bytes.Buffer
	buf.ReadFrom(resp.Body)
	return buf.Bytes()
}

// requireNoUnknownFields round-trips raw JSON through the typed struct and back,
// failing if any key present in raw is missing from the typed re-encoding.
func requireNoUnknownFields(t *testing.T, raw []byte, typed any) {
	require.NoError(t, json.Unmarshal(raw, typed))
	reencoded, err := json.Marshal(typed)
	require.NoError(t, err)
	var a, b map[string]any
	json.Unmarshal(raw, &a)
	json.Unmarshal(reencoded, &b)
	for k := range a {
		_, ok := b[k]
		require.Truef(t, ok, "field %q present in daemon JSON but missing from struct", k)
	}
}

func TestSandboxInfo_NoDrift(t *testing.T) {
	raw := liveGet(t, "/sandbox")
	var arr []SandboxInfo
	requireNoUnknownFields(t, raw, &arr) // arr-level; per-element checked below
	var rawArr []json.RawMessage
	require.NoError(t, json.Unmarshal(raw, &rawArr))
	for _, el := range rawArr {
		var si SandboxInfo
		requireNoUnknownFields(t, el, &si)
	}
}
