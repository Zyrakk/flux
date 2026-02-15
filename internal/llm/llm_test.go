package llm

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- Test helpers ---

func newMockOpenAIServer(t *testing.T, handler http.HandlerFunc) *httptest.Server {
	t.Helper()
	return httptest.NewServer(handler)
}

func openAIHandler(responseContent string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		resp := ChatResponse{
			Choices: []ChatChoice{
				{Message: ChatMessage{Role: "assistant", Content: responseContent}},
			},
			Usage: &ChatUsage{PromptTokens: 100, CompletionTokens: 50, TotalTokens: 150},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}
}

func anthropicHandler(responseContent string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Verify Anthropic-specific headers
		if r.Header.Get("x-api-key") == "" {
			http.Error(w, "missing x-api-key", http.StatusUnauthorized)
			return
		}
		if r.Header.Get("anthropic-version") == "" {
			http.Error(w, "missing anthropic-version", http.StatusBadRequest)
			return
		}

		resp := anthropicResponse{
			Content: []anthropicContent{
				{Type: "text", Text: responseContent},
			},
			Usage: &anthropicUsage{InputTokens: 100, OutputTokens: 50},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}
}

var testArticles = []ArticleInput{
	{
		ID:         "art-1",
		Title:      "Critical CVE in Kubernetes RBAC",
		Content:    "A new vulnerability CVE-2025-1234 has been found in Kubernetes RBAC...",
		Section:    "cybersecurity",
		SourceType: "rss",
		URL:        "https://example.com/cve-k8s",
	},
	{
		ID:         "art-2",
		Title:      "Go 1.24 Released with Major Performance Improvements",
		Content:    "The Go team has released Go 1.24 with significant improvements to the garbage collector...",
		Section:    "tech",
		SourceType: "hn",
		URL:        "https://go.dev/blog/go1.24",
	},
}

var testClassificationResponse = `[
	{"article_id": "art-1", "relevant": true, "section": "cybersecurity", "clickbait": false, "reason": "Real CVE affecting Kubernetes RBAC"},
	{"article_id": "art-2", "relevant": true, "section": "tech", "clickbait": false, "reason": "Major Go release with concrete improvements"}
]`

// --- Factory tests ---

func TestNewAnalyzer(t *testing.T) {
	tests := []struct {
		provider string
		wantType string
		wantErr  bool
	}{
		{"glm", "glm", false},
		{"openai_compat", "openai_compat", false},
		{"anthropic", "anthropic", false},
		{"unknown", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.provider, func(t *testing.T) {
			a, err := NewAnalyzer(tt.provider, "http://localhost", "model", "key")
			if tt.wantErr {
				assert.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.wantType, a.Provider())
		})
	}
}

// --- GLM tests ---

func TestGLMClassify(t *testing.T) {
	srv := newMockOpenAIServer(t, openAIHandler(testClassificationResponse))
	defer srv.Close()

	analyzer := NewGLMAnalyzer(srv.URL, "glm-4.7", "test-key")
	results, err := analyzer.Classify(context.Background(), testArticles)
	require.NoError(t, err)
	require.Len(t, results, 2)
	assert.Equal(t, "art-1", results[0].ArticleID)
	assert.True(t, results[0].Relevant)
	assert.Equal(t, "cybersecurity", results[0].Section)
}

func TestGLMSummarize(t *testing.T) {
	expected := "A critical RBAC vulnerability (CVE-2025-1234) was discovered in Kubernetes. Patch available in 1.29.1."
	srv := newMockOpenAIServer(t, openAIHandler(expected))
	defer srv.Close()

	analyzer := NewGLMAnalyzer(srv.URL, "glm-4.7", "test-key")
	result, err := analyzer.Summarize(context.Background(), testArticles[0])
	require.NoError(t, err)
	assert.Equal(t, expected, result)
}

func TestGLMGenerateBriefing(t *testing.T) {
	expected := "## 🔒 Cybersecurity\n\n1. **Critical CVE in K8s RBAC**..."
	srv := newMockOpenAIServer(t, openAIHandler(expected))
	defer srv.Close()

	analyzer := NewGLMAnalyzer(srv.URL, "glm-4.7", "test-key")
	sections := []BriefingSection{
		{
			Name:        "cybersecurity",
			DisplayName: "🔒 Cybersecurity",
			MaxArticles: 5,
			Articles: []SummarizedArticle{
				{ID: "art-1", Title: "Critical CVE", Summary: "A CVE was found.", URL: "https://example.com"},
			},
		},
	}
	result, err := analyzer.GenerateBriefing(context.Background(), sections)
	require.NoError(t, err)
	assert.Contains(t, result, "Cybersecurity")
}

// --- OpenAI-compatible tests ---

func TestOpenAICompatClassify(t *testing.T) {
	srv := newMockOpenAIServer(t, openAIHandler(testClassificationResponse))
	defer srv.Close()

	analyzer := NewOpenAICompatAnalyzer(srv.URL, "gpt-4o-mini", "test-key")
	results, err := analyzer.Classify(context.Background(), testArticles)
	require.NoError(t, err)
	require.Len(t, results, 2)
	assert.True(t, results[1].Relevant)
	assert.Equal(t, "tech", results[1].Section)
}

func TestOpenAICompatSummarize(t *testing.T) {
	expected := "Go 1.24 brings major GC improvements and 15% faster compilation."
	srv := newMockOpenAIServer(t, openAIHandler(expected))
	defer srv.Close()

	analyzer := NewOpenAICompatAnalyzer(srv.URL, "gpt-4o-mini", "")
	result, err := analyzer.Summarize(context.Background(), testArticles[1])
	require.NoError(t, err)
	assert.Equal(t, expected, result)
}

// --- Anthropic tests ---

func TestAnthropicClassify(t *testing.T) {
	srv := newMockOpenAIServer(t, anthropicHandler(testClassificationResponse))
	defer srv.Close()

	analyzer := NewAnthropicAnalyzer(srv.URL, "claude-sonnet-4-20250514", "test-key")
	results, err := analyzer.Classify(context.Background(), testArticles)
	require.NoError(t, err)
	require.Len(t, results, 2)
	assert.False(t, results[0].Clickbait)
}

func TestAnthropicSummarize(t *testing.T) {
	expected := "Kubernetes RBAC has a critical vulnerability. Update immediately."
	srv := newMockOpenAIServer(t, anthropicHandler(expected))
	defer srv.Close()

	analyzer := NewAnthropicAnalyzer(srv.URL, "claude-sonnet-4-20250514", "test-key")
	result, err := analyzer.Summarize(context.Background(), testArticles[0])
	require.NoError(t, err)
	assert.Equal(t, expected, result)
}

func TestAnthropicHeaders(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "test-api-key", r.Header.Get("x-api-key"))
		assert.Equal(t, "2023-06-01", r.Header.Get("anthropic-version"))
		assert.Equal(t, "application/json", r.Header.Get("Content-Type"))

		// Verify request body structure
		var req anthropicRequest
		err := json.NewDecoder(r.Body).Decode(&req)
		require.NoError(t, err)
		assert.Equal(t, "claude-sonnet-4-20250514", req.Model)
		assert.NotEmpty(t, req.System)
		assert.Len(t, req.Messages, 1)
		assert.Equal(t, "user", req.Messages[0].Role)

		resp := anthropicResponse{
			Content: []anthropicContent{{Type: "text", Text: "test"}},
		}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	analyzer := NewAnthropicAnalyzer(srv.URL, "claude-sonnet-4-20250514", "test-api-key")
	_, err := analyzer.Summarize(context.Background(), testArticles[0])
	require.NoError(t, err)
}

// --- Prompt tests ---

func TestBuildClassifyPrompt(t *testing.T) {
	prompt := BuildClassifyPrompt(testArticles)
	assert.Contains(t, prompt, "art-1")
	assert.Contains(t, prompt, "art-2")
	assert.Contains(t, prompt, "cybersecurity")
	assert.Contains(t, prompt, "JSON array")
}

func TestBuildSummarizePrompt(t *testing.T) {
	prompt := BuildSummarizePrompt(testArticles[0])
	assert.Contains(t, prompt, "Critical CVE")
	assert.Contains(t, prompt, "vulnerabilidad")
}

func TestStripCodeFences(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{`[{"id": 1}]`, `[{"id": 1}]`},
		{"```json\n[{\"id\": 1}]\n```", `[{"id": 1}]`},
		{"```\n[{\"id\": 1}]\n```", `[{"id": 1}]`},
	}

	for _, tt := range tests {
		assert.Equal(t, tt.expected, stripCodeFences(tt.input))
	}
}

// --- Error handling tests ---

func TestAPIErrorHandling(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = w.Write([]byte(`{"error": "rate limited"}`))
	}))
	defer srv.Close()

	analyzer := NewGLMAnalyzer(srv.URL, "glm-4.7", "test-key")
	_, err := analyzer.Classify(context.Background(), testArticles)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "429")
}

func TestEmptyResponseHandling(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := ChatResponse{Choices: []ChatChoice{}}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	analyzer := NewOpenAICompatAnalyzer(srv.URL, "model", "key")
	_, err := analyzer.Summarize(context.Background(), testArticles[0])
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "empty response")
}
