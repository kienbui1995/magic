package gateway

import (
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"golang.org/x/time/rate"
)

// newMiniredis returns a real go-redis client wired to an in-process
// miniredis server. The server supports the Lua EVAL commands we use.
func newMiniredis(t *testing.T) (*redis.Client, *miniredis.Miniredis) {
	t.Helper()
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("start miniredis: %v", err)
	}
	t.Cleanup(mr.Close)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	return client, mr
}

func TestRedisLimiter_BurstAllowedThenDenied(t *testing.T) {
	client, _ := newMiniredis(t)
	// 1 token/sec, burst 3 — first 3 calls allowed, 4th denied.
	lim := NewRedisLimiter(client, "test", rate.Every(time.Second), 3, time.Minute)
	ctx := context.Background()

	for i := 0; i < 3; i++ {
		if !lim.Allow(ctx, "user-a") {
			t.Fatalf("call %d should be allowed (burst=3)", i+1)
		}
	}
	if lim.Allow(ctx, "user-a") {
		t.Fatal("4th call should be denied after burst exhausted")
	}
}

func TestRedisLimiter_SeparateKeysIndependent(t *testing.T) {
	client, _ := newMiniredis(t)
	lim := NewRedisLimiter(client, "test", rate.Every(time.Second), 1, time.Minute)
	ctx := context.Background()

	if !lim.Allow(ctx, "a") {
		t.Fatal("first call for user a should pass")
	}
	if !lim.Allow(ctx, "b") {
		t.Fatal("first call for user b should pass (independent bucket)")
	}
	if lim.Allow(ctx, "a") {
		t.Fatal("second call for user a should be denied")
	}
}

func TestRedisLimiter_FailOpenOnRedisDown(t *testing.T) {
	client, mr := newMiniredis(t)
	lim := NewRedisLimiter(client, "test", rate.Every(time.Hour), 1, time.Minute)
	ctx := context.Background()

	// Kill Redis → every subsequent call should be allowed (fail-open).
	mr.Close()

	for i := 0; i < 5; i++ {
		if !lim.Allow(ctx, "user-a") {
			t.Fatalf("call %d must be allowed when redis is down (fail-open), got denied", i+1)
		}
	}
}

func TestRedisLimiter_Refills(t *testing.T) {
	client, mr := newMiniredis(t)
	// 10 tokens/sec, burst 1 → after drain, waiting 150ms refills ~1 token.
	lim := NewRedisLimiter(client, "test", rate.Limit(10), 1, time.Minute)
	ctx := context.Background()

	if !lim.Allow(ctx, "user-a") {
		t.Fatal("first call should be allowed")
	}
	if lim.Allow(ctx, "user-a") {
		t.Fatal("second immediate call should be denied")
	}
	// Advance miniredis server time used for TTLs; for the limiter we rely on
	// real wall clock (tokenBucketLua uses ARGV[3] passed from Go).
	time.Sleep(150 * time.Millisecond)
	_ = mr
	if !lim.Allow(ctx, "user-a") {
		t.Fatal("call after refill window should be allowed again")
	}
}
