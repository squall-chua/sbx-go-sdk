package kit

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/squall-chua/sbx-go-sdk/client"
)

// Signing and verification wrap `sbx kit sign` / `verify` / `provenance`, added
// in sbx v0.39.0. They are cosign-compatible Sigstore operations.
//
// Two signing modes:
//
//   - Key-based. Pass WithKey to Sign and WithPublicKey to Verify. Fully
//     offline, and nothing is written to a public log.
//   - Keyless (the CLI default). Fulcio issues a short-lived certificate
//     against an OIDC identity and Rekor records the signing event. Verify then
//     needs the identity you accept, via WithCertificateIdentity (or its regexp
//     form) plus WithCertificateOIDCIssuer.
//
// Where the signature lives depends on the reference. A local directory gets a
// kit.sig.bundle file beside its spec.yaml. An OCI reference gets an OCI
// referrer attached to the kit manifest.
//
// Whether a sandbox will accept an unsigned or untrusted kit is a daemon
// setting, not something these calls decide: see kit.requireSignature,
// kit.trustedSigners and kit.ignoreTransparencyLog, all reachable through the
// settings package.

type signConfig struct {
	key           string
	identityToken string
	tokenFile     string
	noTlogUpload  bool
}

// SignOption configures Sign, and WithSign for Push.
type SignOption func(*signConfig)

// WithKey signs with an unencrypted PEM private key instead of keylessly
// (`--key`). Key-based signing is fully offline and never writes to Rekor.
func WithKey(pemPath string) SignOption { return func(c *signConfig) { c.key = pemPath } }

// WithIdentityTokenFile supplies the OIDC identity token for keyless signing
// from a file (`--identity-token-file`).
//
// Prefer this over WithIdentityToken: an argument vector is readable by other
// local users and is recorded in shell history.
func WithIdentityTokenFile(path string) SignOption {
	return func(c *signConfig) { c.tokenFile = path }
}

// WithIdentityToken supplies the OIDC identity token for keyless signing
// directly (`--identity-token`).
//
// The token lands in the argument vector, where other local users can read it.
// WithIdentityTokenFile keeps it out. Note that upstream deliberately never
// reads this token from an environment variable, so nothing that can set one
// can choose the signing identity.
//
// Neither is needed in CI: the token is minted automatically for GitHub
// Actions, Buildkite, GCP, SPIFFE, and a projected service-account token on
// disk. Without any of those, and with no token given, the CLI falls back to an
// interactive browser login, which will hang a non-interactive caller.
func WithIdentityToken(token string) SignOption {
	return func(c *signConfig) { c.identityToken = token }
}

// WithoutTransparencyLog keeps a keyless signature out of the public Rekor
// transparency log (`--tlog-upload=false`), for a private kit whose existence
// must not be published.
//
// It requires the signing config to provide a timestamp authority, or the
// signature stops verifying once the short-lived certificate expires. Verifying
// such a signature needs WithoutTransparencyLogCheck. No effect on key-based
// signing, which never uploads to Rekor — for fully offline private signing,
// WithKey is the simpler answer.
func WithoutTransparencyLog() SignOption { return func(c *signConfig) { c.noTlogUpload = true } }

func (c *signConfig) args() []string {
	var args []string
	if c.key != "" {
		args = append(args, "--key", c.key)
	}
	if c.tokenFile != "" {
		args = append(args, "--identity-token-file", c.tokenFile)
	}
	if c.identityToken != "" {
		args = append(args, "--identity-token", c.identityToken)
	}
	if c.noTlogUpload {
		args = append(args, "--tlog-upload=false")
	}
	return args
}

// Sign signs a kit artifact (`sbx kit sign REFERENCE`).
//
// ref may be a local directory or an OCI reference; the signature is written
// beside the spec or attached as an OCI referrer accordingly.
//
// Key-based signing of a local directory is verified live. The keyless and OCI
// paths are not: one needs an OIDC identity provider, the other a registry.
func Sign(ctx context.Context, c *client.Client, ref string, opts ...SignOption) error {
	var cfg signConfig
	for _, o := range opts {
		o(&cfg)
	}
	r, err := c.Runner()
	if err != nil {
		return err
	}
	args := append([]string{"kit", "sign"}, cfg.args()...)
	_, err = r.Capture(ctx, nil, append(args, ref)...)
	return err
}

type verifyConfig struct {
	key            string
	identity       string
	identityRegexp string
	issuer         string
	issuerRegexp   string
	ignoreTlog     bool
}

// VerifyOption configures Verify and Provenance.
type VerifyOption func(*verifyConfig)

// WithPublicKey verifies a key-based signature against a PEM public key
// (`--key`).
func WithPublicKey(pemPath string) VerifyOption { return func(c *verifyConfig) { c.key = pemPath } }

// WithCertificateIdentity accepts exactly one keyless signer identity, matched
// against the certificate SAN (`--certificate-identity`).
//
// Pair it with WithCertificateOIDCIssuer. Verifying keylessly without an
// identity is refused by the CLI — a signature is only meaningful against the
// identity you decided to trust.
func WithCertificateIdentity(id string) VerifyOption {
	return func(c *verifyConfig) { c.identity = id }
}

// WithCertificateIdentityRegexp accepts any keyless signer identity matching a
// pattern (`--certificate-identity-regexp`), e.g. every signer in one domain.
//
// A loose pattern widens who can sign for you; keep it anchored.
func WithCertificateIdentityRegexp(re string) VerifyOption {
	return func(c *verifyConfig) { c.identityRegexp = re }
}

// WithCertificateOIDCIssuer requires the keyless identity to have been issued
// by exactly this OIDC issuer (`--certificate-oidc-issuer`).
//
// An identity is only unique within its issuer, so pinning the issuer is what
// stops a different provider from vouching for the same-looking address.
func WithCertificateOIDCIssuer(url string) VerifyOption {
	return func(c *verifyConfig) { c.issuer = url }
}

// WithCertificateOIDCIssuerRegexp is the pattern form of
// WithCertificateOIDCIssuer (`--certificate-oidc-issuer-regexp`).
func WithCertificateOIDCIssuerRegexp(re string) VerifyOption {
	return func(c *verifyConfig) { c.issuerRegexp = re }
}

// WithoutTransparencyLogCheck drops the requirement for a Rekor transparency-log
// entry (`--insecure-ignore-tlog`), relying on the timestamp-authority timestamp
// instead.
//
// It exists for signatures deliberately made with WithoutTransparencyLog. It
// weakens the guarantee, hence upstream's "insecure" in the flag name: without
// the log there is no independent record that the signing happened when it
// claims to. No effect on key-based verification.
func WithoutTransparencyLogCheck() VerifyOption {
	return func(c *verifyConfig) { c.ignoreTlog = true }
}

func (c *verifyConfig) args() []string {
	var args []string
	// Ordered deliberately: a map range would reorder the flags between runs.
	for _, f := range []struct{ flag, value string }{
		{"--key", c.key},
		{"--certificate-identity", c.identity},
		{"--certificate-identity-regexp", c.identityRegexp},
		{"--certificate-oidc-issuer", c.issuer},
		{"--certificate-oidc-issuer-regexp", c.issuerRegexp},
	} {
		if f.value != "" {
			args = append(args, f.flag, f.value)
		}
	}
	if c.ignoreTlog {
		args = append(args, "--insecure-ignore-tlog")
	}
	return args
}

// signatureFailure is the CLI's own prefix for a signature that did not check
// out, as distinct from being asked to verify wrongly.
const signatureFailure = "kit signature verification failed"

// classifyVerify turns a genuine verification failure into ErrSignatureInvalid
// and leaves every other failure — bad flags, missing file — as it was.
func classifyVerify(err error) error {
	var ce *client.CLIError
	if errors.As(err, &ce) && strings.Contains(ce.Stderr, signatureFailure) {
		return fmt.Errorf("%w: %s", client.ErrSignatureInvalid, strings.TrimSpace(ce.Stderr))
	}
	return err
}

// Verify checks a kit artifact's signature (`sbx kit verify REFERENCE`) and
// returns nil when it checks out.
//
// A signature that does not check out returns client.ErrSignatureInvalid —
// branch on that rather than on any error, since being asked to verify wrongly
// (keyless with no identity, a missing file) stays a plain *client.CLIError.
//
// Pass WithPublicKey for a key-based signature. For a keyless one, name the
// identity you accept with WithCertificateIdentity or its regexp form, plus the
// issuer. Verifying keylessly with neither is refused by the CLI.
//
// ref may be a local directory (its kit.sig.bundle sidecar is checked), a git
// reference (cloned, then its committed sidecar is checked), or an OCI
// reference (its attached referrers are checked). The key-based local-directory
// path is verified live, including that tampering with the spec afterwards
// makes verification fail. The classification of a failure on the OCI path is
// unverified, so a failure there may surface as a plain *client.CLIError rather
// than the sentinel — an error either way, never a false pass.
func Verify(ctx context.Context, c *client.Client, ref string, opts ...VerifyOption) error {
	var cfg verifyConfig
	for _, o := range opts {
		o(&cfg)
	}
	r, err := c.Runner()
	if err != nil {
		return err
	}
	args := append([]string{"kit", "verify"}, cfg.args()...)
	if _, err := r.Capture(ctx, nil, append(args, ref)...); err != nil {
		return classifyVerify(err)
	}
	return nil
}

// Provenance returns the SLSA provenance attestation attached to an OCI kit
// (`sbx kit provenance REFERENCE`), as the CLI prints it.
//
// Every `kit push` attaches one as an OCI referrer, recording the kit's content
// digests, the sandbox image its spec declares, and the source git commit it was
// pushed from. Push without WithSign leaves it unsigned, and the CLI marks such
// an attestation UNSIGNED when printing it — anyone with push access to the
// repository could have written it, so treat it as a claim, not evidence.
//
// Passing verification options checks the attestation before printing it; only
// one that verifies AND whose subject matches the kit's own digest is reported
// as VERIFIED. A failure to verify returns client.ErrSignatureInvalid on the
// same terms as Verify.
//
// The output is returned raw rather than parsed: it has no --json flag, and no
// unsigned-versus-verified example was reachable to model it from. Unverified —
// provenance only exists on a pushed OCI kit, and no registry was reachable.
func Provenance(ctx context.Context, c *client.Client, ref string, opts ...VerifyOption) (string, error) {
	var cfg verifyConfig
	for _, o := range opts {
		o(&cfg)
	}
	r, err := c.Runner()
	if err != nil {
		return "", err
	}
	args := append([]string{"kit", "provenance"}, cfg.args()...)
	out, err := r.Capture(ctx, nil, append(args, ref)...)
	if err != nil {
		return "", classifyVerify(err)
	}
	return out, nil
}
