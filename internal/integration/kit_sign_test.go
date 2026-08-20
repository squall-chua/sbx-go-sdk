//go:build integration

package integration

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/squall-chua/sbx-go-sdk/client"
	"github.com/squall-chua/sbx-go-sdk/kit"
	"github.com/stretchr/testify/require"
)

// signingKeys writes an unencrypted P-256 key pair with openssl, which is what
// cosign-compatible key-based signing accepts. Skips if openssl is absent.
func signingKeys(t *testing.T) (priv, pub string) {
	t.Helper()
	if _, err := exec.LookPath("openssl"); err != nil {
		t.Skip("openssl not on PATH; key-based signing cannot be exercised")
	}
	dir := t.TempDir()
	priv = filepath.Join(dir, "cosign.key")
	pub = filepath.Join(dir, "cosign.pub")

	require.NoError(t, exec.Command("openssl",
		"ecparam", "-genkey", "-name", "prime256v1", "-noout", "-out", priv).Run())
	require.NoError(t, exec.Command("openssl",
		"ec", "-in", priv, "-pubout", "-out", pub).Run())
	return priv, pub
}

// The key-based local-directory path is the only signing path reachable
// without an OIDC provider or a registry, so it is the one pinned live: a
// signature verifies, and stops verifying the moment the kit changes.
func TestSmoke_KitSignVerifyRoundTrip(t *testing.T) {
	ctx := context.Background()
	c, err := client.New(ctx, client.WithAutoStart())
	require.NoError(t, err)

	priv, pub := signingKeys(t)
	dir := fixtureKit(t)

	require.NoError(t, kit.Sign(ctx, c, dir, kit.WithKey(priv)))
	require.FileExists(t, filepath.Join(dir, "kit.sig.bundle"),
		"a signed directory gets its bundle beside spec.yaml")

	require.NoError(t, kit.Verify(ctx, c, dir, kit.WithPublicKey(pub)))

	// Changing the kit after signing must invalidate it, or the signature is
	// covering nothing that matters.
	spec := filepath.Join(dir, "spec.yaml")
	body, err := os.ReadFile(spec)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(spec, append(body, []byte("\n# tampered\n")...), 0o644))

	err = kit.Verify(ctx, c, dir, kit.WithPublicKey(pub))
	require.ErrorIs(t, err, client.ErrSignatureInvalid)
}

// Verifying keylessly with no accepted identity is a misuse, not a verdict on
// the kit, so it must not surface as ErrSignatureInvalid.
func TestSmoke_KitVerifyWithoutAnIdentityIsMisuse(t *testing.T) {
	ctx := context.Background()
	c, err := client.New(ctx, client.WithAutoStart())
	require.NoError(t, err)

	priv, _ := signingKeys(t)
	dir := fixtureKit(t)
	require.NoError(t, kit.Sign(ctx, c, dir, kit.WithKey(priv)))

	err = kit.Verify(ctx, c, dir)
	require.Error(t, err)
	require.NotErrorIs(t, err, client.ErrSignatureInvalid)
}
