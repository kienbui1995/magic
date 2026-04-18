package auth

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestLooksLikeJWT(t *testing.T) {
	cases := []struct {
		in   string
		want bool
	}{
		{"eyJhbGciOiJSUzI1NiJ9.eyJzdWIiOiJhYmMifQ.sig", true},
		{"mct_abcdef", false},
		{"plain-api-key-1234567890abcdef", false},
		{"ey.no.third", true}, // shape match, verify will fail
		{"eyJ.two", false},
		{"", false},
	}
	for _, c := range cases {
		if got := LooksLikeJWT(c.in); got != c.want {
			t.Errorf("LooksLikeJWT(%q) = %v, want %v", c.in, got, c.want)
		}
	}
}

func TestClaimsRoundtrip(t *testing.T) {
	c := &Claims{Subject: "user@example.com", OrgID: "org_1", Roles: []string{"admin"}}
	ctx := WithClaims(context.Background(), c)
	got := ClaimsFromContext(ctx)
	if got == nil || got.Subject != "user@example.com" || got.OrgID != "org_1" {
		t.Fatalf("roundtrip failed: %#v", got)
	}
	if ClaimsFromContext(context.Background()) != nil {
		t.Fatal("expected nil for empty context")
	}
}

func TestOIDCMiddleware_NilPassthrough(t *testing.T) {
	// With a nil verifier, the middleware must be a no-op so existing
	// deployments keep working.
	called := false
	h := OIDCMiddleware(nil)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	}))
	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("Authorization", "Bearer some-api-key")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if !called {
		t.Fatal("expected next handler to be called when verifier is nil")
	}
	if rec.Code != 200 {
		t.Fatalf("want 200, got %d", rec.Code)
	}
}

func TestOIDCMiddleware_NonJWTPassthrough(t *testing.T) {
	// Non-JWT bearer (e.g. MAGIC_API_KEY) must fall through to the next
	// middleware, even with OIDC configured.
	v := &OIDCVerifier{issuer: "https://example.com", audience: "client"}
	// verifier field left nil; middleware should never call Verify
	// because the token doesn't look like a JWT.
	called := false
	h := OIDCMiddleware(v)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		if IsJWTAuthed(r.Context()) {
			t.Error("should not be marked JWT-authed for API key")
		}
	}))
	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("Authorization", "Bearer mct_abcdef1234567890")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if !called {
		t.Fatal("expected next handler to be called for non-JWT")
	}
}

func TestOIDCMiddleware_InvalidJWT(t *testing.T) {
	// A JWT-shaped token with a nil internal verifier should be rejected
	// (treated as invalid) rather than falling through to API-key auth.
	v := &OIDCVerifier{issuer: "https://example.com", audience: "client"}
	h := OIDCMiddleware(v)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("next should not run for invalid JWT")
	}))
	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("Authorization", "Bearer eyJhbGciOiJIUzI1NiJ9.eyJzdWIiOiJ4In0.sig")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("want 401 for invalid JWT, got %d", rec.Code)
	}
}

func TestNewOIDCVerifier_Validation(t *testing.T) {
	ctx := context.Background()
	if _, err := NewOIDCVerifier(ctx, "", "cid", ""); err == nil {
		t.Error("expected error for empty issuer")
	}
	if _, err := NewOIDCVerifier(ctx, "https://x", "", ""); err == nil {
		t.Error("expected error for missing client_id and audience")
	}
}
