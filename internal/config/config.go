package config

import (
	"os"
	"strconv"
	"strings"
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

	// API Server
	APIPort int

	// Rate Limiting (domain -> "requests/period" e.g. "60/min")
	RateLimits map[string]string

	// General
	LogLevel  string
	UserAgent string
}

// Load reads configuration from environment variables.
func Load() *Config {
	cfg := &Config{
		DatabaseURL: getEnv("DATABASE_URL", "postgres://flux:flux@localhost:5432/flux?sslmode=disable"),
		NatsURL:     getEnv("NATS_URL", "nats://localhost:4222"),
		RedisURL:    getEnv("REDIS_URL", "redis://localhost:6379/0"),
		LLMProvider: getEnv("LLM_PROVIDER", "glm"),
		LLMEndpoint: getEnv("LLM_ENDPOINT", "https://open.bigmodel.cn/api/paas/v4"),
		LLMModel:    getEnv("LLM_MODEL", "glm-4.7"),
		LLMAPIKey:   getEnv("LLM_API_KEY", ""),
		APIPort:     getEnvInt("API_PORT", 8080),
		LogLevel:    getEnv("LOG_LEVEL", "info"),
		UserAgent:   getEnv("USER_AGENT", "Flux/1.0 (+https://github.com/zyrak/flux)"),
	}

	cfg.RateLimits = parseRateLimits(getEnv("RATE_LIMITS", "reddit.com=60/min,hacker-news.firebaseio.com=30/min,api.github.com=83/min,default=10/min"))

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
