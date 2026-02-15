package store

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/zyrak/flux/internal/models"
)

// CreateBriefing inserts a new briefing.
func (s *Store) CreateBriefing(ctx context.Context, b *models.Briefing) error {
	return s.pool.QueryRow(ctx, `
		INSERT INTO briefings (content, article_ids, metadata)
		VALUES ($1, $2, $3)
		RETURNING id, generated_at`,
		b.Content, b.ArticleIDs, b.Metadata,
	).Scan(&b.ID, &b.GeneratedAt)
}

// GetLatestBriefing returns the most recently generated briefing.
func (s *Store) GetLatestBriefing(ctx context.Context) (*models.Briefing, error) {
	b := &models.Briefing{}
	err := s.pool.QueryRow(ctx, `
		SELECT id, generated_at, content, article_ids, metadata
		FROM briefings ORDER BY generated_at DESC LIMIT 1`).
		Scan(&b.ID, &b.GeneratedAt, &b.Content, &b.ArticleIDs, &b.Metadata)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("getting latest briefing: %w", err)
	}
	return b, nil
}

// ListBriefings returns briefings ordered by date, with pagination.
func (s *Store) ListBriefings(ctx context.Context, limit, offset int) ([]*models.Briefing, error) {
	if limit <= 0 {
		limit = 20
	}
	rows, err := s.pool.Query(ctx, `
		SELECT id, generated_at, content, article_ids, metadata
		FROM briefings ORDER BY generated_at DESC LIMIT $1 OFFSET $2`, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("listing briefings: %w", err)
	}
	defer rows.Close()

	var briefings []*models.Briefing
	for rows.Next() {
		b := &models.Briefing{}
		if err := rows.Scan(&b.ID, &b.GeneratedAt, &b.Content, &b.ArticleIDs, &b.Metadata); err != nil {
			return nil, fmt.Errorf("scanning briefing: %w", err)
		}
		briefings = append(briefings, b)
	}
	return briefings, rows.Err()
}

// GetBriefingByID returns one briefing by id.
func (s *Store) GetBriefingByID(ctx context.Context, id string) (*models.Briefing, error) {
	b := &models.Briefing{}
	err := s.pool.QueryRow(ctx, `
		SELECT id, generated_at, content, article_ids, metadata
		FROM briefings WHERE id = $1`,
		id,
	).Scan(&b.ID, &b.GeneratedAt, &b.Content, &b.ArticleIDs, &b.Metadata)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("getting briefing %s: %w", id, err)
	}
	return b, nil
}
