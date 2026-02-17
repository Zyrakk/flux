package store

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/pgvector/pgvector-go"
)

// SimilarArticle is a lightweight projection used for semantic deduplication.
type SimilarArticle struct {
	ID         string
	Title      string
	SourceType string
	Similarity float64
	IngestedAt time.Time
	Metadata   json.RawMessage
}

// FindSimilarArticlesLast48h returns nearest neighbors by cosine similarity from recent articles.
func (s *Store) FindSimilarArticlesLast48h(ctx context.Context, embedding []float32, excludeArticleID string, limit int) ([]*SimilarArticle, error) {
	if len(embedding) == 0 {
		return []*SimilarArticle{}, nil
	}
	if limit <= 0 {
		limit = 5
	}

	vec := pgvector.NewVector(embedding)
	rows, err := s.pool.Query(ctx, `
		SELECT id, title, source_type, ingested_at, metadata, 1 - (embedding <=> $1) AS similarity
		FROM articles
		WHERE id <> $2
			AND ingested_at > NOW() - INTERVAL '48 hours'
			AND embedding IS NOT NULL
		ORDER BY embedding <=> $1
		LIMIT $3`,
		vec, excludeArticleID, limit,
	)
	if err != nil {
		return nil, fmt.Errorf("listing similar recent articles: %w", err)
	}
	defer rows.Close()

	out := make([]*SimilarArticle, 0, limit)
	for rows.Next() {
		a := &SimilarArticle{}
		if err := rows.Scan(&a.ID, &a.Title, &a.SourceType, &a.IngestedAt, &a.Metadata, &a.Similarity); err != nil {
			return nil, fmt.Errorf("scanning similar recent article: %w", err)
		}
		out = append(out, a)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

// UpdateArticleMetadata replaces article metadata JSON.
func (s *Store) UpdateArticleMetadata(ctx context.Context, id string, metadata json.RawMessage) error {
	_, err := s.pool.Exec(ctx, `UPDATE articles SET metadata = $1 WHERE id = $2`, metadata, id)
	if err != nil {
		return fmt.Errorf("updating article metadata %s: %w", id, err)
	}
	return nil
}
