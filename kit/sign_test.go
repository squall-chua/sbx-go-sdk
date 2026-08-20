package kit

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/squall-chua/sbx-go-sdk/client"
	"github.com/stretchr/testify/require"
)

func TestSign_KeyBased(t *testing.T) {
	argFile := filepath.Join(t.TempDir(), "args.txt")
	c := fakeClient(t, argFile, "Signed ./my-kit (directory)", "", 0)

	require.NoError(t, Sign(context.Background(), c, "./my-kit", WithKey("cosign.key")))

	args, _ := os.ReadFile(argFile)
	require.Contains(t, string(args), "kit sign --key cosign.key ./my-kit")
}

func TestSign_KeylessOptions(t *testing.T) {
	argFile := filepath.Join(t.TempDir(), "args.txt")
	c := fakeClient(t, argFile, "Signed", "", 0)

	require.NoError(t, Sign(context.Background(), c, "ghcr.io/org/k:1",
		WithIdentityTokenFile("/run/tok"), WithoutTransparencyLog()))

	args, _ := os.ReadFile(argFile)
	require.Contains(t, string(args), "--identity-token-file /run/tok")
	require.Contains(t, string(args), "--tlog-upload=false")
}

func TestVerify_PassesIdentityFlagsInAStableOrder(t *testing.T) {
	argFile := filepath.Join(t.TempDir(), "args.txt")
	c := fakeClient(t, argFile, "VERIFIED: ./my-kit (directory)", "", 0)

	require.NoError(t, Verify(context.Background(), c, "./my-kit",
		WithCertificateIdentity("me@example.com"),
		WithCertificateOIDCIssuer("https://accounts.google.com"),
		WithoutTransparencyLogCheck(),
	))

	args, _ := os.ReadFile(argFile)
	require.Contains(t, string(args),
		"kit verify --certificate-identity me@example.com --certificate-oidc-issuer https://accounts.google.com --insecure-ignore-tlog ./my-kit")
}

// Captured verbatim from `sbx kit verify` at sbx v0.39.0, against a kit whose
// spec.yaml was edited after signing.
const tamperedStderr = "ERROR: kit signature verification failed: failed to verify signature: could not verify message: invalid signature when validating ASN.1 encoded signature"

func TestVerify_BadSignatureIsItsOwnSentinel(t *testing.T) {
	c := fakeClient(t, "", "", tamperedStderr, 1)

	err := Verify(context.Background(), c, "./my-kit", WithPublicKey("cosign.pub"))
	require.ErrorIs(t, err, client.ErrSignatureInvalid)
	require.Contains(t, err.Error(), "invalid signature")
}

// Being asked to verify wrongly must not read as "this kit is untrustworthy".
// Captured verbatim at sbx v0.39.0.
func TestVerify_MisuseStaysAPlainCLIError(t *testing.T) {
	const misuse = "ERROR: keyless verification requires --certificate-identity or --certificate-identity-regexp (or --key for a key-based signature)"
	c := fakeClient(t, "", "", misuse, 1)

	err := Verify(context.Background(), c, "./my-kit")
	require.Error(t, err)
	require.NotErrorIs(t, err, client.ErrSignatureInvalid)

	var ce *client.CLIError
	require.ErrorAs(t, err, &ce)
}

func TestPush_UnsignedByDefault(t *testing.T) {
	argFile := filepath.Join(t.TempDir(), "args.txt")
	c := fakeClient(t, argFile, "", "", 0)

	require.NoError(t, Push(context.Background(), c, "./my-kit", "ghcr.io/org/k:1"))

	args, _ := os.ReadFile(argFile)
	require.Contains(t, string(args), "kit push ./my-kit ghcr.io/org/k:1")
	require.NotContains(t, string(args), "--sign")
}

func TestPush_WithSignCarriesTheSigningOptions(t *testing.T) {
	argFile := filepath.Join(t.TempDir(), "args.txt")
	c := fakeClient(t, argFile, "", "", 0)

	require.NoError(t, Push(context.Background(), c, "./my-kit", "ghcr.io/org/k:1",
		WithSign(WithKey("cosign.key"), WithoutTransparencyLog())))

	args, _ := os.ReadFile(argFile)
	require.Contains(t, string(args), "--sign --key cosign.key --tlog-upload=false")
}

func TestProvenance_ReturnsTheAttestationText(t *testing.T) {
	c := fakeClient(t, "", "subject: sha256:abc\nUNSIGNED", "", 0)

	out, err := Provenance(context.Background(), c, "ghcr.io/org/k:1")
	require.NoError(t, err)
	require.Contains(t, out, "UNSIGNED")
}
