package store

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/zyrak/flux/internal/models"
)

// ArticleListQuery holds filters and pagination for listing articles.
type ArticleListQuery struct {
	SectionName *string
	SourceType  *string
	Status      *string
	From        *time.Time
	To          *time.Time
	Limit       int
	Offset      int
}

// ArticleWithRelations contains article data plus section/source labels for API responses.
type ArticleWithRelations struct {
	models.Article
	SectionName        *string `json:"section_name,omitempty"`
	SectionDisplayName *string `json:"section_display_name,omitempty"`
	SourceName         string  `json:"source_name"`
	SourceRef          *string `json:"source_ref,omitempty"`
}

// ListArticlesWithRelations returns paginated articles and total count with section/source labels.
func (s *Store) ListArticlesWithRelations(ctx context.Context, q ArticleListQuery) ([]*ArticleWithRelations, int, error) {
	limit := q.Limit
	if limit <= 0 {
		limit = 20
	}

	conditions := []string{}
	args := []interface{}{}
	argIdx := 1

	if q.SectionName != nil {
		conditions = append(conditions, fmt.Sprintf("sec.name = $%d", argIdx))
		args = append(args, *q.SectionName)
		argIdx++
	}
	if q.SourceType != nil {
		conditions = append(conditions, fmt.Sprintf("a.source_type = $%d", argIdx))
		args = append(args, *q.SourceType)
		argIdx++
	}
	if q.Status != nil {
		conditions = append(conditions, fmt.Sprintf("a.status = $%d", argIdx))
		args = append(args, *q.Status)
		argIdx++
	}
	if q.From != nil {
		conditions = append(conditions, fmt.Sprintf("a.ingested_at >= $%d", argIdx))
		args = append(args, *q.From)
		argIdx++
	}
	if q.To != nil {
		conditions = append(conditions, fmt.Sprintf("a.ingested_at <= $%d", argIdx))
		args = append(args, *q.To)
		argIdx++
	}

	where := ""
	if len(conditions) > 0 {
		where = " WHERE " + strings.Join(conditions, " AND ")
	}

	countQuery := `
		SELECT COUNT(*)
		FROM articles a
		LEFT JOIN sections sec ON sec.id = a.section_id` + where

	var total int
	if err := s.pool.QueryRow(ctx, countQuery, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("counting articles: %w", err)
	}

	query := fmt.Sprintf(`
		SELECT
			a.id, a.source_type, a.source_id, a.section_id, a.url, a.title, a.content, a.summary,
			a.author, a.published_at, a.ingested_at, a.processed_at, a.relevance_score,
			a.categories, a.status, a.metadata,
			sec.name, sec.display_name,
			COALESCE(NULLIF(a.metadata->>'source_name', ''), CASE WHEN a.source_type = 'hn' THEN 'Hacker News' ELSE a.source_type END) AS source_name,
			NULLIF(a.metadata->>'source_ref', '') AS source_ref
		FROM articles a
		LEFT JOIN sections sec ON sec.id = a.section_id
		%s
		ORDER BY a.ingested_at DESC
		LIMIT $%d OFFSET $%d`, where, argIdx, argIdx+1)

	args = append(args, limit, q.Offset)

	rows, err := s.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("listing articles with relations: %w", err)
	}
	defer rows.Close()

	var out []*ArticleWithRelations
	for rows.Next() {
		a := &ArticleWithRelations{}
		if err := rows.Scan(
			&a.ID, &a.SourceType, &a.SourceID, &a.SectionID, &a.URL, &a.Title, &a.Content, &a.Summary,
			&a.Author, &a.PublishedAt, &a.IngestedAt, &a.ProcessedAt, &a.RelevanceScore,
			&a.Categories, &a.Status, &a.Metadata,
			&a.SectionName, &a.SectionDisplayName,
			&a.SourceName, &a.SourceRef,
		); err != nil {
			return nil, 0, fmt.Errorf("scanning article with relations: %w", err)
		}
		out = append(out, a)
	}

	return out, total, rows.Err()
}

// GetArticleWithRelationsByID returns a single article enriched with section/source labels.
func (s *Store) GetArticleWithRelationsByID(ctx context.Context, id string) (*ArticleWithRelations, error) {
	query := `
		SELECT
			a.id, a.source_type, a.source_id, a.section_id, a.url, a.title, a.content, a.summary,
			a.author, a.published_at, a.ingested_at, a.processed_at, a.relevance_score,
			a.categories, a.status, a.metadata,
			sec.name, sec.display_name,
			COALESCE(NULLIF(a.metadata->>'source_name', ''), CASE WHEN a.source_type = 'hn' THEN 'Hacker News' ELSE a.source_type END) AS source_name,
			NULLIF(a.metadata->>'source_ref', '') AS source_ref
		FROM articles a
		LEFT JOIN sections sec ON sec.id = a.section_id
		WHERE a.id = $1`

	a := &ArticleWithRelations{}
	err := s.pool.QueryRow(ctx, query, id).Scan(
		&a.ID, &a.SourceType, &a.SourceID, &a.SectionID, &a.URL, &a.Title, &a.Content, &a.Summary,
		&a.Author, &a.PublishedAt, &a.IngestedAt, &a.ProcessedAt, &a.RelevanceScore,
		&a.Categories, &a.Status, &a.Metadata,
		&a.SectionName, &a.SectionDisplayName,
		&a.SourceName, &a.SourceRef,
	)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("getting article with relations by id: %w", err)
	}
	return a, nil
}

// SourceSectionRef is a lightweight section projection used in source list responses.
type SourceSectionRef struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	DisplayName string `json:"display_name"`
}

// SourceWithSections contains a source plus linked sections.
type SourceWithSections struct {
	Source   *models.Source     `json:"source"`
	Sections []SourceSectionRef `json:"sections"`
}

// ListSourcesWithSections returns all sources with linked section details.
func (s *Store) ListSourcesWithSections(ctx context.Context) ([]*SourceWithSections, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT
			s.id, s.source_type, s.name, s.config, s.enabled, s.last_fetched_at, s.error_count, s.last_error,
			sec.id, sec.name, sec.display_name
		FROM sources s
		LEFT JOIN source_sections ss ON ss.source_id = s.id
		LEFT JOIN sections sec ON sec.id = ss.section_id
		ORDER BY s.name, sec.sort_order`)
	if err != nil {
		return nil, fmt.Errorf("listing sources with sections: %w", err)
	}
	defer rows.Close()

	var out []*SourceWithSections
	byID := make(map[string]*SourceWithSections)

	for rows.Next() {
		src := &models.Source{}
		var sectionID, sectionName, sectionDisplayName *string
		if err := rows.Scan(
			&src.ID, &src.SourceType, &src.Name, &src.Config, &src.Enabled, &src.LastFetchedAt, &src.ErrorCount, &src.LastError,
			&sectionID, &sectionName, &sectionDisplayName,
		); err != nil {
			return nil, fmt.Errorf("scanning source with sections: %w", err)
		}

		entry, ok := byID[src.ID]
		if !ok {
			entry = &SourceWithSections{
				Source:   src,
				Sections: []SourceSectionRef{},
			}
			byID[src.ID] = entry
			out = append(out, entry)
		}

		if sectionID != nil && sectionName != nil && sectionDisplayName != nil {
			entry.Sections = append(entry.Sections, SourceSectionRef{
				ID:          *sectionID,
				Name:        *sectionName,
				DisplayName: *sectionDisplayName,
			})
		}
	}

	return out, rows.Err()
}

// GetSourceWithSectionsByID returns one source with linked section details.
func (s *Store) GetSourceWithSectionsByID(ctx context.Context, sourceID string) (*SourceWithSections, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT
			s.id, s.source_type, s.name, s.config, s.enabled, s.last_fetched_at, s.error_count, s.last_error,
			sec.id, sec.name, sec.display_name
		FROM sources s
		LEFT JOIN source_sections ss ON ss.source_id = s.id
		LEFT JOIN sections sec ON sec.id = ss.section_id
		WHERE s.id = $1
		ORDER BY sec.sort_order`, sourceID)
	if err != nil {
		return nil, fmt.Errorf("getting source with sections: %w", err)
	}
	defer rows.Close()

	var out *SourceWithSections
	for rows.Next() {
		src := &models.Source{}
		var sectionID, sectionName, sectionDisplayName *string
		if err := rows.Scan(
			&src.ID, &src.SourceType, &src.Name, &src.Config, &src.Enabled, &src.LastFetchedAt, &src.ErrorCount, &src.LastError,
			&sectionID, &sectionName, &sectionDisplayName,
		); err != nil {
			return nil, fmt.Errorf("scanning source with sections: %w", err)
		}

		if out == nil {
			out = &SourceWithSections{
				Source:   src,
				Sections: []SourceSectionRef{},
			}
		}

		if sectionID != nil && sectionName != nil && sectionDisplayName != nil {
			out.Sections = append(out.Sections, SourceSectionRef{
				ID:          *sectionID,
				Name:        *sectionName,
				DisplayName: *sectionDisplayName,
			})
		}
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

// SectionStats contains section counters used by the API.
type SectionStats struct {
	models.Section
	ArticleCount  int `json:"article_count"`
	ActiveSources int `json:"active_sources"`
}

// ListSectionsWithStats returns sections with article/source counters.
func (s *Store) ListSectionsWithStats(ctx context.Context) ([]*SectionStats, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT
			sec.id, sec.name, sec.display_name, sec.enabled, sec.sort_order,
			sec.max_briefing_articles, sec.seed_keywords, sec.config,
			COALESCE(a.article_count, 0) AS article_count,
			COALESCE(src.active_sources, 0) AS active_sources
		FROM sections sec
		LEFT JOIN (
			SELECT section_id, COUNT(*) AS article_count
			FROM articles
			WHERE section_id IS NOT NULL
			GROUP BY section_id
		) a ON a.section_id = sec.id
		LEFT JOIN (
			SELECT ss.section_id, COUNT(DISTINCT s.id) AS active_sources
			FROM source_sections ss
			JOIN sources s ON s.id = ss.source_id
			WHERE s.enabled = TRUE
			GROUP BY ss.section_id
		) src ON src.section_id = sec.id
		ORDER BY sec.sort_order`)
	if err != nil {
		return nil, fmt.Errorf("listing sections with stats: %w", err)
	}
	defer rows.Close()

	var out []*SectionStats
	for rows.Next() {
		sec := &SectionStats{}
		var cfg json.RawMessage
		if err := rows.Scan(
			&sec.ID, &sec.Name, &sec.DisplayName, &sec.Enabled, &sec.SortOrder,
			&sec.MaxBriefingArticles, &sec.SeedKeywords, &cfg,
			&sec.ArticleCount, &sec.ActiveSources,
		); err != nil {
			return nil, fmt.Errorf("scanning section stats: %w", err)
		}
		sec.Config = cfg
		out = append(out, sec)
	}

	return out, rows.Err()
}
