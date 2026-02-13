package store

import (
	"context"
	"fmt"

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
