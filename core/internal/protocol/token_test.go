package protocol

import (
	"strings"
	"testing"
	"time"
)

func TestGenerateToken_Format(t *testing.T) {
	raw, _ := GenerateToken()
	if !strings.HasPrefix(raw, "mct_") {
		t.Errorf("token should start with 'mct_', got %q", raw)
	}
	if len(raw) != 68 {
		t.Errorf("token length should be 68, got %d", len(raw))
	}
}

func TestGenerateToken_Unique(t *testing.T) {
	raw1, _ := GenerateToken()
	raw2, _ := GenerateToken()
	if raw1 == raw2 {
		t.Error("two calls to GenerateToken produced identical tokens")
	}
}

func TestHashToken_Deterministic(t *testing.T) {
	raw, _ := GenerateToken()
	h1 := HashToken(raw)
	h2 := HashToken(raw)
	if h1 != h2 {
		t.Errorf("HashToken is not deterministic: %q != %q", h1, h2)
	}
}

func TestHashToken_DifferentFromInput(t *testing.T) {
	raw, _ := GenerateToken()
	h := HashToken(raw)
	if h == raw {
		t.Error("hash should not equal the raw token")
	}
}

func TestWorkerToken_IsValid_Active(t *testing.T) {
	future := time.Now().Add(24 * time.Hour)
	tok := &WorkerToken{
		ExpiresAt: &future,
		RevokedAt: nil,
	}
	if !tok.IsValid() {
		t.Error("token with future expiry and no revocation should be valid")
	}
}

func TestWorkerToken_IsValid_NoExpiry(t *testing.T) {
	tok := &WorkerToken{
		ExpiresAt: nil,
		RevokedAt: nil,
	}
	if !tok.IsValid() {
		t.Error("token with no expiry and no revocation should be valid")
	}
}

func TestWorkerToken_IsValid_Expired(t *testing.T) {
	past := time.Now().Add(-1 * time.Hour)
	tok := &WorkerToken{
		ExpiresAt: &past,
		RevokedAt: nil,
	}
	if tok.IsValid() {
		t.Error("token with past ExpiresAt should not be valid")
	}
}

func TestWorkerToken_IsValid_Revoked(t *testing.T) {
	now := time.Now()
	tok := &WorkerToken{
		ExpiresAt: nil,
		RevokedAt: &now,
	}
	if tok.IsValid() {
		t.Error("token with RevokedAt set should not be valid")
	}
}
