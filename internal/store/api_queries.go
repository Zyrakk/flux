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
	SectionName  *string
	SectionNames []string
	SourceType   *string
	SourceRef    *string
	Status       *string
	LikedOnly    bool
	From         *time.Time
	To           *time.Time
	Limit        int
	Offset       int
}

// ArticleWithRelations contains article data plus section/source labels for API responses.
type ArticleWithRelations struct {
	models.Article
	SectionName        *string `json:"section_name,omitempty"`
	SectionDisplayName *string `json:"section_display_name,omitempty"`
	SourceName         string  `json:"source_name"`
	SourceRef          *string `json:"source_ref,omitempty"`
	LikeCount          int     `json:"like_count"`
	DislikeCount       int     `json:"dislike_count"`
	SaveCount          int     `json:"save_count"`
	Liked              bool    `json:"liked"`
	Disliked           bool    `json:"disliked"`
	Saved              bool    `json:"saved"`
	LatestLikeID       *string `json:"latest_like_id,omitempty"`
	LatestDislikeID    *string `json:"latest_dislike_id,omitempty"`
	LatestSaveID       *string `json:"latest_save_id,omitempty"`
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
	if len(q.SectionNames) > 0 {
		conditions = append(conditions, fmt.Sprintf("sec.name = ANY($%d)", argIdx))
		args = append(args, q.SectionNames)
		argIdx++
	}
	if q.SourceType != nil {
		conditions = append(conditions, fmt.Sprintf("a.source_type = $%d", argIdx))
		args = append(args, *q.SourceType)
		argIdx++
	}
	if q.SourceRef != nil {
		conditions = append(conditions, fmt.Sprintf("a.metadata->>'source_ref' = $%d", argIdx))
		args = append(args, *q.SourceRef)
		argIdx++
	}
	if q.Status != nil {
		conditions = append(conditions, fmt.Sprintf("a.status = $%d", argIdx))
		args = append(args, *q.Status)
		argIdx++
	}
	if q.LikedOnly {
		conditions = append(conditions, "EXISTS (SELECT 1 FROM feedback f WHERE f.article_id = a.id AND f.action = 'like')")
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
			NULLIF(a.metadata->>'source_ref', '') AS source_ref,
			COALESCE(fstats.like_count, 0) AS like_count,
			COALESCE(fstats.dislike_count, 0) AS dislike_count,
			COALESCE(fstats.save_count, 0) AS save_count,
			COALESCE(fstats.liked, FALSE) AS liked,
			COALESCE(fstats.disliked, FALSE) AS disliked,
			COALESCE(fstats.saved, FALSE) AS saved,
			fstats.latest_like_id,
			fstats.latest_dislike_id,
			fstats.latest_save_id
		FROM articles a
		LEFT JOIN sections sec ON sec.id = a.section_id
		LEFT JOIN LATERAL (
			SELECT
				COUNT(*) FILTER (WHERE action = 'like') AS like_count,
				COUNT(*) FILTER (WHERE action = 'dislike') AS dislike_count,
				COUNT(*) FILTER (WHERE action = 'save') AS save_count,
				BOOL_OR(action = 'like') AS liked,
				BOOL_OR(action = 'dislike') AS disliked,
				BOOL_OR(action = 'save') AS saved,
				(
					SELECT id::text
					FROM feedback f2
					WHERE f2.article_id = a.id AND f2.action = 'like'
					ORDER BY f2.created_at DESC
					LIMIT 1
				) AS latest_like_id,
				(
					SELECT id::text
					FROM feedback f3
					WHERE f3.article_id = a.id AND f3.action = 'dislike'
					ORDER BY f3.created_at DESC
					LIMIT 1
				) AS latest_dislike_id,
				(
					SELECT id::text
					FROM feedback f4
					WHERE f4.article_id = a.id AND f4.action = 'save'
					ORDER BY f4.created_at DESC
					LIMIT 1
				) AS latest_save_id
			FROM feedback f
			WHERE f.article_id = a.id
		) fstats ON TRUE
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
			&a.LikeCount, &a.DislikeCount, &a.SaveCount, &a.Liked, &a.Disliked, &a.Saved,
			&a.LatestLikeID, &a.LatestDislikeID, &a.LatestSaveID,
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
			NULLIF(a.metadata->>'source_ref', '') AS source_ref,
			COALESCE(fstats.like_count, 0) AS like_count,
			COALESCE(fstats.dislike_count, 0) AS dislike_count,
			COALESCE(fstats.save_count, 0) AS save_count,
			COALESCE(fstats.liked, FALSE) AS liked,
			COALESCE(fstats.disliked, FALSE) AS disliked,
			COALESCE(fstats.saved, FALSE) AS saved,
			fstats.latest_like_id,
			fstats.latest_dislike_id,
			fstats.latest_save_id
		FROM articles a
		LEFT JOIN sections sec ON sec.id = a.section_id
		LEFT JOIN LATERAL (
			SELECT
				COUNT(*) FILTER (WHERE action = 'like') AS like_count,
				COUNT(*) FILTER (WHERE action = 'dislike') AS dislike_count,
				COUNT(*) FILTER (WHERE action = 'save') AS save_count,
				BOOL_OR(action = 'like') AS liked,
				BOOL_OR(action = 'dislike') AS disliked,
				BOOL_OR(action = 'save') AS saved,
				(
					SELECT id::text
					FROM feedback f2
					WHERE f2.article_id = a.id AND f2.action = 'like'
					ORDER BY f2.created_at DESC
					LIMIT 1
				) AS latest_like_id,
				(
					SELECT id::text
					FROM feedback f3
					WHERE f3.article_id = a.id AND f3.action = 'dislike'
					ORDER BY f3.created_at DESC
					LIMIT 1
				) AS latest_dislike_id,
				(
					SELECT id::text
					FROM feedback f4
					WHERE f4.article_id = a.id AND f4.action = 'save'
					ORDER BY f4.created_at DESC
					LIMIT 1
				) AS latest_save_id
			FROM feedback f
			WHERE f.article_id = a.id
		) fstats ON TRUE
		WHERE a.id = $1`

	a := &ArticleWithRelations{}
	err := s.pool.QueryRow(ctx, query, id).Scan(
		&a.ID, &a.SourceType, &a.SourceID, &a.SectionID, &a.URL, &a.Title, &a.Content, &a.Summary,
		&a.Author, &a.PublishedAt, &a.IngestedAt, &a.ProcessedAt, &a.RelevanceScore,
		&a.Categories, &a.Status, &a.Metadata,
		&a.SectionName, &a.SectionDisplayName,
		&a.SourceName, &a.SourceRef,
		&a.LikeCount, &a.DislikeCount, &a.SaveCount, &a.Liked, &a.Disliked, &a.Saved,
		&a.LatestLikeID, &a.LatestDislikeID, &a.LatestSaveID,
	)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("getting article with relations by id: %w", err)
	}
	return a, nil
}

// ListArticlesWithRelationsByIDs returns article details for the provided IDs preserving input order.
func (s *Store) ListArticlesWithRelationsByIDs(ctx context.Context, ids []string) ([]*ArticleWithRelations, error) {
	if len(ids) == 0 {
		return []*ArticleWithRelations{}, nil
	}

	rows, err := s.pool.Query(ctx, `
		WITH input_ids AS (
			SELECT id, ord
			FROM UNNEST($1::uuid[]) WITH ORDINALITY AS t(id, ord)
		)
		SELECT
			a.id, a.source_type, a.source_id, a.section_id, a.url, a.title, a.content, a.summary,
			a.author, a.published_at, a.ingested_at, a.processed_at, a.relevance_score,
			a.categories, a.status, a.metadata,
			sec.name, sec.display_name,
			COALESCE(NULLIF(a.metadata->>'source_name', ''), CASE WHEN a.source_type = 'hn' THEN 'Hacker News' ELSE a.source_type END) AS source_name,
			NULLIF(a.metadata->>'source_ref', '') AS source_ref,
			COALESCE(fstats.like_count, 0) AS like_count,
			COALESCE(fstats.dislike_count, 0) AS dislike_count,
			COALESCE(fstats.save_count, 0) AS save_count,
			COALESCE(fstats.liked, FALSE) AS liked,
			COALESCE(fstats.disliked, FALSE) AS disliked,
			COALESCE(fstats.saved, FALSE) AS saved,
			fstats.latest_like_id,
			fstats.latest_dislike_id,
			fstats.latest_save_id
		FROM input_ids i
		JOIN articles a ON a.id = i.id
		LEFT JOIN sections sec ON sec.id = a.section_id
		LEFT JOIN LATERAL (
			SELECT
				COUNT(*) FILTER (WHERE action = 'like') AS like_count,
				COUNT(*) FILTER (WHERE action = 'dislike') AS dislike_count,
				COUNT(*) FILTER (WHERE action = 'save') AS save_count,
				BOOL_OR(action = 'like') AS liked,
				BOOL_OR(action = 'dislike') AS disliked,
				BOOL_OR(action = 'save') AS saved,
				(
					SELECT id::text
					FROM feedback f2
					WHERE f2.article_id = a.id AND f2.action = 'like'
					ORDER BY f2.created_at DESC
					LIMIT 1
				) AS latest_like_id,
				(
					SELECT id::text
					FROM feedback f3
					WHERE f3.article_id = a.id AND f3.action = 'dislike'
					ORDER BY f3.created_at DESC
					LIMIT 1
				) AS latest_dislike_id,
				(
					SELECT id::text
					FROM feedback f4
					WHERE f4.article_id = a.id AND f4.action = 'save'
					ORDER BY f4.created_at DESC
					LIMIT 1
				) AS latest_save_id
			FROM feedback f
			WHERE f.article_id = a.id
		) fstats ON TRUE
		ORDER BY i.ord`,
		ids,
	)
	if err != nil {
		return nil, fmt.Errorf("listing articles by ids with relations: %w", err)
	}
	defer rows.Close()

	out := make([]*ArticleWithRelations, 0, len(ids))
	for rows.Next() {
		a := &ArticleWithRelations{}
		if err := rows.Scan(
			&a.ID, &a.SourceType, &a.SourceID, &a.SectionID, &a.URL, &a.Title, &a.Content, &a.Summary,
			&a.Author, &a.PublishedAt, &a.IngestedAt, &a.ProcessedAt, &a.RelevanceScore,
			&a.Categories, &a.Status, &a.Metadata,
			&a.SectionName, &a.SectionDisplayName,
			&a.SourceName, &a.SourceRef,
			&a.LikeCount, &a.DislikeCount, &a.SaveCount, &a.Liked, &a.Disliked, &a.Saved,
			&a.LatestLikeID, &a.LatestDislikeID, &a.LatestSaveID,
		); err != nil {
			return nil, fmt.Errorf("scanning article by id with relations: %w", err)
		}
		out = append(out, a)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
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
	Stats    SourceIngestStats  `json:"stats"`
}

// SourceIngestStats summarizes ingestion performance for a source.
type SourceIngestStats struct {
	TotalIngested int     `json:"total_ingested"`
	Last24h       int     `json:"last_24h"`
	PassRatePct   float64 `json:"pass_rate_pct"`
}

// ListSourcesWithSections returns all sources with linked section details.
func (s *Store) ListSourcesWithSections(ctx context.Context) ([]*SourceWithSections, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT
			s.id, s.source_type, s.name, s.config, s.enabled, s.last_fetched_at, s.error_count, s.last_error,
			sec.id, sec.name, sec.display_name,
			COALESCE(stats.total_ingested, 0) AS total_ingested,
			COALESCE(stats.last_24h, 0) AS last_24h,
			COALESCE(stats.pass_rate_pct, 0) AS pass_rate_pct
		FROM sources s
		LEFT JOIN source_sections ss ON ss.source_id = s.id
		LEFT JOIN sections sec ON sec.id = ss.section_id
		LEFT JOIN LATERAL (
			SELECT
				COUNT(*) AS total_ingested,
				COUNT(*) FILTER (WHERE a.ingested_at >= NOW() - INTERVAL '24 hours') AS last_24h,
				COALESCE(
					ROUND(
						(
							COUNT(*) FILTER (
								WHERE a.status IN ('pending', 'processed', 'briefed')
							)::numeric / NULLIF(COUNT(*), 0)::numeric
						) * 100.0,
						2
					),
					0
				) AS pass_rate_pct
			FROM articles a
			WHERE (a.metadata->>'source_ref' = s.id::text)
				OR (s.source_type = 'hn' AND a.source_type = 'hn')
		) stats ON TRUE
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
		var totalIngested, last24h int
		var passRate float64
		if err := rows.Scan(
			&src.ID, &src.SourceType, &src.Name, &src.Config, &src.Enabled, &src.LastFetchedAt, &src.ErrorCount, &src.LastError,
			&sectionID, &sectionName, &sectionDisplayName,
			&totalIngested, &last24h, &passRate,
		); err != nil {
			return nil, fmt.Errorf("scanning source with sections: %w", err)
		}

		entry, ok := byID[src.ID]
		if !ok {
			entry = &SourceWithSections{
				Source:   src,
				Sections: []SourceSectionRef{},
				Stats: SourceIngestStats{
					TotalIngested: totalIngested,
					Last24h:       last24h,
					PassRatePct:   passRate,
				},
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
			sec.id, sec.name, sec.display_name,
			COALESCE(stats.total_ingested, 0) AS total_ingested,
			COALESCE(stats.last_24h, 0) AS last_24h,
			COALESCE(stats.pass_rate_pct, 0) AS pass_rate_pct
		FROM sources s
		LEFT JOIN source_sections ss ON ss.source_id = s.id
		LEFT JOIN sections sec ON sec.id = ss.section_id
		LEFT JOIN LATERAL (
			SELECT
				COUNT(*) AS total_ingested,
				COUNT(*) FILTER (WHERE a.ingested_at >= NOW() - INTERVAL '24 hours') AS last_24h,
				COALESCE(
					ROUND(
						(
							COUNT(*) FILTER (
								WHERE a.status IN ('pending', 'processed', 'briefed')
							)::numeric / NULLIF(COUNT(*), 0)::numeric
						) * 100.0,
						2
					),
					0
				) AS pass_rate_pct
			FROM articles a
			WHERE (a.metadata->>'source_ref' = s.id::text)
				OR (s.source_type = 'hn' AND a.source_type = 'hn')
		) stats ON TRUE
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
		var totalIngested, last24h int
		var passRate float64
		if err := rows.Scan(
			&src.ID, &src.SourceType, &src.Name, &src.Config, &src.Enabled, &src.LastFetchedAt, &src.ErrorCount, &src.LastError,
			&sectionID, &sectionName, &sectionDisplayName,
			&totalIngested, &last24h, &passRate,
		); err != nil {
			return nil, fmt.Errorf("scanning source with sections: %w", err)
		}

		if out == nil {
			out = &SourceWithSections{
				Source:   src,
				Sections: []SourceSectionRef{},
				Stats: SourceIngestStats{
					TotalIngested: totalIngested,
					Last24h:       last24h,
					PassRatePct:   passRate,
				},
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
