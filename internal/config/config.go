package config

import (
	"os"
	"strconv"
	"strings"
	"time"
)

// Config holds all application configuration.
type Config struct {
	// Database
	DatabaseURL string

	// NATS
	NatsURL string

	// Redis
	RedisURL string

	// LLM
	LLMProvider string // "glm", "openai_compat", "anthropic"
	LLMEndpoint string
	LLMModel    string
	LLMAPIKey   string

	// Embeddings
	EmbeddingsURL string

	// Relevance
	RelevanceThresholdDefault float64
	RelevanceThresholdMin     float64
	RelevanceThresholdMax     float64
	RelevanceThresholdStep    float64
	SourceBoosts              map[string]float64

	// Briefing
	BriefingSchedule string

	// API Server
	APIPort int
	// Static bearer token auth for personal deployments.
	AuthToken string

	// Rate Limiting (domain -> "requests/period" e.g. "60/min")
	RateLimits map[string]string

	// General
	LogLevel  string
	UserAgent string

	// Profile recalculation
	ProfileRecalcTrigger string
	ProfileRecalcEvery   time.Duration
}

// Load reads configuration from environment variables.
func Load() *Config {
	cfg := &Config{
		DatabaseURL:               getEnv("DATABASE_URL", "postgres://flux:flux@localhost:5432/flux?sslmode=disable"),
		NatsURL:                   getEnv("NATS_URL", "nats://localhost:4222"),
		RedisURL:                  getEnv("REDIS_URL", "redis://localhost:6379/0"),
		LLMProvider:               getEnv("LLM_PROVIDER", "glm"),
		LLMEndpoint:               getEnv("LLM_ENDPOINT", "https://open.bigmodel.cn/api/coding/paas/v4"),
		LLMModel:                  getEnv("LLM_MODEL", "glm-4.7"),
		LLMAPIKey:                 getEnv("LLM_API_KEY", ""),
		EmbeddingsURL:             getEnv("EMBEDDINGS_URL", "http://embeddings-svc:8000"),
		RelevanceThresholdDefault: getEnvFloat("RELEVANCE_THRESHOLD_DEFAULT", 0.30),
		RelevanceThresholdMin:     getEnvFloat("RELEVANCE_THRESHOLD_MIN", 0.15),
		RelevanceThresholdMax:     getEnvFloat("RELEVANCE_THRESHOLD_MAX", 0.60),
		RelevanceThresholdStep:    getEnvFloat("RELEVANCE_THRESHOLD_STEP", 0.05),
		BriefingSchedule:          getEnv("BRIEFING_SCHEDULE", "0 3 * * *"),
		APIPort:                   getEnvInt("API_PORT", 8080),
		AuthToken:                 strings.TrimSpace(getEnv("AUTH_TOKEN", "")),
		LogLevel:                  getEnv("LOG_LEVEL", "info"),
		UserAgent:                 getEnv("USER_AGENT", "Flux/1.0 (+https://github.com/zyrak/flux)"),
		ProfileRecalcTrigger:      strings.ToLower(strings.TrimSpace(getEnv("PROFILE_RECALC_TRIGGER", "immediate"))),
		ProfileRecalcEvery:        getEnvDuration("PROFILE_RECALC_EVERY", time.Hour),
	}

	cfg.RateLimits = parseRateLimits(getEnv("RATE_LIMITS", "reddit.com=60/min,oauth.reddit.com=60/min,hacker-news.firebaseio.com=30/min,api.github.com=5000/hour,default=10/min"))
	cfg.SourceBoosts = parseFloatMap(getEnv("SOURCE_BOOSTS", ""))

	return cfg
}

func getEnv(key, fallback string) string {
	if val, ok := os.LookupEnv(key); ok {
		return val
	}
	return fallback
}

func getEnvInt(key string, fallback int) int {
	if val, ok := os.LookupEnv(key); ok {
		if i, err := strconv.Atoi(val); err == nil {
			return i
		}
	}
	return fallback
}

func getEnvFloat(key string, fallback float64) float64 {
	if val, ok := os.LookupEnv(key); ok {
		if f, err := strconv.ParseFloat(val, 64); err == nil {
			return f
		}
	}
	return fallback
}

func getEnvDuration(key string, fallback time.Duration) time.Duration {
	if val, ok := os.LookupEnv(key); ok {
		if d, err := time.ParseDuration(strings.TrimSpace(val)); err == nil && d > 0 {
			return d
		}
	}
	return fallback
}

// parseRateLimits parses "domain1=rate1,domain2=rate2" into a map.
func parseRateLimits(s string) map[string]string {
	limits := make(map[string]string)
	for _, pair := range strings.Split(s, ",") {
		parts := strings.SplitN(strings.TrimSpace(pair), "=", 2)
		if len(parts) == 2 {
			limits[strings.TrimSpace(parts[0])] = strings.TrimSpace(parts[1])
		}
	}
	return limits
}

func parseFloatMap(s string) map[string]float64 {
	out := make(map[string]float64)
	for _, pair := range strings.Split(s, ",") {
		parts := strings.SplitN(strings.TrimSpace(pair), "=", 2)
		if len(parts) != 2 {
			continue
		}
		key := strings.TrimSpace(parts[0])
		if key == "" {
			continue
		}
		value, err := strconv.ParseFloat(strings.TrimSpace(parts[1]), 64)
		if err != nil {
			continue
		}
		out[strings.ToLower(key)] = value
	}
	return out
}
