package llm

import (
	"context"
	"fmt"
)

// OpenAICompatAnalyzer implements the Analyzer interface for any OpenAI-compatible API.
// Works with: OpenAI, Ollama, vLLM, LiteLLM, Together, Groq, etc.
type OpenAICompatAnalyzer struct {
	base baseClient
}

// NewOpenAICompatAnalyzer creates an OpenAI-compatible analyzer.
func NewOpenAICompatAnalyzer(endpoint, model, apiKey string) *OpenAICompatAnalyzer {
	if endpoint == "" {
		endpoint = "https://api.openai.com/v1"
	}
	if model == "" {
		model = "gpt-4o-mini"
	}
	return &OpenAICompatAnalyzer{
		base: newBaseClient(endpoint, model, apiKey),
	}
}

func (o *OpenAICompatAnalyzer) Provider() string { return "openai_compat" }

func (o *OpenAICompatAnalyzer) Classify(ctx context.Context, articles []ArticleInput) ([]Classification, error) {
	prompt := BuildClassifyPrompt(articles)

	req := ChatRequest{
		Model: o.base.model,
		Messages: []ChatMessage{
			{Role: "system", Content: systemPrompt},
			{Role: "user", Content: prompt},
		},
		Temperature: 0.1,
	}

	headers := map[string]string{}
	if o.base.apiKey != "" {
		headers["Authorization"] = "Bearer " + o.base.apiKey
	}

	resp, err := o.base.chatCompletion(ctx, "/chat/completions", headers, req)
	if err != nil {
		return nil, fmt.Errorf("openai classify: %w", err)
	}

	content, err := extractContent(resp)
	if err != nil {
		return nil, fmt.Errorf("openai classify extract: %w", err)
	}

	return parseClassifications(content)
}

func (o *OpenAICompatAnalyzer) Summarize(ctx context.Context, article ArticleInput) (string, error) {
	prompt := BuildSummarizePrompt(article)

	req := ChatRequest{
		Model: o.base.model,
		Messages: []ChatMessage{
			{Role: "system", Content: systemPrompt},
			{Role: "user", Content: prompt},
		},
		Temperature: 0.3,
		MaxTokens:   500,
	}

	headers := map[string]string{}
	if o.base.apiKey != "" {
		headers["Authorization"] = "Bearer " + o.base.apiKey
	}

	resp, err := o.base.chatCompletion(ctx, "/chat/completions", headers, req)
	if err != nil {
		return "", fmt.Errorf("openai summarize: %w", err)
	}

	return extractContent(resp)
}

func (o *OpenAICompatAnalyzer) GenerateBriefing(ctx context.Context, sections []BriefingSection) (string, error) {
	prompt := BuildBriefingPrompt(sections)

	req := ChatRequest{
		Model: o.base.model,
		Messages: []ChatMessage{
			{Role: "system", Content: systemPrompt},
			{Role: "user", Content: prompt},
		},
		Temperature: 0.5,
		MaxTokens:   4000,
	}

	headers := map[string]string{}
	if o.base.apiKey != "" {
		headers["Authorization"] = "Bearer " + o.base.apiKey
	}

	resp, err := o.base.chatCompletion(ctx, "/chat/completions", headers, req)
	if err != nil {
		return "", fmt.Errorf("openai briefing: %w", err)
	}

	return extractContent(resp)
}
