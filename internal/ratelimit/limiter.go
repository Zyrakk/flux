package ratelimit

import (
	"context"
	"fmt"
	"math"
	"math/rand"
	"strconv"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
	log "github.com/sirupsen/logrus"
)

// Limiter provides centralized rate limiting backed by Redis.
// All outgoing HTTP requests must pass through this limiter.
type Limiter struct {
	rdb       *redis.Client
	limits    map[string]rateSpec
	userAgent string
}

// rateSpec defines a rate limit: maxRequests per period.
type rateSpec struct {
	MaxRequests int
	Period      time.Duration
}

// Config holds rate limiter configuration.
type Config struct {
	// Limits maps domain -> "requests/period" (e.g. "60/min", "5000/hour")
	Limits    map[string]string
	UserAgent string
}

// Lua script for atomic token bucket check-and-decrement.
// Returns 1 if allowed, 0 if rate limited, along with time-to-wait in ms.
var tokenBucketScript = redis.NewScript(`
local key = KEYS[1]
local max_tokens = tonumber(ARGV[1])
local refill_rate = tonumber(ARGV[2]) -- tokens per second
local now = tonumber(ARGV[3])

local bucket = redis.call('HMGET', key, 'tokens', 'last_refill')
local tokens = tonumber(bucket[1])
local last_refill = tonumber(bucket[2])

if tokens == nil then
    tokens = max_tokens
    last_refill = now
end

-- Refill tokens based on elapsed time
local elapsed = now - last_refill
local new_tokens = elapsed * refill_rate
tokens = math.min(max_tokens, tokens + new_tokens)

if tokens >= 1 then
    tokens = tokens - 1
    redis.call('HMSET', key, 'tokens', tokens, 'last_refill', now)
    redis.call('EXPIRE', key, math.ceil(max_tokens / refill_rate) + 10)
    return {1, 0}
else
    -- Calculate wait time until next token
    local wait_ms = math.ceil((1 - tokens) / refill_rate * 1000)
    redis.call('HMSET', key, 'tokens', tokens, 'last_refill', now)
    redis.call('EXPIRE', key, math.ceil(max_tokens / refill_rate) + 10)
    return {0, wait_ms}
end
`)

// New creates a new rate limiter.
func New(rdb *redis.Client, cfg Config) (*Limiter, error) {
	limits := make(map[string]rateSpec)
	for domain, spec := range cfg.Limits {
		rs, err := parseRateSpec(spec)
		if err != nil {
			return nil, fmt.Errorf("parsing rate spec for %q: %w", domain, err)
		}
		limits[domain] = rs
	}

	userAgent := cfg.UserAgent
	if userAgent == "" {
		userAgent = "Flux/1.0 (+https://github.com/zyrak/flux)"
	}

	return &Limiter{rdb: rdb, limits: limits, userAgent: userAgent}, nil
}

// Wait blocks until a request to the given domain is allowed, or ctx expires.
// It also applies jitter between requests to the same domain.
func (l *Limiter) Wait(ctx context.Context, domain string) error {
	// Check if domain is in backoff
	if err := l.checkBackoff(ctx, domain); err != nil {
		return err
	}

	spec := l.getSpec(domain)
	key := "flux:ratelimit:" + domain

	refillRate := float64(spec.MaxRequests) / spec.Period.Seconds()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		now := float64(time.Now().UnixMilli()) / 1000.0
		result, err := tokenBucketScript.Run(ctx, l.rdb, []string{key},
			spec.MaxRequests, refillRate, now).Int64Slice()
		if err != nil {
			return fmt.Errorf("executing rate limit script: %w", err)
		}

		if result[0] == 1 {
			// Allowed — apply jitter (1-3s) for content fetching domains
			jitter := time.Duration(1000+rand.Intn(2000)) * time.Millisecond
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(jitter):
			}
			return nil
		}

		// Rate limited — wait
		waitMs := result[1]
		log.WithFields(log.Fields{
			"domain": domain,
			"wait":   fmt.Sprintf("%dms", waitMs),
		}).Debug("Rate limited, waiting")

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(time.Duration(waitMs) * time.Millisecond):
		}
	}
}

// Allow performs a non-blocking check. Returns true if a request is allowed.
func (l *Limiter) Allow(ctx context.Context, domain string) bool {
	// Check backoff first
	backoffKey := "flux:backoff:" + domain
	exists, _ := l.rdb.Exists(ctx, backoffKey).Result()
	if exists > 0 {
		return false
	}

	spec := l.getSpec(domain)
	key := "flux:ratelimit:" + domain
	refillRate := float64(spec.MaxRequests) / spec.Period.Seconds()
	now := float64(time.Now().UnixMilli()) / 1000.0

	result, err := tokenBucketScript.Run(ctx, l.rdb, []string{key},
		spec.MaxRequests, refillRate, now).Int64Slice()
	if err != nil {
		return false
	}
	return result[0] == 1
}

// RecordError records a 429/403 response and applies exponential backoff.
func (l *Limiter) RecordError(ctx context.Context, domain string, statusCode int, retryAfter time.Duration) {
	if statusCode != 429 && statusCode != 403 {
		return
	}

	backoffKey := "flux:backoff:" + domain
	countKey := "flux:backoff_count:" + domain

	// Get current backoff count
	count, _ := l.rdb.Incr(ctx, countKey).Result()
	l.rdb.Expire(ctx, countKey, 24*time.Hour)

	// Calculate backoff duration
	var duration time.Duration
	if retryAfter > 0 {
		duration = retryAfter
	} else {
		// Exponential backoff: min(2^count * 30s, 1h)
		seconds := math.Min(math.Pow(2, float64(count))*30, 3600)
		duration = time.Duration(seconds) * time.Second
	}

	l.rdb.Set(ctx, backoffKey, "1", duration)

	log.WithFields(log.Fields{
		"domain":   domain,
		"status":   statusCode,
		"backoff":  duration,
		"attempts": count,
	}).Warn("Domain in backoff after error")
}

// ResetBackoff clears the backoff state for a domain (e.g., after a successful request).
func (l *Limiter) ResetBackoff(ctx context.Context, domain string) {
	l.rdb.Del(ctx, "flux:backoff:"+domain, "flux:backoff_count:"+domain)
}

// UserAgent returns the configured User-Agent string.
func (l *Limiter) UserAgent() string {
	return l.userAgent
}

// checkBackoff returns an error if the domain is currently in backoff.
func (l *Limiter) checkBackoff(ctx context.Context, domain string) error {
	backoffKey := "flux:backoff:" + domain
	ttl, err := l.rdb.TTL(ctx, backoffKey).Result()
	if err != nil {
		return nil // Redis error — proceed anyway
	}
	if ttl > 0 {
		return fmt.Errorf("domain %s is in backoff for %v", domain, ttl)
	}
	return nil
}

// getSpec returns the rate spec for a domain, falling back to "default".
func (l *Limiter) getSpec(domain string) rateSpec {
	if spec, ok := l.limits[domain]; ok {
		return spec
	}
	if spec, ok := l.limits["default"]; ok {
		return spec
	}
	return rateSpec{MaxRequests: 10, Period: time.Minute} // ultimate fallback
}

// parseRateSpec parses "60/min", "5000/hour", "10/sec" into a rateSpec.
func parseRateSpec(s string) (rateSpec, error) {
	parts := strings.SplitN(s, "/", 2)
	if len(parts) != 2 {
		return rateSpec{}, fmt.Errorf("invalid rate spec %q: expected format 'N/period'", s)
	}

	maxReq, err := strconv.Atoi(strings.TrimSpace(parts[0]))
	if err != nil {
		return rateSpec{}, fmt.Errorf("invalid request count in %q: %w", s, err)
	}

	var period time.Duration
	switch strings.TrimSpace(strings.ToLower(parts[1])) {
	case "sec", "second", "s":
		period = time.Second
	case "min", "minute", "m":
		period = time.Minute
	case "hour", "h":
		period = time.Hour
	default:
		return rateSpec{}, fmt.Errorf("unknown period %q in rate spec", parts[1])
	}

	return rateSpec{MaxRequests: maxReq, Period: period}, nil
}
