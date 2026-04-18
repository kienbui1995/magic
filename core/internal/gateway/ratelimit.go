package gateway

import (
	"context"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/kienbui1995/magic/core/internal/monitor"
	"golang.org/x/time/rate"
)

// rateLimitingEnabled returns false when MAGIC_RATE_LIMIT_DISABLE=true.
func rateLimitingEnabled() bool {
	return os.Getenv("MAGIC_RATE_LIMIT_DISABLE") != "true"
}

// Limiter checks whether a request identified by key is allowed.
// Implementations must be safe for concurrent use.
//
// Two implementations ship with MagiC:
//   - MemoryLimiter (default): per-process token buckets; fast but each
//     gateway replica counts independently.
//   - RedisLimiter: shared-state token buckets backed by Redis; required
//     for correct per-user limits in multi-instance deployments.
type Limiter interface {
	Allow(ctx context.Context, key string) bool
}

// maxLimiters caps the number of tracked IPs to prevent memory exhaustion
// under DDoS with unique spoofed IPs. Entries for active IPs are preserved;
// the oldest entry is evicted when the cap is hit.
const maxLimiters = 10_000

// memoryLimiter holds per-key token-bucket limiters with LRU-like cleanup.
// Implements the Limiter interface using golang.org/x/time/rate in-process.
type memoryLimiter struct {
	mu       sync.Mutex
	limiters map[string]*entry
	r        rate.Limit // tokens per second
	b        int        // burst
	stopCh   chan struct{}
}

type entry struct {
	limiter  *rate.Limiter
	lastSeen time.Time
}

// NewMemoryLimiter returns an in-process token-bucket limiter.
// It is the default implementation when MAGIC_REDIS_URL is unset.
func NewMemoryLimiter(r rate.Limit, b int) Limiter {
	return newLimiterStore(r, b)
}

func newLimiterStore(r rate.Limit, b int) *memoryLimiter {
	ls := &memoryLimiter{
		limiters: make(map[string]*entry),
		r:        r,
		b:        b,
		stopCh:   make(chan struct{}),
	}
	go ls.cleanup()
	return ls
}

func (ls *memoryLimiter) get(key string) *rate.Limiter {
	ls.mu.Lock()
	defer ls.mu.Unlock()
	e, ok := ls.limiters[key]
	if !ok {
		// Evict oldest entry if we've hit the cap — prevents memory exhaustion
		// under DDoS with many unique IPs.
		if len(ls.limiters) >= maxLimiters {
			var oldest string
			var oldestTime time.Time
			for k, v := range ls.limiters {
				if oldest == "" || v.lastSeen.Before(oldestTime) {
					oldest, oldestTime = k, v.lastSeen
				}
			}
			delete(ls.limiters, oldest)
		}
		e = &entry{limiter: rate.NewLimiter(ls.r, ls.b)}
		ls.limiters[key] = e
	}
	e.lastSeen = time.Now()
	return e.limiter
}

// Allow implements Limiter.
func (ls *memoryLimiter) Allow(_ context.Context, key string) bool {
	return ls.get(key).Allow()
}

// cleanup removes entries not seen in the last 5 minutes.
func (ls *memoryLimiter) cleanup() {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			ls.mu.Lock()
			cutoff := time.Now().Add(-5 * time.Minute)
			for k, e := range ls.limiters {
				if e.lastSeen.Before(cutoff) {
					delete(ls.limiters, k)
				}
			}
			ls.mu.Unlock()
		case <-ls.stopCh:
			return
		}
	}
}

func (ls *memoryLimiter) stop() {
	close(ls.stopCh)
}

// clientIP extracts the real client IP, respecting X-Forwarded-For from trusted proxies.
//
// SECURITY ASSUMPTION: This trusts X-Forwarded-For unconditionally.
// Direct access to port 8080 would allow attackers to spoof the IP and bypass
// per-IP rate limits. In production, ensure the server is only reachable via
// a trusted reverse proxy (e.g. Cloudflare Tunnel, nginx) and port 8080 is
// not exposed directly to the internet.
func clientIP(r *http.Request) string {
	// Only trust X-Forwarded-For when behind a trusted reverse proxy
	if os.Getenv("MAGIC_TRUSTED_PROXY") == "true" {
		if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
			for i, c := range xff {
				if c == ',' {
					return xff[:i]
				}
			}
			return xff
		}
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

// rateLimitMiddleware returns a middleware that limits requests using the given Limiter.
// The key function extracts the rate-limit key from the request (e.g. IP, worker ID).
// On limit exceeded, writes 429 Too Many Requests.
func rateLimitMiddleware(l Limiter, keyFn func(*http.Request) string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !rateLimitingEnabled() {
				next.ServeHTTP(w, r)
				return
			}
			key := keyFn(r)
			if !l.Allow(r.Context(), key) {
				monitor.MetricRateLimitHitsTotal.WithLabelValues(r.URL.Path).Inc()
				writeError(w, http.StatusTooManyRequests, "rate limit exceeded")
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
