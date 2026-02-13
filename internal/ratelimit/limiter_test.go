package ratelimit

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseRateSpec(t *testing.T) {
	tests := []struct {
		input   string
		want    rateSpec
		wantErr bool
	}{
		{"60/min", rateSpec{MaxRequests: 60, Period: time.Minute}, false},
		{"10/sec", rateSpec{MaxRequests: 10, Period: time.Second}, false},
		{"5000/hour", rateSpec{MaxRequests: 5000, Period: time.Hour}, false},
		{"30/m", rateSpec{MaxRequests: 30, Period: time.Minute}, false},
		{"100/h", rateSpec{MaxRequests: 100, Period: time.Hour}, false},
		{"bad", rateSpec{}, true},
		{"abc/min", rateSpec{}, true},
		{"10/unknown", rateSpec{}, true},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got, err := parseRateSpec(tt.input)
			if tt.wantErr {
				assert.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want.MaxRequests, got.MaxRequests)
			assert.Equal(t, tt.want.Period, got.Period)
		})
	}
}

func TestGetSpec(t *testing.T) {
	l := &Limiter{
		limits: map[string]rateSpec{
			"reddit.com":     {MaxRequests: 60, Period: time.Minute},
			"hn.algolia.com": {MaxRequests: 30, Period: time.Minute},
			"default":        {MaxRequests: 10, Period: time.Minute},
		},
	}

	// Exact match
	spec := l.getSpec("reddit.com")
	assert.Equal(t, 60, spec.MaxRequests)

	// Fallback to default
	spec = l.getSpec("unknown.com")
	assert.Equal(t, 10, spec.MaxRequests)
}

func TestUserAgent(t *testing.T) {
	l := &Limiter{userAgent: "Flux/1.0 (+https://github.com/zyrak/flux)"}
	assert.Equal(t, "Flux/1.0 (+https://github.com/zyrak/flux)", l.UserAgent())
}

// Integration tests with real Redis would use testcontainers:
//
// func TestWaitWithRedis(t *testing.T) {
//     // Start Redis container
//     ctx := context.Background()
//     rdb := redis.NewClient(&redis.Options{Addr: "localhost:6379"})
//     defer rdb.Close()
//
//     cfg := Config{
//         Limits: map[string]string{"test.com": "5/sec"},
//     }
//     limiter, err := New(rdb, cfg)
//     require.NoError(t, err)
//
//     // First 5 requests should be fast
//     for i := 0; i < 5; i++ {
//         err := limiter.Wait(ctx, "test.com")
//         require.NoError(t, err)
//     }
//
//     // 6th request should be delayed
//     start := time.Now()
//     err = limiter.Wait(ctx, "test.com")
//     require.NoError(t, err)
//     assert.True(t, time.Since(start) > 100*time.Millisecond)
// }
