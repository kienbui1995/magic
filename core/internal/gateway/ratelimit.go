package gateway

import (
	"net/http"
	"os"
	"sync"
	"time"

	"golang.org/x/time/rate"
)

// rateLimitingEnabled returns false when MAGIC_RATE_LIMIT_DISABLE=true.
func rateLimitingEnabled() bool {
	return os.Getenv("MAGIC_RATE_LIMIT_DISABLE") != "true"
}

// limiterStore holds per-key token-bucket limiters with LRU-like cleanup.
type limiterStore struct {
	mu       sync.Mutex
	limiters map[string]*entry
	r        rate.Limit // tokens per second
	b        int        // burst
}

type entry struct {
	limiter  *rate.Limiter
	lastSeen time.Time
}

func newLimiterStore(r rate.Limit, b int) *limiterStore {
	ls := &limiterStore{
		limiters: make(map[string]*entry),
		r:        r,
		b:        b,
	}
	go ls.cleanup()
	return ls
}

func (ls *limiterStore) get(key string) *rate.Limiter {
	ls.mu.Lock()
	defer ls.mu.Unlock()
	e, ok := ls.limiters[key]
	if !ok {
		e = &entry{limiter: rate.NewLimiter(ls.r, ls.b)}
		ls.limiters[key] = e
	}
	e.lastSeen = time.Now()
	return e.limiter
}

// cleanup removes entries not seen in the last 5 minutes.
func (ls *limiterStore) cleanup() {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()
	for range ticker.C {
		ls.mu.Lock()
		cutoff := time.Now().Add(-5 * time.Minute)
		for k, e := range ls.limiters {
			if e.lastSeen.Before(cutoff) {
				delete(ls.limiters, k)
			}
		}
		ls.mu.Unlock()
	}
}

// clientIP extracts the real client IP, respecting X-Forwarded-For from trusted proxies.
func clientIP(r *http.Request) string {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		// Take first IP in the chain (closest to client)
		for i, c := range xff {
			if c == ',' {
				return xff[:i]
			}
		}
		return xff
	}
	// Fall back to RemoteAddr (strip port)
	host := r.RemoteAddr
	for i := len(host) - 1; i >= 0; i-- {
		if host[i] == ':' {
			return host[:i]
		}
	}
	return host
}

// rateLimitMiddleware returns a middleware that limits requests using the given store.
// The key function extracts the rate-limit key from the request (e.g. IP, worker ID).
// On limit exceeded, writes 429 Too Many Requests.
func rateLimitMiddleware(ls *limiterStore, keyFn func(*http.Request) string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !rateLimitingEnabled() {
				next.ServeHTTP(w, r)
				return
			}
			key := keyFn(r)
			if !ls.get(key).Allow() {
				writeError(w, http.StatusTooManyRequests, "rate limit exceeded")
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
