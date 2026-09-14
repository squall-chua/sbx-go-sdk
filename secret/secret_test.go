package secret

import (
	"context"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"github.com/squall-chua/sbx-go-sdk/client"
	"github.com/stretchr/testify/require"
)

func recordingClient(t *testing.T, argFile string) *client.Client {
	t.Helper()
	sock := filepath.Join(t.TempDir(), "d.sock")
	l, err := net.Listen("unix", sock)
	require.NoError(t, err)
	srv := &http.Server{Handler: http.NewServeMux()}
	go srv.Serve(l)
	t.Cleanup(func() { srv.Close() })
	bin := filepath.Join(t.TempDir(), "sbx")
	// `secret ls --json` gets an empty listing, the pre-flight check SetToken,
	// SetRegistry and Import run; everything else gets plain text.
	script := "#!/bin/sh\nprintf '%s\\n' \"$*\" >> " + argFile + "\n" +
		"case \"$*\" in *--json) echo '{\"secrets\": [], \"custom_secrets\": []}' ;; *) echo SECRET-TEXT ;; esac\nexit 0\n"
	require.NoError(t, os.WriteFile(bin, []byte(script), 0o755))
	c, err := client.New(context.Background(), client.WithSocketPath(sock), client.WithBinaryPath(bin))
	require.NoError(t, err)
	return c
}

func TestSecretOps(t *testing.T) {
	argFile := filepath.Join(t.TempDir(), "args.txt")
	c := recordingClient(t, argFile)
	ctx := context.Background()

	require.NoError(t, SetCustom(ctx, c, "", CustomSecret{Host: "api.example.com", Env: "API_KEY", Value: "sk-123"}))
	require.NoError(t, SetCustom(ctx, c, "", CustomSecret{
		Host: "*.example.com", Hosts: []string{"api.other.io"}, Env: "API_KEY", Value: "sk-456",
	}))
	txt, err := ListRaw(ctx, c, "")
	require.NoError(t, err)
	require.Contains(t, txt, "SECRET-TEXT")
	require.NoError(t, Remove(ctx, c, "mysandbox", "openai"))
	require.NoError(t, RemoveCustom(ctx, c, "", "api.example.com"))
	require.NoError(t, RemoveCustom(ctx, c, "my-sandbox", "api.example.com"))
	got, err := List(ctx, c, "my-sandbox")
	require.NoError(t, err)
	require.Empty(t, got.Stored)

	data, _ := os.ReadFile(argFile)
	lines := string(data)
	// sbx v0.38.0 made global the default and deprecated both "-g" and the bare
	// positional sandbox name, so global emits no scope argument at all and a
	// sandbox scope emits "--sandbox NAME".
	require.Contains(t, lines, "secret set-custom --host api.example.com --env API_KEY --value sk-123")
	require.Contains(t, lines, "secret set-custom --host *.example.com --host api.other.io --env API_KEY --value sk-456")
	require.Contains(t, lines, "secret ls\n", "ListRaw reads the text, not --json")
	require.Contains(t, lines, "secret ls --sandbox my-sandbox --json")
	require.Contains(t, lines, "secret rm --sandbox mysandbox openai -f")
	require.Contains(t, lines, "secret rm --host api.example.com -f")
	require.Contains(t, lines, "secret rm --sandbox my-sandbox --host api.example.com -f")
	require.NotContains(t, lines, "-g ", "the deprecated -g spelling prints a warning into parsed output")
}

// Captured from `sbx secret ls --json` at sbx v0.42.1: every scope word, a
// registry username, a resolver, and a multi-host custom secret.
const lsJSON = `{
  "secrets": [
    {
      "scope": "host-only",
      "type": "registry",
      "name": "probe-hostonly.example.com",
      "secret": "**",
      "username": "u"
    },
    {
      "scope": "global",
      "type": "registry",
      "name": "probe-global.example.com",
      "secret": "**",
      "username": "u"
    },
    {
      "scope": "my-sandbox",
      "type": "service",
      "name": "openai",
      "secret": "testte**"
    }
  ],
  "custom_secrets": [
    {
      "scope": "global",
      "targets": [
        "probe-cmd.example.com"
      ],
      "env": "PROBECMD",
      "placeholder": "sbx-cs-osQVWhw2sJHkftdF",
      "kind": "command",
      "source": "echo v",
      "refresh": "30m"
    },
    {
      "scope": "sdkprobe",
      "targets": [
        "probe-m1.example.com",
        "probe-m2.example.com"
      ],
      "env": "PROBEMULTI",
      "placeholder": "sbx-cs-W7zrjIwhnKNxWxaE",
      "secret": "*"
    }
  ],
  "shadowed_services": [],
  "env_only_count": 0
}`

// ValueMasked and Targets must read exactly as the text table's columns did,
// so callers see no change from the move to --json.
func TestParseSecretList(t *testing.T) {
	got, err := parseSecretList(lsJSON)
	require.NoError(t, err)

	require.Equal(t, []Stored{
		{Scope: HostOnlyScope, Type: "registry", Name: "probe-hostonly.example.com", ValueMasked: "u/**"},
		{Scope: "", Type: "registry", Name: "probe-global.example.com", ValueMasked: "u/**"},
		{Scope: "my-sandbox", Type: "service", Name: "openai", ValueMasked: "testte**"},
	}, got.Stored)

	require.Equal(t, []Custom{
		{Scope: "", Targets: "probe-cmd.example.com", Env: "PROBECMD", Placeholder: "sbx-cs-osQVWhw2sJHkftdF", ValueMasked: "command:echo v (30m)"},
		{Scope: "sdkprobe", Targets: "probe-m1.example.com, probe-m2.example.com", Env: "PROBEMULTI", Placeholder: "sbx-cs-W7zrjIwhnKNxWxaE", ValueMasked: "*"},
	}, got.Custom)
}

// A scope holding nothing still prints both keys.
func TestParseSecretList_Empty(t *testing.T) {
	got, err := parseSecretList(`{"secrets": [], "custom_secrets": [], "shadowed_services": [], "env_only_count": 0}`)
	require.NoError(t, err)
	require.Empty(t, got.Stored)
	require.Empty(t, got.Custom)
}

// Anything but the expected JSON — the old text table, or JSON missing a
// list — must surface client.ErrUnexpectedFormat, never an empty result.
func TestParseSecretList_Drift(t *testing.T) {
	for _, raw := range []string{
		"SCOPE       TYPE     NAME    SECRET\nmy-sandbox  service  openai  testte**\n",
		`No secrets found for scope "zzz".`,
		`{"secrets": []}`,
		`{"custom_secrets": []}`,
	} {
		_, err := parseSecretList(raw)
		require.ErrorIs(t, err, client.ErrUnexpectedFormat, raw)
	}
}
