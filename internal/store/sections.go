package store

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/zyrak/flux/internal/models"
)

// ListSections returns all sections ordered by sort_order.
func (s *Store) ListSections(ctx context.Context) ([]*models.Section, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, name, display_name, enabled, sort_order, max_briefing_articles, seed_keywords, config
		FROM sections ORDER BY sort_order`)
	if err != nil {
		return nil, fmt.Errorf("listing sections: %w", err)
	}
	defer rows.Close()

	var sections []*models.Section
	for rows.Next() {
		sec := &models.Section{}
		if err := rows.Scan(&sec.ID, &sec.Name, &sec.DisplayName, &sec.Enabled,
			&sec.SortOrder, &sec.MaxBriefingArticles, &sec.SeedKeywords, &sec.Config); err != nil {
			return nil, fmt.Errorf("scanning section: %w", err)
		}
		sections = append(sections, sec)
	}
	return sections, rows.Err()
}

// GetSectionByName returns a section by its unique name.
func (s *Store) GetSectionByName(ctx context.Context, name string) (*models.Section, error) {
	sec := &models.Section{}
	err := s.pool.QueryRow(ctx, `
		SELECT id, name, display_name, enabled, sort_order, max_briefing_articles, seed_keywords, config
		FROM sections WHERE name = $1`, name).
		Scan(&sec.ID, &sec.Name, &sec.DisplayName, &sec.Enabled,
			&sec.SortOrder, &sec.MaxBriefingArticles, &sec.SeedKeywords, &sec.Config)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("getting section %q: %w", name, err)
	}
	return sec, nil
}

// CreateSection inserts a new section.
func (s *Store) CreateSection(ctx context.Context, sec *models.Section) error {
	return s.pool.QueryRow(ctx, `
		INSERT INTO sections (name, display_name, enabled, sort_order, max_briefing_articles, seed_keywords, config)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		RETURNING id`,
		sec.Name, sec.DisplayName, sec.Enabled, sec.SortOrder,
		sec.MaxBriefingArticles, sec.SeedKeywords, sec.Config,
	).Scan(&sec.ID)
}

// UpdateSection updates a section's mutable fields.
func (s *Store) UpdateSection(ctx context.Context, sec *models.Section) error {
	_, err := s.pool.Exec(ctx, `
		UPDATE sections
		SET display_name = $1, enabled = $2, sort_order = $3,
			max_briefing_articles = $4, seed_keywords = $5, config = $6
		WHERE id = $7`,
		sec.DisplayName, sec.Enabled, sec.SortOrder,
		sec.MaxBriefingArticles, sec.SeedKeywords, sec.Config, sec.ID)
	return err
}

// ToggleSection enables or disables a section.
func (s *Store) ToggleSection(ctx context.Context, id string, enabled bool) error {
	_, err := s.pool.Exec(ctx, `UPDATE sections SET enabled = $1 WHERE id = $2`, enabled, id)
	return err
}

// UpdateSectionThreshold stores the current relevance threshold in section config.
func (s *Store) UpdateSectionThreshold(ctx context.Context, sectionID string, threshold float64) error {
	_, err := s.pool.Exec(ctx, `
		UPDATE sections
		SET config = jsonb_set(
			COALESCE(config, '{}'::jsonb),
			'{relevance_threshold}',
			to_jsonb($1::float8),
			true
		)
		WHERE id = $2`,
		threshold, sectionID,
	)
	if err != nil {
		return fmt.Errorf("updating section threshold for %s: %w", sectionID, err)
	}
	return nil
}
