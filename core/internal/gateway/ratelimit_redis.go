package gateway

import (
	"context"
	"fmt"
	"log"
	"strconv"
	"sync/atomic"
	"time"

	"github.com/redis/go-redis/v9"
	"golang.org/x/time/rate"
)

// tokenBucketLua implements an atomic token-bucket refill+consume in Redis.
//
// KEYS[1]              — bucket key (e.g. magic:ratelimit:register:1.2.3.4)
// ARGV[1] rate         — tokens per second (float, may be <1)
// ARGV[2] burst        — max bucket size (integer)
// ARGV[3] now          — current unix time in milliseconds (integer)
// ARGV[4] ttl          — key TTL in seconds (integer)
//
// Returns 1 if a token was consumed (request allowed), 0 if denied.
//
// State stored in a Redis hash:
//
//	tokens     — current token count (float)
//	updated_ms — last refill timestamp in milliseconds
const tokenBucketLua = `
local key = KEYS[1]
local rate = tonumber(ARGV[1])
local burst = tonumber(ARGV[2])
local now_ms = tonumber(ARGV[3])
local ttl = tonumber(ARGV[4])

local data = redis.call('HMGET', key, 'tokens', 'updated_ms')
local tokens = tonumber(data[1])
local updated_ms = tonumber(data[2])

if tokens == nil then
  tokens = burst
  updated_ms = now_ms
end

local elapsed_ms = now_ms - updated_ms
if elapsed_ms < 0 then elapsed_ms = 0 end
local refill = (elapsed_ms / 1000.0) * rate
tokens = math.min(burst, tokens + refill)

local allowed = 0
if tokens >= 1 then
  tokens = tokens - 1
  allowed = 1
end

redis.call('HSET', key, 'tokens', tokens, 'updated_ms', now_ms)
redis.call('EXPIRE', key, ttl)
return allowed
`

// redisLimiter is a distributed token-bucket Limiter backed by Redis.
// It fails open: if Redis is unavailable or returns an error, the request
// is allowed through (a warning is logged, rate-limited to one line per
// ~5s to avoid log floods).
type redisLimiter struct {
	client *redis.Client
	name   string // bucket namespace, e.g. "register"
	rate   rate.Limit
	burst  int
	ttl    time.Duration
	script *redis.Script // initialized once in constructor, thread-safe

	// lastWarnUnix is the unix seconds of the last "redis error, failing open"
	// log line. Used to rate-limit warnings when Redis is down.
	lastWarnUnix atomic.Int64
}

// NewRedisLimiter returns a Limiter that keeps per-key token buckets in Redis.
//
// name is a short namespace used to segregate buckets for different endpoint
// groups (e.g. "register", "heartbeat"). Two limiters with the same name
// would share state.
//
// ttl controls how long unused bucket keys linger in Redis. It is refreshed
// on every access; a value several times the refill interval (e.g. 10m) is
// usually appropriate.
//
// The limiter fails open on Redis errors — callers never block on Redis
// availability. Operators monitor the magic_rate_limit_hits_total metric
// and Redis health separately.
func NewRedisLimiter(client *redis.Client, name string, r rate.Limit, burst int, ttl time.Duration) Limiter {
	if ttl <= 0 {
		ttl = 10 * time.Minute
	}
	return &redisLimiter{
		client: client,
		name:   name,
		rate:   r,
		burst:  burst,
		ttl:    ttl,
		script: redis.NewScript(tokenBucketLua),
	}
}

// Allow consults Redis to decide. On any Redis error, returns true (fail-open).
func (rl *redisLimiter) Allow(ctx context.Context, key string) bool {
	fullKey := fmt.Sprintf("magic:ratelimit:%s:%s", rl.name, key)
	now := time.Now().UnixMilli()
	rateStr := strconv.FormatFloat(float64(rl.rate), 'f', -1, 64)
	ttlSec := int64(rl.ttl / time.Second)
	if ttlSec <= 0 {
		ttlSec = 1
	}
	args := []interface{}{rateStr, rl.burst, now, ttlSec}

	res, err := rl.script.Run(ctx, rl.client, []string{fullKey}, args...).Result()
	if err != nil {
		rl.warnFailOpen(err)
		return true
	}

	n, ok := res.(int64)
	if !ok {
		rl.warnFailOpen(fmt.Errorf("unexpected redis response type %T", res))
		return true
	}
	return n == 1
}

func (rl *redisLimiter) warnFailOpen(err error) {
	now := time.Now().Unix()
	last := rl.lastWarnUnix.Load()
	if now-last < 5 {
		return
	}
	if rl.lastWarnUnix.CompareAndSwap(last, now) {
		log.Printf("rate limiter: redis error on bucket %q, failing open: %v", rl.name, err)
	}
}

