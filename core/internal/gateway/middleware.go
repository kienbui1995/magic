package gateway

import (
	"context"
	"crypto/subtle"
	"net/http"
	"os"
	"strings"

	"github.com/kienbui1995/magic/core/internal/protocol"
	"github.com/kienbui1995/magic/core/internal/rbac"
	"github.com/kienbui1995/magic/core/internal/store"
)

// apiVersionMiddleware sets the X-API-Version response header on every response
// and validates the client-supplied X-API-Version header if present.
//
// Compatibility rules:
//   - If client omits X-API-Version → allow (legacy clients).
//   - If client MAJOR matches server MAJOR → allow.
//   - If client MAJOR differs from server MAJOR → 400 with machine-readable body.
//
// Clients can read the server version from the X-API-Version response header.
func apiVersionMiddleware(next http.Handler) http.Handler {
	serverMajor := majorVersion(protocol.ProtocolVersion)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set(protocol.APIVersionHeader, protocol.ProtocolVersion)
		if requested := r.Header.Get(protocol.APIVersionHeader); requested != "" {
			if majorVersion(requested) != serverMajor {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusBadRequest)
				_, _ = w.Write([]byte(`{"error":"incompatible api version","server_version":"` +
					protocol.ProtocolVersion + `","client_version":"` + requested + `"}`))
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

// majorVersion extracts the MAJOR component from a semver-like string.
// "1.0" → "1", "2.3" → "2", "abc" → "abc" (treated as-is).
func majorVersion(v string) string {
	if i := strings.Index(v, "."); i >= 0 {
		return v[:i]
	}
	return v
}

// contextKey is the type for context keys in this package.
type contextKey string

const ctxKeyWorkerToken contextKey = "worker_token"

// TokenFromContext retrieves the validated WorkerToken from the request context.
// Returns nil if not present.
func TokenFromContext(ctx context.Context) *protocol.WorkerToken {
	v := ctx.Value(ctxKeyWorkerToken)
	if v == nil {
		return nil
	}
	t, _ := v.(*protocol.WorkerToken)
	return t
}

// extractBearerToken extracts the raw token value from "Authorization: Bearer <token>" header.
// Returns empty string if the header is missing or malformed.
func extractBearerToken(r *http.Request) string {
	h := r.Header.Get("Authorization")
	if h == "" {
		return ""
	}
	parts := strings.SplitN(h, " ", 2)
	if len(parts) != 2 || strings.ToLower(parts[0]) != "bearer" {
		return ""
	}
	return strings.TrimSpace(parts[1])
}

// workerAuthMiddleware validates mct_ tokens for worker lifecycle endpoints.
// In dev mode (no tokens stored), all requests pass through without auth.
// Auth rejections are recorded to the audit log via store.AppendAudit.
func workerAuthMiddleware(s store.Store) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Dev mode: no tokens configured, allow all
			if !s.HasAnyWorkerTokens() {
				next.ServeHTTP(w, r)
				return
			}

			reqID := r.Header.Get("X-Request-ID")
			if reqID == "" {
				reqID = w.Header().Get("X-Request-ID")
			}

			raw := extractBearerToken(r)
			if raw == "" {
				s.AppendAudit(&protocol.AuditEntry{ //nolint:errcheck
					ID:        protocol.GenerateID("audit"),
					Action:    "auth.rejected",
					Resource:  r.URL.Path,
					RequestID: reqID,
					Outcome:   "denied",
					Detail:    map[string]any{"reason": "missing token"},
				})
				writeError(w, http.StatusUnauthorized, "worker token required")
				return
			}

			hash := protocol.HashToken(raw)
			token, err := s.GetWorkerTokenByHash(hash)
			if err != nil || !token.IsValid() {
				s.AppendAudit(&protocol.AuditEntry{ //nolint:errcheck
					ID:        protocol.GenerateID("audit"),
					Action:    "auth.rejected",
					Resource:  r.URL.Path,
					RequestID: reqID,
					Outcome:   "denied",
					Detail:    map[string]any{"reason": "invalid or revoked token"},
				})
				writeError(w, http.StatusUnauthorized, "invalid or revoked token")
				return
			}

			ctx := context.WithValue(r.Context(), ctxKeyWorkerToken, token)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

const maxBodySize = 1 << 20 // 1 MB

func authMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Skip admin auth for health, dashboard, and worker lifecycle endpoints.
		// Worker endpoints (/workers/register, /workers/heartbeat) have their own
		// workerAuthMiddleware — they must not require the admin API key.
		workerPaths := r.URL.Path == "/api/v1/workers/register" ||
			r.URL.Path == "/api/v1/workers/heartbeat"
		if r.URL.Path == "/health" || r.URL.Path == "/dashboard" || r.URL.Path == "/metrics" || workerPaths {
			next.ServeHTTP(w, r)
			return
		}

		apiKey := os.Getenv("MAGIC_API_KEY")
		if apiKey == "" {
			// No API key configured — allow all (dev mode)
			next.ServeHTTP(w, r)
			return
		}

		token := r.Header.Get("Authorization")
		if token == "" {
			token = r.Header.Get("X-API-Key")
		}
		bearerToken := "Bearer " + apiKey
		if subtle.ConstantTimeCompare([]byte(token), []byte(bearerToken)) != 1 &&
			subtle.ConstantTimeCompare([]byte(token), []byte(apiKey)) != 1 {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusUnauthorized)
			w.Write([]byte(`{"error": "unauthorized"}`))
			return
		}

		next.ServeHTTP(w, r)
	})
}

func bodySizeMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Body != nil && r.ContentLength > maxBodySize {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusRequestEntityTooLarge)
			w.Write([]byte(`{"error": "request body too large"}`))
			return
		}
		if r.Body != nil {
			r.Body = http.MaxBytesReader(w, r.Body, maxBodySize)
		}
		next.ServeHTTP(w, r)
	})
}

func requestIDMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reqID := r.Header.Get("X-Request-ID")
		if reqID == "" {
			reqID = protocol.GenerateID("req")
		}
		w.Header().Set("X-Request-ID", reqID)
		next.ServeHTTP(w, r)
	})
}

func corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := os.Getenv("MAGIC_CORS_ORIGIN")
		if origin != "" {
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Vary", "Origin")
		}
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, X-API-Key, X-Request-ID")
		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusOK)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func securityHeadersMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Cache-Control", "no-store")
		next.ServeHTTP(w, r)
	})
}

// rbacMiddleware checks RBAC permissions based on the request method.
// Maps HTTP methods to RBAC actions: GET→read, POST/PUT→write, DELETE→delete.
// If no RBAC enforcer is configured (nil), all requests pass through.
func rbacMiddleware(enforcer *rbac.Enforcer) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if enforcer == nil {
				next.ServeHTTP(w, r)
				return
			}

			// Determine org and subject from context
			token := TokenFromContext(r.Context())
			orgID := ""
			subject := ""
			if token != nil {
				orgID = token.OrgID
				subject = token.WorkerID
			}
			// Also check path for org-scoped endpoints
			if pathOrg := r.PathValue("orgID"); pathOrg != "" && orgID == "" {
				orgID = pathOrg
			}

			if orgID == "" {
				next.ServeHTTP(w, r) // dev mode
				return
			}

			action := methodToAction(r.Method)
			if !enforcer.Check(orgID, subject, action) {
				writeError(w, http.StatusForbidden, "insufficient permissions")
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

func methodToAction(method string) string {
	switch method {
	case "GET", "HEAD":
		return rbac.ActionRead
	case "DELETE":
		return rbac.ActionDelete
	default:
		return rbac.ActionWrite
	}
}
