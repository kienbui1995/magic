package gateway

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"golang.org/x/time/rate"
)

func TestRateLimitMiddleware_Blocks(t *testing.T) {
	// 1 req/second, burst 1
	ls := newLimiterStore(rate.Every(time.Second), 1)
	handler := rateLimitMiddleware(ls, clientIP)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	// First request should pass
	req := httptest.NewRequest("GET", "/", nil)
	req.RemoteAddr = "1.2.3.4:5678"
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}

	// Second immediate request should be blocked
	rr2 := httptest.NewRecorder()
	handler.ServeHTTP(rr2, req)
	if rr2.Code != http.StatusTooManyRequests {
		t.Fatalf("expected 429, got %d", rr2.Code)
	}
}

func TestRateLimitMiddleware_DisabledByEnv(t *testing.T) {
	t.Setenv("MAGIC_RATE_LIMIT_DISABLE", "true")
	ls := newLimiterStore(rate.Every(time.Hour), 1) // very restrictive
	handler := rateLimitMiddleware(ls, clientIP)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	for i := 0; i < 10; i++ {
		req := httptest.NewRequest("GET", "/", nil)
		req.RemoteAddr = "1.2.3.4:5678"
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)
		if rr.Code != http.StatusOK {
			t.Fatalf("rate limiter should be disabled, but got %d on request %d", rr.Code, i+1)
		}
	}
}

func TestClientIP_ForwardedFor(t *testing.T) {
	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("X-Forwarded-For", "203.0.113.1, 10.0.0.1")
	if ip := clientIP(req); ip != "203.0.113.1" {
		t.Fatalf("expected 203.0.113.1, got %s", ip)
	}
}

func TestRateLimitMiddleware_DifferentKeys(t *testing.T) {
	// Burst of 1 per key
	ls := newLimiterStore(rate.Every(time.Second), 1)
	handler := rateLimitMiddleware(ls, clientIP)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	// Two different IPs should not interfere
	for _, ip := range []string{"1.1.1.1:111", "2.2.2.2:222"} {
		req := httptest.NewRequest("GET", "/", nil)
		req.RemoteAddr = ip
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)
		if rr.Code != http.StatusOK {
			t.Fatalf("IP %s should pass, got %d", ip, rr.Code)
		}
	}
}
