package llm

import (
	"fmt"

	log "github.com/sirupsen/logrus"
)

// Supported provider names.
const (
	ProviderGLM          = "glm"
	ProviderOpenAICompat = "openai_compat"
	ProviderAnthropic    = "anthropic"
)

// NewAnalyzer creates the appropriate Analyzer implementation based on the provider string.
// Configuration is read from the provided parameters, typically sourced from env vars.
func NewAnalyzer(provider, endpoint, model, apiKey string) (Analyzer, error) {
	switch provider {
	case ProviderGLM:
		log.WithFields(log.Fields{
			"provider": provider,
			"endpoint": endpoint,
			"model":    model,
		}).Info("Initializing GLM analyzer")
		return NewGLMAnalyzer(endpoint, model, apiKey), nil

	case ProviderOpenAICompat:
		log.WithFields(log.Fields{
			"provider": provider,
			"endpoint": endpoint,
			"model":    model,
		}).Info("Initializing OpenAI-compatible analyzer")
		return NewOpenAICompatAnalyzer(endpoint, model, apiKey), nil

	case ProviderAnthropic:
		log.WithFields(log.Fields{
			"provider": provider,
			"endpoint": endpoint,
			"model":    model,
		}).Info("Initializing Anthropic analyzer")
		return NewAnthropicAnalyzer(endpoint, model, apiKey), nil

	default:
		return nil, fmt.Errorf("unknown LLM provider %q: must be one of: %s, %s, %s",
			provider, ProviderGLM, ProviderOpenAICompat, ProviderAnthropic)
	}
}
