package llm

import (
	"context"
)

// Analyzer defines the interface for LLM-powered article analysis.
// All implementations (GLM, OpenAI-compatible, Anthropic) must satisfy this.
type Analyzer interface {
	// Classify takes a batch of articles and returns classifications for each.
	// Used in Phase 2 of the pipeline to filter irrelevant/clickbait content.
	Classify(ctx context.Context, articles []ArticleInput) ([]Classification, error)

	// Summarize generates a concise summary of a single article.
	Summarize(ctx context.Context, article ArticleInput) (string, error)

	// GenerateBriefing synthesizes multiple summarized articles into a structured briefing.
	GenerateBriefing(ctx context.Context, sections []BriefingSection) (string, error)

	// Provider returns the name of the LLM provider (for logging/metrics).
	Provider() string
}

// ArticleInput is the minimal article data sent to the LLM.
type ArticleInput struct {
	ID         string `json:"id"`
	Title      string `json:"title"`
	Content    string `json:"content"`     // Full text or truncated
	Section    string `json:"section"`     // Pre-assigned section name
	SourceType string `json:"source_type"` // rss, hn, reddit
	URL        string `json:"url"`
}

// Classification is the LLM's verdict on an article.
type Classification struct {
	ArticleID string `json:"article_id"`
	Relevant  bool   `json:"relevant"`
	Section   string `json:"section"` // Confirmed or corrected section
	Clickbait bool   `json:"clickbait"`
	Reason    string `json:"reason"`
}

// SummarizedArticle is an article with its LLM-generated summary, ready for briefing.
type SummarizedArticle struct {
	ID         string   `json:"id"`
	Title      string   `json:"title"`
	Summary    string   `json:"summary"`
	URL        string   `json:"url"`
	SourceType string   `json:"source_type"`
	Categories []string `json:"categories,omitempty"`
}

// BriefingSection groups summarized articles by section for briefing generation.
type BriefingSection struct {
	Name        string              `json:"name"`
	DisplayName string              `json:"display_name"`
	MaxArticles int                 `json:"max_articles"`
	Articles    []SummarizedArticle `json:"articles"`
}

// ChatMessage represents a message in a chat completion request.
type ChatMessage struct {
	Role    string `json:"role"` // "system", "user", "assistant"
	Content string `json:"content"`
}

// ChatRequest is a generic chat completion request body.
type ChatRequest struct {
	Model       string        `json:"model"`
	Messages    []ChatMessage `json:"messages"`
	Temperature float64       `json:"temperature,omitempty"`
	MaxTokens   int           `json:"max_tokens,omitempty"`
}

// ChatResponse is a generic chat completion response.
type ChatResponse struct {
	Choices []ChatChoice `json:"choices"`
	Usage   *ChatUsage   `json:"usage,omitempty"`
}

// ChatChoice is a single choice in a chat response.
type ChatChoice struct {
	Message ChatMessage `json:"message"`
}

// ChatUsage tracks token consumption.
type ChatUsage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}
