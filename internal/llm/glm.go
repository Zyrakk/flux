package llm

import (
	"context"
	"fmt"
)

// GLMAnalyzer implements the Analyzer interface for Zhipu's GLM models.
// GLM uses an OpenAI-compatible API format with minor differences.
type GLMAnalyzer struct {
	base baseClient
}

// NewGLMAnalyzer creates a GLM analyzer.
// Default endpoint: https://open.bigmodel.cn/api/coding/paas/v4
// Default model: glm-4.7
func NewGLMAnalyzer(endpoint, model, apiKey string) *GLMAnalyzer {
	if endpoint == "" {
		endpoint = "https://open.bigmodel.cn/api/coding/paas/v4"
	}
	if model == "" {
		model = "glm-4.7"
	}
	return &GLMAnalyzer{
		base: newBaseClient(endpoint, model, apiKey),
	}
}

func (g *GLMAnalyzer) Provider() string { return "glm" }

func (g *GLMAnalyzer) Classify(ctx context.Context, articles []ArticleInput) ([]Classification, error) {
	prompt := BuildClassifyPrompt(articles)

	req := ChatRequest{
		Model: g.base.model,
		Messages: []ChatMessage{
			{Role: "system", Content: systemPrompt},
			{Role: "user", Content: prompt},
		},
		Temperature: 0.1,
	}

	headers := map[string]string{
		"Authorization": "Bearer " + g.base.apiKey,
	}

	resp, err := g.base.chatCompletion(ctx, "/chat/completions", headers, req)
	if err != nil {
		return nil, fmt.Errorf("glm classify: %w", err)
	}

	content, err := extractContent(resp)
	if err != nil {
		return nil, fmt.Errorf("glm classify extract: %w", err)
	}

	return parseClassifications(content)
}

func (g *GLMAnalyzer) Summarize(ctx context.Context, article ArticleInput) (string, error) {
	prompt := BuildSummarizePrompt(article)

	req := ChatRequest{
		Model: g.base.model,
		Messages: []ChatMessage{
			{Role: "system", Content: systemPrompt},
			{Role: "user", Content: prompt},
		},
		Temperature: 0.3,
		MaxTokens:   500,
	}

	headers := map[string]string{
		"Authorization": "Bearer " + g.base.apiKey,
	}

	resp, err := g.base.chatCompletion(ctx, "/chat/completions", headers, req)
	if err != nil {
		return "", fmt.Errorf("glm summarize: %w", err)
	}

	return extractContent(resp)
}

func (g *GLMAnalyzer) GenerateBriefing(ctx context.Context, sections []BriefingSection) (string, error) {
	prompt := BuildBriefingPrompt(sections)

	req := ChatRequest{
		Model: g.base.model,
		Messages: []ChatMessage{
			{Role: "system", Content: systemPrompt},
			{Role: "user", Content: prompt},
		},
		Temperature: 0.5,
		MaxTokens:   4000,
	}

	headers := map[string]string{
		"Authorization": "Bearer " + g.base.apiKey,
	}

	resp, err := g.base.chatCompletion(ctx, "/chat/completions", headers, req)
	if err != nil {
		return "", fmt.Errorf("glm briefing: %w", err)
	}

	return extractContent(resp)
}
