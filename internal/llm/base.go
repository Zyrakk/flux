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

// baseClient provides shared HTTP and parsing logic for LLM implementations.
type baseClient struct {
	httpClient *http.Client
	endpoint   string
	model      string
	apiKey     string
}

func newBaseClient(endpoint, model, apiKey string) baseClient {
	return baseClient{
		httpClient: &http.Client{Timeout: 120 * time.Second},
		endpoint:   endpoint,
		model:      model,
		apiKey:     apiKey,
	}
}

// chatCompletion sends an OpenAI-compatible chat completion request.
func (c *baseClient) chatCompletion(ctx context.Context, path string, headers map[string]string, req ChatRequest) (*ChatResponse, error) {
	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("marshalling request: %w", err)
	}

	url := c.endpoint + path
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")
	for k, v := range headers {
		httpReq.Header.Set(k, v)
	}

	start := time.Now()
	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("executing request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading response body: %w", err)
	}

	duration := time.Since(start)

	if resp.StatusCode != http.StatusOK {
		log.WithFields(log.Fields{
			"status":   resp.StatusCode,
			"body":     string(respBody[:min(len(respBody), 500)]),
			"duration": duration,
		}).Error("LLM API error")
		return nil, fmt.Errorf("API returned status %d: %s", resp.StatusCode, string(respBody[:min(len(respBody), 200)]))
	}

	var chatResp ChatResponse
	if err := json.Unmarshal(respBody, &chatResp); err != nil {
		return nil, fmt.Errorf("unmarshalling response: %w", err)
	}

	if chatResp.Usage != nil {
		log.WithFields(log.Fields{
			"prompt_tokens":     chatResp.Usage.PromptTokens,
			"completion_tokens": chatResp.Usage.CompletionTokens,
			"total_tokens":      chatResp.Usage.TotalTokens,
			"duration":          duration,
		}).Debug("LLM API usage")
	}

	return &chatResp, nil
}

// extractContent returns the text content from the first choice in a response.
func extractContent(resp *ChatResponse) (string, error) {
	if len(resp.Choices) == 0 {
		return "", fmt.Errorf("empty response: no choices returned")
	}
	return resp.Choices[0].Message.Content, nil
}

// parseClassifications parses the JSON array from the LLM classification response.
func parseClassifications(raw string) ([]Classification, error) {
	// Strip markdown code fences if present
	raw = stripCodeFences(raw)

	var classifications []Classification
	if err := json.Unmarshal([]byte(raw), &classifications); err != nil {
		return nil, fmt.Errorf("parsing classifications JSON: %w (raw: %.200s)", err, raw)
	}
	return classifications, nil
}

// stripCodeFences removes ```json ... ``` wrappers from LLM output.
func stripCodeFences(s string) string {
	s = trimPrefix(s, "```json\n")
	s = trimPrefix(s, "```json")
	s = trimPrefix(s, "```\n")
	s = trimPrefix(s, "```")
	s = trimSuffix(s, "\n```")
	s = trimSuffix(s, "```")
	return s
}

func trimPrefix(s, prefix string) string {
	if len(s) >= len(prefix) && s[:len(prefix)] == prefix {
		return s[len(prefix):]
	}
	return s
}

func trimSuffix(s, suffix string) string {
	if len(s) >= len(suffix) && s[len(s)-len(suffix):] == suffix {
		return s[:len(s)-len(suffix)]
	}
	return s
}
