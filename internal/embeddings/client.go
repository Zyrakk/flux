package embeddings

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// Client communicates with the local embeddings service (all-MiniLM-L6-v2).
type Client struct {
	httpClient *http.Client
	endpoint   string
}

// EmbeddingRequest is the request body for the embeddings service.
type EmbeddingRequest struct {
	Texts []string `json:"texts"`
}

// EmbeddingResponse is the response from the embeddings service.
type EmbeddingResponse struct {
	Embeddings [][]float32 `json:"embeddings"`
}

// NewClient creates a new embeddings client.
func NewClient(endpoint string) *Client {
	if endpoint == "" {
		endpoint = "http://localhost:8000"
	}
	return &Client{
		httpClient: &http.Client{Timeout: 30 * time.Second},
		endpoint:   endpoint,
	}
}

// Embed generates embeddings for one or more texts.
func (c *Client) Embed(ctx context.Context, texts []string) ([][]float32, error) {
	body, err := json.Marshal(EmbeddingRequest{Texts: texts})
	if err != nil {
		return nil, fmt.Errorf("marshalling request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint+"/embed", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("executing request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("embeddings service returned %d: %s", resp.StatusCode, string(respBody))
	}

	var embResp EmbeddingResponse
	if err := json.Unmarshal(respBody, &embResp); err != nil {
		return nil, fmt.Errorf("unmarshalling response: %w", err)
	}

	return embResp.Embeddings, nil
}

// EmbedSingle generates an embedding for a single text.
func (c *Client) EmbedSingle(ctx context.Context, text string) ([]float32, error) {
	results, err := c.Embed(ctx, []string{text})
	if err != nil {
		return nil, err
	}
	if len(results) == 0 {
		return nil, fmt.Errorf("no embeddings returned")
	}
	return results[0], nil
}

// CosineSimilarity calculates the cosine similarity between two vectors.
func CosineSimilarity(a, b []float32) float64 {
	if len(a) != len(b) || len(a) == 0 {
		return 0
	}

	var dotProduct, normA, normB float64
	for i := range a {
		dotProduct += float64(a[i]) * float64(b[i])
		normA += float64(a[i]) * float64(a[i])
		normB += float64(b[i]) * float64(b[i])
	}

	if normA == 0 || normB == 0 {
		return 0
	}

	return dotProduct / (sqrt(normA) * sqrt(normB))
}

func sqrt(x float64) float64 {
	if x <= 0 {
		return 0
	}
	// Newton's method
	z := x
	for i := 0; i < 20; i++ {
		z = z - (z*z-x)/(2*z)
	}
	return z
}
