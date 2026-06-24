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
	require.NoError(t, os.WriteFile(bin, []byte("#!/bin/sh\nprintf '%s\\n' \"$*\" >> "+argFile+"\necho SECRET-TEXT\nexit 0\n"), 0o755))
	c, err := client.New(context.Background(), client.WithSocketPath(sock), client.WithBinaryPath(bin))
	require.NoError(t, err)
	return c
}

func TestSecretOps(t *testing.T) {
	argFile := filepath.Join(t.TempDir(), "args.txt")
	c := recordingClient(t, argFile)
	ctx := context.Background()

	require.NoError(t, SetCustom(ctx, c, "", CustomSecret{Host: "api.example.com", Env: "API_KEY", Value: "sk-123"}))
	txt, err := ListRaw(ctx, c, "")
	require.NoError(t, err)
	require.Contains(t, txt, "SECRET-TEXT")
	require.NoError(t, Remove(ctx, c, "mysandbox", "openai"))
	require.NoError(t, RemoveCustom(ctx, c, "", "api.example.com"))
	require.NoError(t, RemoveCustom(ctx, c, "my-sandbox", "api.example.com"))

	data, _ := os.ReadFile(argFile)
	lines := string(data)
	require.Contains(t, lines, "secret set-custom -g --host api.example.com --env API_KEY --value sk-123")
	require.Contains(t, lines, "secret ls")
	require.Contains(t, lines, "secret rm mysandbox openai -f")
	require.Contains(t, lines, "secret rm -g --host api.example.com -f")
	require.Contains(t, lines, "secret rm my-sandbox --host api.example.com -f")
}

func TestParseSecretList(t *testing.T) {
	raw := "SCOPE       TYPE     NAME    SECRET\n" +
		"my-sandbox  service  openai  testte**\n" +
		"\n" +
		"CUSTOM SECRETS\n" +
		"SCOPE     TARGET    ENV      PLACEHOLDER  SECRET\n" +
		"(global)  api.x.io  API_KEY  ph-123       sk-***\n"

	got, err := parseSecretList(raw)
	require.NoError(t, err)

	require.Equal(t, []Stored{{
		Scope: "my-sandbox", Type: "service", Name: "openai", ValueMasked: "testte**",
	}}, got.Stored)

	require.Equal(t, []Custom{{
		Scope: "", Target: "api.x.io", Env: "API_KEY", Placeholder: "ph-123", ValueMasked: "sk-***",
	}}, got.Custom)
}

func TestParseSecretList_Empty(t *testing.T) {
	got, err := parseSecretList(`No secrets found for scope "zzz".` + "\n")
	require.NoError(t, err)
	require.Empty(t, got.Stored)
	require.Empty(t, got.Custom)
}

func TestParseSecretList_CustomOnly(t *testing.T) {
	raw := "CUSTOM SECRETS\n" +
		"SCOPE     TARGET    ENV      PLACEHOLDER  SECRET\n" +
		"(global)  api.x.io  API_KEY  ph-123       sk-***\n"

	got, err := parseSecretList(raw)
	require.NoError(t, err)
	require.Empty(t, got.Stored)
	require.Len(t, got.Custom, 1)
	require.Equal(t, "api.x.io", got.Custom[0].Target)
}

func TestParseSecretList_Drift(t *testing.T) {
	raw := "SCOPE       KIND     NAME    SECRET\n" +
		"my-sandbox  service  openai  testte**\n"
	_, err := parseSecretList(raw)
	require.ErrorIs(t, err, client.ErrUnexpectedFormat)
}

func TestParseSecretList_StandardOnly(t *testing.T) {
	// Standard table with no CUSTOM SECRETS section: exercises splitCustomSection's
	// "no label -> all standard" branch and an empty Custom slice.
	raw := "SCOPE       TYPE     NAME    SECRET\n" +
		"my-sandbox  service  openai  testte**\n"
	got, err := parseSecretList(raw)
	require.NoError(t, err)
	require.Len(t, got.Stored, 1)
	require.Equal(t, "openai", got.Stored[0].Name)
	require.Empty(t, got.Custom)
}

func TestParseSecretList_CustomDrift(t *testing.T) {
	// Drift in the custom section (TARGET renamed to HOST) must also surface
	// client.ErrUnexpectedFormat — the standard section is empty here.
	raw := "CUSTOM SECRETS\n" +
		"SCOPE     HOST      ENV      PLACEHOLDER  SECRET\n" +
		"(global)  api.x.io  API_KEY  ph-123       sk-***\n"
	_, err := parseSecretList(raw)
	require.ErrorIs(t, err, client.ErrUnexpectedFormat)
}
