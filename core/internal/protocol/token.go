package protocol

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
)

const tokenPrefix = "mct_"
const tokenBytes = 32

// GenerateToken creates a new token value and its SHA-256 hash.
// Returns (rawToken, tokenHash). The raw token is shown once and never stored.
func GenerateToken() (raw string, hash string) {
	b := make([]byte, tokenBytes)
	if _, err := rand.Read(b); err != nil {
		panic("crypto/rand failed: " + err.Error())
	}
	raw = tokenPrefix + hex.EncodeToString(b)
	return raw, HashToken(raw)
}

// HashToken computes the SHA-256 hash of a raw token string.
func HashToken(raw string) string {
	h := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(h[:])
}
