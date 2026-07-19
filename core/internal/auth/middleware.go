package auth

import (
	"context"
	"net/http"
	"strings"
)

type contextKey string

const ctxKeyClaims contextKey = "oidc_claims"

// ClaimsFromContext retrieves validated OIDC Claims from the request
// context. Returns nil if the request was not authenticated via JWT (e.g.
// authenticated via API key or worker token).
func ClaimsFromContext(ctx context.Context) *Claims {
	if ctx == nil {
		return nil
	}
	v := ctx.Value(ctxKeyClaims)
	if v == nil {
		return nil
	}
	c, _ := v.(*Claims)
	return c
}

// WithClaims returns a context with the provided claims attached.
func WithClaims(ctx context.Context, c *Claims) context.Context {
	return context.WithValue(ctx, ctxKeyClaims, c)
}

// extractBearer returns the raw bearer token from the Authorization header
// or an empty string if absent / malformed.
func extractBearer(r *http.Request) string {
	h := r.Header.Get("Authorization")
	if h == "" {
		return ""
	}
	parts := strings.SplitN(h, " ", 2)
	if len(parts) != 2 || !strings.EqualFold(parts[0], "bearer") {
		return ""
	}
	return strings.TrimSpace(parts[1])
}

// jwtAuthedMarker marks the request as JWT-authenticated so the downstream
// API-key middleware can short-circuit.
const ctxKeyJWTAuthed contextKey = "jwt_authed"

// IsJWTAuthed reports whether the request was already authenticated by
// the OIDC middleware. Used by authMiddleware to skip API-key checks.
func IsJWTAuthed(ctx context.Context) bool {
	if ctx == nil {
		return false
	}
	v, _ := ctx.Value(ctxKeyJWTAuthed).(bool)
	return v
}

// OIDCMiddleware returns an HTTP middleware that validates JWT bearer
// tokens against the given verifier. Behavior:
//
//   - If v is nil (OIDC not configured) → pass through unchanged.
//   - If the Authorization header is absent or does not look like a JWT
//     → pass through (let the API-key middleware handle it).
//   - If the token is a JWT and verifies → attach Claims to context and
//     mark the request as JWT-authed; the next handlers (including the
//     API-key middleware) will skip their own auth check.
//   - If the token is a JWT but fails verification → return 401
//     immediately. Falling through to API-key would be a misleading
//     error; the client sent a JWT, so tell them it failed.
func OIDCMiddleware(v *OIDCVerifier) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		if v == nil {
			return next
		}
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			raw := extractBearer(r)
			if raw == "" || !LooksLikeJWT(raw) {
				next.ServeHTTP(w, r)
				return
			}
			claims, err := v.Verify(r.Context(), raw)
			if err != nil {
				w.Header().Set("Content-Type", "application/json")
				w.Header().Set("WWW-Authenticate", `Bearer error="invalid_token"`)
				w.WriteHeader(http.StatusUnauthorized)
				_, _ = w.Write([]byte(`{"error":"invalid or expired token"}`))
				return
			}
			ctx := WithClaims(r.Context(), claims)
			ctx = context.WithValue(ctx, ctxKeyJWTAuthed, true)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}
