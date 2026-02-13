package store

import (
	"context"
	"fmt"
	"time"

	"github.com/zyrak/flux/internal/models"
)

// ListSources returns sources, optionally filtered.
func (s *Store) ListSources(ctx context.Context, f models.SourceFilter) ([]*models.Source, error) {
	query := `
		SELECT s.id, s.source_type, s.name, s.config, s.enabled, s.last_fetched_at, s.error_count, s.last_error
		FROM sources s`
	var args []interface{}
	argIdx := 1
	conditions := ""

	if f.SectionID != nil {
		query += ` JOIN source_sections ss ON s.id = ss.source_id`
		conditions += fmt.Sprintf(" AND ss.section_id = $%d", argIdx)
		args = append(args, *f.SectionID)
		argIdx++
	}
	if f.SourceType != nil {
		conditions += fmt.Sprintf(" AND s.source_type = $%d", argIdx)
		args = append(args, *f.SourceType)
		argIdx++
	}
	if f.Enabled != nil {
		conditions += fmt.Sprintf(" AND s.enabled = $%d", argIdx)
		args = append(args, *f.Enabled)
	}

	if conditions != "" {
		query += " WHERE " + conditions[5:] // strip leading " AND "
	}
	query += " ORDER BY s.name"

	rows, err := s.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("listing sources: %w", err)
	}
	defer rows.Close()

	var sources []*models.Source
	for rows.Next() {
		src := &models.Source{}
		if err := rows.Scan(&src.ID, &src.SourceType, &src.Name, &src.Config,
			&src.Enabled, &src.LastFetchedAt, &src.ErrorCount, &src.LastError); err != nil {
			return nil, fmt.Errorf("scanning source: %w", err)
		}
		sources = append(sources, src)
	}
	return sources, rows.Err()
}

// CreateSource inserts a new source and links it to sections.
func (s *Store) CreateSource(ctx context.Context, src *models.Source, sectionIDs []string) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	err = tx.QueryRow(ctx, `
		INSERT INTO sources (source_type, name, config, enabled)
		VALUES ($1, $2, $3, $4) RETURNING id`,
		src.SourceType, src.Name, src.Config, src.Enabled,
	).Scan(&src.ID)
	if err != nil {
		return fmt.Errorf("inserting source: %w", err)
	}

	for _, secID := range sectionIDs {
		_, err = tx.Exec(ctx, `INSERT INTO source_sections (source_id, section_id) VALUES ($1, $2)`,
			src.ID, secID)
		if err != nil {
			return fmt.Errorf("linking source to section: %w", err)
		}
	}

	return tx.Commit(ctx)
}

// UpdateSource updates a source's config and enabled state.
func (s *Store) UpdateSource(ctx context.Context, src *models.Source) error {
	_, err := s.pool.Exec(ctx, `
		UPDATE sources SET name = $1, config = $2, enabled = $3 WHERE id = $4`,
		src.Name, src.Config, src.Enabled, src.ID)
	return err
}

// UpdateSourceFetchStatus records the result of a fetch attempt.
func (s *Store) UpdateSourceFetchStatus(ctx context.Context, id string, fetchErr error) error {
	now := time.Now()
	if fetchErr == nil {
		_, err := s.pool.Exec(ctx, `
			UPDATE sources SET last_fetched_at = $1, error_count = 0, last_error = NULL WHERE id = $2`,
			now, id)
		return err
	}
	_, err := s.pool.Exec(ctx, `
		UPDATE sources SET last_fetched_at = $1, error_count = error_count + 1, last_error = $2 WHERE id = $3`,
		now, fetchErr.Error(), id)
	return err
}

// GetSourcesBySection returns all enabled sources linked to a section.
func (s *Store) GetSourcesBySection(ctx context.Context, sectionID string) ([]*models.Source, error) {
	enabled := true
	return s.ListSources(ctx, models.SourceFilter{SectionID: &sectionID, Enabled: &enabled})
}
