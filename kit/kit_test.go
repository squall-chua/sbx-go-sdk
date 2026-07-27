package kit

import (
	"context"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/squall-chua/sbx-go-sdk/client"
	"github.com/stretchr/testify/require"
)

// fakeClient returns a client whose fake sbx binary records its args to argFile,
// prints stdout, prints stderr, and exits with code. argFile may be "".
func fakeClient(t *testing.T, argFile, stdout, stderr string, code int) *client.Client {
	t.Helper()
	sock := filepath.Join(t.TempDir(), "d.sock")
	l, err := net.Listen("unix", sock)
	require.NoError(t, err)
	srv := &http.Server{Handler: http.NewServeMux()}
	go srv.Serve(l)
	t.Cleanup(func() { srv.Close() })
	if argFile == "" {
		argFile = filepath.Join(t.TempDir(), "args.txt")
	}
	bin := filepath.Join(t.TempDir(), "sbx")
	script := "#!/bin/sh\n" +
		"printf '%s\\n' \"$*\" >> " + argFile + "\n" +
		"cat <<'SBXOUT'\n" + stdout + "\nSBXOUT\n" +
		"cat >&2 <<'SBXERR'\n" + stderr + "\nSBXERR\n" +
		"exit " + strconv.Itoa(code) + "\n"
	require.NoError(t, os.WriteFile(bin, []byte(script), 0o755))
	c, err := client.New(context.Background(), client.WithSocketPath(sock), client.WithBinaryPath(bin))
	require.NoError(t, err)
	return c
}

const sampleJSON = `{
  "manifest": {
    "schemaVersion": "2",
    "kind": "sandbox",
    "name": "fullkit",
    "version": "0.1.0",
    "template": "alpine:3.20",
    "runOptions": ["--foo"],
    "resources": {"cpu": 2, "memoryMB": 2048}
  },
  "mixins": ["ghcr.io/org/other:1.0"],
  "caps": {"network": {"allow": ["api.example.com"]}},
  "warnings": ["field \"mixins\" is accepted but not yet implemented"]
}`

func TestInspect_PassesJSONFlagAndRef(t *testing.T) {
	argFile := filepath.Join(t.TempDir(), "args.txt")
	c := fakeClient(t, argFile, sampleJSON, "", 0)

	_, err := Inspect(context.Background(), c, "./mykit")
	require.NoError(t, err)

	args, err := os.ReadFile(argFile)
	require.NoError(t, err)
	require.Contains(t, string(args), "kit inspect")
	require.Contains(t, string(args), "--json")
	require.Contains(t, string(args), "./mykit")
}

// Guards ADR 0005: every manifest field is present, strings typed and
// structs raw. An earlier draft hand-picked six fields and dropped
// "template", which real output contains for kind: sandbox kits.
func TestInspect_DecodesTypedStringsAndRawStructs(t *testing.T) {
	c := fakeClient(t, "", sampleJSON, "", 0)

	info, err := Inspect(context.Background(), c, "./mykit")
	require.NoError(t, err)

	require.Equal(t, "2", info.Manifest.SchemaVersion)
	require.Equal(t, "sandbox", info.Manifest.Kind)
	require.Equal(t, "fullkit", info.Manifest.Name)
	require.Equal(t, "alpine:3.20", info.Manifest.Template)
	require.Equal(t, []string{"--foo"}, info.Manifest.RunOptions)
	require.JSONEq(t, `{"cpu":2,"memoryMB":2048}`, string(info.Manifest.Resources))

	require.Equal(t, []string{"ghcr.io/org/other:1.0"}, info.Mixins)
	require.JSONEq(t, `{"network":{"allow":["api.example.com"]}}`, string(info.Caps))
	require.Len(t, info.Warnings, 1)
}

func TestInspect_MalformedJSONIsUnexpectedFormat(t *testing.T) {
	c := fakeClient(t, "", "not json at all", "", 0)

	_, err := Inspect(context.Background(), c, "./mykit")
	require.ErrorIs(t, err, client.ErrUnexpectedFormat)
}

func TestValidate_PassesRefAndSucceedsOnExitZero(t *testing.T) {
	argFile := filepath.Join(t.TempDir(), "args.txt")
	c := fakeClient(t, argFile, "VALID: ./mykit (directory)", "", 0)

	require.NoError(t, Validate(context.Background(), c, "./mykit"))

	args, err := os.ReadFile(argFile)
	require.NoError(t, err)
	require.Contains(t, string(args), "kit validate")
	require.Contains(t, string(args), "./mykit")
}

// `sbx kit validate` exits 1 both for a refused artifact and for a missing
// path. Only the leading "INVALID:" on stderr separates them, so that prefix
// is what the sentinel keys on. See the ledger in ADR 0002.
func TestValidate_InvalidPrefixMapsToErrKitRejected(t *testing.T) {
	c := fakeClient(t, "", "", "INVALID: artifact: spec.yaml is required\nERROR: artifact validation failed", 1)

	err := Validate(context.Background(), c, "./mykit")
	require.ErrorIs(t, err, client.ErrKitRejected)
	require.Contains(t, err.Error(), "spec.yaml is required")
}

func TestValidate_ErrorWithoutInvalidPrefixIsNotErrKitRejected(t *testing.T) {
	c := fakeClient(t, "", "", `ERROR: kit reference "./nope": path does not exist`, 1)

	err := Validate(context.Background(), c, "./mykit")
	require.Error(t, err)
	require.NotErrorIs(t, err, client.ErrKitRejected)
}
