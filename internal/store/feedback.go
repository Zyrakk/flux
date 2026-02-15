package store

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/pgvector/pgvector-go"
	"github.com/zyrak/flux/internal/models"
)

// CreateFeedback records a user feedback action on an article.
func (s *Store) CreateFeedback(ctx context.Context, f *models.Feedback) error {
	return s.pool.QueryRow(ctx, `
		INSERT INTO feedback (article_id, action) VALUES ($1, $2)
		RETURNING id, created_at`,
		f.ArticleID, f.Action,
	).Scan(&f.ID, &f.CreatedAt)
}

// GetFeedbackByID returns a single feedback item by id.
func (s *Store) GetFeedbackByID(ctx context.Context, id string) (*models.Feedback, error) {
	f := &models.Feedback{}
	err := s.pool.QueryRow(ctx, `
		SELECT id, article_id, action, created_at
		FROM feedback WHERE id = $1`, id,
	).Scan(&f.ID, &f.ArticleID, &f.Action, &f.CreatedAt)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("getting feedback %s: %w", id, err)
	}
	return f, nil
}

// DeleteFeedbackByID deletes one feedback row and returns the deleted object.
func (s *Store) DeleteFeedbackByID(ctx context.Context, id string) (*models.Feedback, error) {
	f := &models.Feedback{}
	err := s.pool.QueryRow(ctx, `
		DELETE FROM feedback
		WHERE id = $1
		RETURNING id, article_id, action, created_at`, id,
	).Scan(&f.ID, &f.ArticleID, &f.Action, &f.CreatedAt)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("deleting feedback %s: %w", id, err)
	}
	return f, nil
}

// GetFeedbackByArticle returns all feedback for a specific article.
func (s *Store) GetFeedbackByArticle(ctx context.Context, articleID string) ([]*models.Feedback, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, article_id, action, created_at
		FROM feedback WHERE article_id = $1 ORDER BY created_at DESC`, articleID)
	if err != nil {
		return nil, fmt.Errorf("getting feedback for article %s: %w", articleID, err)
	}
	defer rows.Close()

	var feedbacks []*models.Feedback
	for rows.Next() {
		f := &models.Feedback{}
		if err := rows.Scan(&f.ID, &f.ArticleID, &f.Action, &f.CreatedAt); err != nil {
			return nil, fmt.Errorf("scanning feedback: %w", err)
		}
		feedbacks = append(feedbacks, f)
	}
	return feedbacks, rows.Err()
}

// GetFeedbackBySection returns all feedback for articles in a given section.
func (s *Store) GetFeedbackBySection(ctx context.Context, sectionID string) ([]*models.Feedback, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT f.id, f.article_id, f.action, f.created_at
		FROM feedback f
		JOIN articles a ON f.article_id = a.id
		WHERE a.section_id = $1
		ORDER BY f.created_at DESC`, sectionID)
	if err != nil {
		return nil, fmt.Errorf("getting feedback for section %s: %w", sectionID, err)
	}
	defer rows.Close()

	var feedbacks []*models.Feedback
	for rows.Next() {
		f := &models.Feedback{}
		if err := rows.Scan(&f.ID, &f.ArticleID, &f.Action, &f.CreatedAt); err != nil {
			return nil, fmt.Errorf("scanning feedback: %w", err)
		}
		feedbacks = append(feedbacks, f)
	}
	return feedbacks, rows.Err()
}

// CountFeedbackBySection returns like and dislike counts for a section.
func (s *Store) CountFeedbackBySection(ctx context.Context, sectionID string) (likes, dislikes int, err error) {
	err = s.pool.QueryRow(ctx, `
		SELECT
			COUNT(*) FILTER (WHERE f.action = 'like'),
			COUNT(*) FILTER (WHERE f.action = 'dislike')
		FROM feedback f
		JOIN articles a ON f.article_id = a.id
		WHERE a.section_id = $1`, sectionID).Scan(&likes, &dislikes)
	return
}

// ListSectionEmbeddingsByFeedbackAction returns article embeddings for one section/action.
func (s *Store) ListSectionEmbeddingsByFeedbackAction(ctx context.Context, sectionID, action string) ([][]float32, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT a.embedding
		FROM articles a
		JOIN (
			SELECT DISTINCT article_id
			FROM feedback
			WHERE action = $2
		) f ON f.article_id = a.id
		WHERE a.section_id = $1
			AND a.embedding IS NOT NULL`, sectionID, action)
	if err != nil {
		return nil, fmt.Errorf("listing section embeddings by action (%s): %w", action, err)
	}
	defer rows.Close()

	out := make([][]float32, 0, 64)
	for rows.Next() {
		var emb pgvector.Vector
		if err := rows.Scan(&emb); err != nil {
			return nil, fmt.Errorf("scanning feedback embedding: %w", err)
		}
		out = append(out, emb.Slice())
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}
