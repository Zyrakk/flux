package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	log "github.com/sirupsen/logrus"
)

// AnthropicAnalyzer implements the Analyzer interface for Anthropic's Claude API.
// Uses the Messages API format which differs from OpenAI's.
type AnthropicAnalyzer struct {
	httpClient *http.Client
	endpoint   string
	model      string
	apiKey     string
}

// Anthropic-specific request/response types.

type anthropicRequest struct {
	Model       string             `json:"model"`
	MaxTokens   int                `json:"max_tokens"`
	System      string             `json:"system,omitempty"`
	Messages    []anthropicMessage `json:"messages"`
	Temperature float64            `json:"temperature,omitempty"`
}

type anthropicMessage struct {
	Role    string `json:"role"` // "user" or "assistant"
	Content string `json:"content"`
}

type anthropicResponse struct {
	Content []anthropicContent `json:"content"`
	Usage   *anthropicUsage    `json:"usage,omitempty"`
}

type anthropicContent struct {
	Type string `json:"type"` // "text"
	Text string `json:"text"`
}

type anthropicUsage struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
}

// NewAnthropicAnalyzer creates an Anthropic analyzer.
func NewAnthropicAnalyzer(endpoint, model, apiKey string) *AnthropicAnalyzer {
	if endpoint == "" {
		endpoint = "https://api.anthropic.com"
	}
	if model == "" {
		model = "claude-sonnet-4-20250514"
	}
	return &AnthropicAnalyzer{
		httpClient: &http.Client{Timeout: 120 * time.Second},
		endpoint:   endpoint,
		model:      model,
		apiKey:     apiKey,
	}
}

func (a *AnthropicAnalyzer) Provider() string { return "anthropic" }

func (a *AnthropicAnalyzer) complete(ctx context.Context, system, userMessage string, maxTokens int, temperature float64) (string, error) {
	req := anthropicRequest{
		Model:     a.model,
		MaxTokens: maxTokens,
		System:    system,
		Messages: []anthropicMessage{
			{Role: "user", Content: userMessage},
		},
		Temperature: temperature,
	}

	body, err := json.Marshal(req)
	if err != nil {
		return "", fmt.Errorf("marshalling request: %w", err)
	}

	url := a.endpoint + "/v1/messages"
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("creating request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("x-api-key", a.apiKey)
	httpReq.Header.Set("anthropic-version", "2023-06-01")

	start := time.Now()
	resp, err := a.httpClient.Do(httpReq)
	if err != nil {
		return "", fmt.Errorf("executing request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("reading response: %w", err)
	}

	duration := time.Since(start)

	if resp.StatusCode != http.StatusOK {
		log.WithFields(log.Fields{
			"status":   resp.StatusCode,
			"body":     string(respBody[:min(len(respBody), 500)]),
			"duration": duration,
		}).Error("Anthropic API error")
		return "", fmt.Errorf("API returned status %d: %s", resp.StatusCode, string(respBody[:min(len(respBody), 200)]))
	}

	var anthropicResp anthropicResponse
	if err := json.Unmarshal(respBody, &anthropicResp); err != nil {
		return "", fmt.Errorf("unmarshalling response: %w", err)
	}

	if anthropicResp.Usage != nil {
		log.WithFields(log.Fields{
			"input_tokens":  anthropicResp.Usage.InputTokens,
			"output_tokens": anthropicResp.Usage.OutputTokens,
			"duration":      duration,
		}).Debug("Anthropic API usage")
	}

	if len(anthropicResp.Content) == 0 {
		return "", fmt.Errorf("empty response: no content blocks returned")
	}

	// Concatenate all text blocks
	var result string
	for _, block := range anthropicResp.Content {
		if block.Type == "text" {
			result += block.Text
		}
	}
	return result, nil
}

func (a *AnthropicAnalyzer) Classify(ctx context.Context, articles []ArticleInput) ([]Classification, error) {
	prompt := BuildClassifyPrompt(articles)

	content, err := a.complete(ctx, systemPrompt, prompt, 2000, 0.1)
	if err != nil {
		return nil, fmt.Errorf("anthropic classify: %w", err)
	}

	return parseClassifications(content)
}

func (a *AnthropicAnalyzer) Summarize(ctx context.Context, article ArticleInput) (string, error) {
	prompt := BuildSummarizePrompt(article)

	content, err := a.complete(ctx, systemPrompt, prompt, 500, 0.3)
	if err != nil {
		return "", fmt.Errorf("anthropic summarize: %w", err)
	}
	return content, nil
}

func (a *AnthropicAnalyzer) GenerateBriefing(ctx context.Context, sections []BriefingSection) (string, error) {
	prompt := BuildBriefingPrompt(sections)

	content, err := a.complete(ctx, systemPrompt, prompt, 4000, 0.5)
	if err != nil {
		return "", fmt.Errorf("anthropic briefing: %w", err)
	}
	return content, nil
}
