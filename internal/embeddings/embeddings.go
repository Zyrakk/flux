package embeddings

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"os"
	"time"
)

// Client communicates with the local embeddings service (all-MiniLM-L6-v2).
type Client struct {
	httpClient *http.Client
	endpoint   string
	maxRetries int
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
		endpoint = os.Getenv("EMBEDDINGS_URL")
	}
	if endpoint == "" {
		endpoint = "http://embeddings-svc:8000"
	}
	return &Client{
		httpClient: &http.Client{Timeout: 30 * time.Second},
		endpoint:   endpoint,
		maxRetries: 6,
	}
}

// Embed generates embeddings for one or more texts.
func (c *Client) Embed(ctx context.Context, texts []string) ([][]float32, error) {
	if len(texts) == 0 {
		return [][]float32{}, nil
	}

	// Large payloads are split into smaller requests to reduce memory spikes.
	if len(texts) > 100 {
		return c.embedInBatches(ctx, texts, 32)
	}

	return c.embedRequestWithRetry(ctx, texts)
}

func (c *Client) embedInBatches(ctx context.Context, texts []string, batchSize int) ([][]float32, error) {
	out := make([][]float32, 0, len(texts))
	for start := 0; start < len(texts); start += batchSize {
		end := start + batchSize
		if end > len(texts) {
			end = len(texts)
		}
		batchEmbeddings, err := c.embedRequestWithRetry(ctx, texts[start:end])
		if err != nil {
			return nil, err
		}
		out = append(out, batchEmbeddings...)
	}
	return out, nil
}

func (c *Client) embedRequestWithRetry(ctx context.Context, texts []string) ([][]float32, error) {
	body, err := json.Marshal(EmbeddingRequest{Texts: texts})
	if err != nil {
		return nil, fmt.Errorf("marshalling request: %w", err)
	}

	var lastErr error
	backoff := 500 * time.Millisecond
	for attempt := 1; attempt <= c.maxRetries; attempt++ {
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint+"/embed", bytes.NewReader(body))
		if err != nil {
			return nil, fmt.Errorf("creating request: %w", err)
		}
		req.Header.Set("Content-Type", "application/json")

		resp, err := c.httpClient.Do(req)
		if err != nil {
			lastErr = fmt.Errorf("executing request: %w", err)
		} else {
			respBody, readErr := io.ReadAll(resp.Body)
			resp.Body.Close()
			if readErr != nil {
				lastErr = fmt.Errorf("reading response: %w", readErr)
			} else if resp.StatusCode == http.StatusOK {
				var embResp EmbeddingResponse
				if err := json.Unmarshal(respBody, &embResp); err != nil {
					return nil, fmt.Errorf("unmarshalling response: %w", err)
				}
				if len(embResp.Embeddings) != len(texts) {
					return nil, fmt.Errorf("embeddings count mismatch: requested=%d got=%d", len(texts), len(embResp.Embeddings))
				}
				return embResp.Embeddings, nil
			} else {
				lastErr = fmt.Errorf("embeddings service returned %d: %s", resp.StatusCode, string(respBody))
				if !isRetryableStatus(resp.StatusCode) {
					return nil, lastErr
				}
			}
		}

		if attempt == c.maxRetries {
			break
		}
		if err := sleepWithContext(ctx, backoff); err != nil {
			return nil, err
		}
		backoff *= 2
		if backoff > 8*time.Second {
			backoff = 8 * time.Second
		}
	}

	if lastErr == nil {
		lastErr = errors.New("unknown embeddings error")
	}
	return nil, fmt.Errorf("embeddings request failed after retries: %w", lastErr)
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

	return dotProduct / (math.Sqrt(normA) * math.Sqrt(normB))
}

func isRetryableStatus(status int) bool {
	switch status {
	case http.StatusTooManyRequests, http.StatusBadGateway, http.StatusServiceUnavailable, http.StatusGatewayTimeout:
		return true
	default:
		return false
	}
}

func sleepWithContext(ctx context.Context, d time.Duration) error {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
