// Package auth provides OIDC/JWT authentication middleware for MagiC,
// complementing the built-in API key and worker-token mechanisms.
//
// When MAGIC_OIDC_ISSUER is configured at startup, the gateway accepts
// bearer tokens in two forms — an opaque API key (existing behavior) or a
// JWT issued by the configured OIDC provider (Okta, Azure AD / Entra,
// Auth0, Google Workspace, Keycloak, ...). Either authentication path is
// sufficient; both are checked in series so existing clients keep working.
//
// Tokens are validated against the issuer's JWKS (fetched and cached by
// coreos/go-oidc). Signature, issuer, audience, and expiry are all
// checked. Extracted claims (sub, email, roles, org_id, ...) are attached
// to the request context for downstream RBAC.
package auth

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
)

// Claims holds the subset of JWT claims MagiC uses for authorization.
// org_id and roles are custom claims that must be mapped in the OIDC
// provider (e.g. via a "Groups" claim or custom attribute). When absent,
// RBAC falls back to path-scoped or worker-token-based authorization.
type Claims struct {
	Subject string   `json:"sub"`
	Email   string   `json:"email,omitempty"`
	Name    string   `json:"name,omitempty"`
	OrgID   string   `json:"org_id,omitempty"`
	Roles   []string `json:"roles,omitempty"`
	Issuer  string   `json:"iss,omitempty"`
	Exp     int64    `json:"exp,omitempty"`
}

// OIDCVerifier wraps go-oidc's IDTokenVerifier with MagiC-specific
// configuration and claim extraction.
type OIDCVerifier struct {
	verifier *oidc.IDTokenVerifier
	issuer   string
	audience string
}

// NewOIDCVerifier performs OIDC discovery against the issuer and returns a
// verifier configured to validate tokens issued for the given clientID /
// audience. Blocks for up to the context's deadline during discovery;
// callers should pass a context with a 10s timeout at startup.
//
// If audience is empty, the clientID is used as the expected audience
// (standard behavior for most providers). Set audience explicitly when
// the provider issues API-style access tokens whose aud ≠ client_id
// (common on Auth0 and Okta custom authorization servers).
func NewOIDCVerifier(ctx context.Context, issuer, clientID, audience string) (*OIDCVerifier, error) {
	if issuer == "" {
		return nil, errors.New("oidc: issuer is required")
	}
	if clientID == "" && audience == "" {
		return nil, errors.New("oidc: client_id or audience is required")
	}
	provider, err := oidc.NewProvider(ctx, issuer)
	if err != nil {
		return nil, fmt.Errorf("oidc: discovery failed for %s: %w", issuer, err)
	}
	aud := audience
	if aud == "" {
		aud = clientID
	}
	cfg := &oidc.Config{
		ClientID:        aud,
		SkipClientIDCheck: false,
		// 60s clock skew tolerance — spec-recommended for distributed systems.
		// go-oidc uses Now() as the reference time for expiry checks. Subtracting
		// 60s simulates the server's clock being 60s behind the IdP, so tokens
		// that expired up to 60s ago are still accepted — the correct direction
		// for clock skew compensation. Adding 60s would do the opposite (reject
		// tokens 60s early), which is wrong.
		Now: func() time.Time { return time.Now().Add(-60 * time.Second) },
	}
	return &OIDCVerifier{
		verifier: provider.Verifier(cfg),
		issuer:   issuer,
		audience: aud,
	}, nil
}

// Issuer returns the configured issuer URL (for logging / diagnostics).
func (v *OIDCVerifier) Issuer() string { return v.issuer }

// Verify parses and validates a raw JWT bearer token. On success returns
// the extracted Claims; on failure returns an error whose message is safe
// to return to clients (it never leaks keys or token contents).
func (v *OIDCVerifier) Verify(ctx context.Context, rawToken string) (*Claims, error) {
	if v == nil || v.verifier == nil {
		return nil, errors.New("oidc: verifier not configured")
	}
	idToken, err := v.verifier.Verify(ctx, rawToken)
	if err != nil {
		return nil, fmt.Errorf("oidc: token verify: %w", err)
	}
	var c Claims
	if err := idToken.Claims(&c); err != nil {
		return nil, fmt.Errorf("oidc: claims decode: %w", err)
	}
	c.Issuer = idToken.Issuer
	c.Subject = idToken.Subject
	if !idToken.Expiry.IsZero() {
		c.Exp = idToken.Expiry.Unix()
	}
	return &c, nil
}

// LooksLikeJWT reports whether a bearer token is shaped like a JWT
// (3 dot-separated segments starting with "ey"). Cheap pre-check to
// decide whether to attempt OIDC verification vs. fall through to the
// API-key path. False negatives are impossible for real JWTs; false
// positives are harmless (verify simply returns an error).
func LooksLikeJWT(token string) bool {
	if !strings.HasPrefix(token, "ey") {
		return false
	}
	parts := strings.Split(token, ".")
	return len(parts) == 3
}
