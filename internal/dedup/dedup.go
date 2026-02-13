package dedup

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"net/url"
	"sort"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
)

const (
	// TTL for dedup entries in Redis.
	dedupTTL = 7 * 24 * time.Hour // 7 days
	// Redis key prefix.
	keyPrefix = "flux:dedup:"
)

// Tracking parameters to strip from URLs before hashing.
var trackingParams = map[string]bool{
	"utm_source": true, "utm_medium": true, "utm_campaign": true,
	"utm_term": true, "utm_content": true, "utm_id": true,
	"fbclid": true, "gclid": true, "dclid": true,
	"mc_cid": true, "mc_eid": true,
	"ref": true, "source": true,
	"_ga": true, "_gl": true,
}

// Checker provides URL deduplication using Redis.
type Checker struct {
	rdb *redis.Client
}

// NewChecker creates a new dedup checker.
func NewChecker(rdb *redis.Client) *Checker {
	return &Checker{rdb: rdb}
}

// IsNew returns true if this URL has not been seen before.
func (c *Checker) IsNew(ctx context.Context, rawURL string) (bool, error) {
	hash := HashURL(rawURL)
	key := keyPrefix + hash

	// SETNX: set only if not exists, with TTL
	set, err := c.rdb.SetNX(ctx, key, "1", dedupTTL).Result()
	if err != nil {
		return false, err
	}
	return set, nil // true = was new (key was set), false = already existed
}

// MarkSeen marks a URL as seen without checking.
func (c *Checker) MarkSeen(ctx context.Context, rawURL string) error {
	hash := HashURL(rawURL)
	key := keyPrefix + hash
	return c.rdb.Set(ctx, key, "1", dedupTTL).Err()
}

// HashURL normalizes a URL and returns its SHA-256 hash.
func HashURL(rawURL string) string {
	normalized := NormalizeURL(rawURL)
	h := sha256.Sum256([]byte(normalized))
	return hex.EncodeToString(h[:])
}

// NormalizeURL removes tracking parameters, normalizes www, lowercases scheme/host,
// removes trailing slashes, and sorts query params for consistent hashing.
func NormalizeURL(rawURL string) string {
	rawURL = strings.TrimSpace(rawURL)

	u, err := url.Parse(rawURL)
	if err != nil {
		return rawURL // fallback to raw if unparseable
	}

	// Lowercase scheme and host
	u.Scheme = strings.ToLower(u.Scheme)
	u.Host = strings.ToLower(u.Host)

	// Remove www. prefix
	u.Host = strings.TrimPrefix(u.Host, "www.")

	// Remove fragment
	u.Fragment = ""

	// Remove trailing slash (except for root path)
	if u.Path != "/" {
		u.Path = strings.TrimRight(u.Path, "/")
	}

	// Clean query params: remove tracking, sort remaining
	if u.RawQuery != "" {
		params := u.Query()
		cleaned := url.Values{}
		for k, v := range params {
			if !trackingParams[strings.ToLower(k)] {
				cleaned[k] = v
			}
		}

		// Sort keys for consistent hashing
		keys := make([]string, 0, len(cleaned))
		for k := range cleaned {
			keys = append(keys, k)
		}
		sort.Strings(keys)

		sorted := url.Values{}
		for _, k := range keys {
			sorted[k] = cleaned[k]
		}
		u.RawQuery = sorted.Encode()
	}

	return u.String()
}
