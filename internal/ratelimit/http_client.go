package ratelimit

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	log "github.com/sirupsen/logrus"
)

// NewHTTPClient builds an HTTP client that enforces the shared Redis-backed limiter.
func NewHTTPClient(limiter *Limiter, timeout time.Duration) *http.Client {
	return &http.Client{
		Timeout: timeout,
		Transport: &rateLimitedTransport{
			base:    http.DefaultTransport,
			limiter: limiter,
		},
	}
}

type rateLimitedTransport struct {
	base    http.RoundTripper
	limiter *Limiter
}

func (t *rateLimitedTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if req.URL == nil {
		return nil, fmt.Errorf("request URL is nil")
	}

	domain := strings.ToLower(req.URL.Hostname())
	if domain == "" {
		return nil, fmt.Errorf("request domain is empty")
	}

	waitStart := time.Now()
	if err := t.limiter.Wait(req.Context(), domain); err != nil {
		return nil, err
	}

	log.WithFields(log.Fields{
		"domain":  domain,
		"wait_ms": time.Since(waitStart).Milliseconds(),
		"url":     req.URL.String(),
	}).Info("Rate limit wait complete")

	clonedReq := req.Clone(req.Context())
	if clonedReq.Header == nil {
		clonedReq.Header = make(http.Header)
	}
	if clonedReq.Header.Get("User-Agent") == "" {
		clonedReq.Header.Set("User-Agent", t.limiter.UserAgent())
	}

	resp, err := t.base.RoundTrip(clonedReq)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode == http.StatusForbidden {
		t.limiter.RecordError(req.Context(), domain, resp.StatusCode, parseRetryAfter(resp.Header.Get("Retry-After")))
	} else if resp.StatusCode < 500 {
		t.limiter.ResetBackoff(req.Context(), domain)
	}

	return resp, nil
}

func parseRetryAfter(value string) time.Duration {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0
	}

	seconds, err := strconv.Atoi(value)
	if err == nil && seconds > 0 {
		return time.Duration(seconds) * time.Second
	}

	when, err := http.ParseTime(value)
	if err != nil {
		return 0
	}
	if d := time.Until(when); d > 0 {
		return d
	}
	return 0
}
