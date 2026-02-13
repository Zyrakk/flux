package store

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/pgvector/pgvector-go"
	"github.com/zyrak/flux/internal/models"
)

// CreateArticle inserts a new article.
func (s *Store) CreateArticle(ctx context.Context, a *models.Article) error {
	query := `
		INSERT INTO articles (source_type, source_id, section_id, url, title, content, summary,
			author, published_at, embedding, relevance_score, categories, status, metadata)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14)
		RETURNING id, ingested_at`

	var emb *pgvector.Vector
	if len(a.Embedding) > 0 {
		v := pgvector.NewVector(a.Embedding)
		emb = &v
	}

	return s.pool.QueryRow(ctx, query,
		a.SourceType, a.SourceID, a.SectionID, a.URL, a.Title, a.Content, a.Summary,
		a.Author, a.PublishedAt, emb, a.RelevanceScore, a.Categories, a.Status, a.Metadata,
	).Scan(&a.ID, &a.IngestedAt)
}

// GetArticleByID retrieves a single article by ID.
func (s *Store) GetArticleByID(ctx context.Context, id string) (*models.Article, error) {
	query := `
		SELECT id, source_type, source_id, section_id, url, title, content, summary,
			author, published_at, ingested_at, processed_at, relevance_score,
			categories, status, metadata
		FROM articles WHERE id = $1`

	a := &models.Article{}
	err := s.pool.QueryRow(ctx, query, id).Scan(
		&a.ID, &a.SourceType, &a.SourceID, &a.SectionID, &a.URL, &a.Title, &a.Content,
		&a.Summary, &a.Author, &a.PublishedAt, &a.IngestedAt, &a.ProcessedAt,
		&a.RelevanceScore, &a.Categories, &a.Status, &a.Metadata,
	)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("getting article %s: %w", id, err)
	}
	return a, nil
}

// ListArticles returns articles matching the given filter.
func (s *Store) ListArticles(ctx context.Context, f models.ArticleFilter) ([]*models.Article, error) {
	var conditions []string
	var args []interface{}
	argIdx := 1

	if f.SectionID != nil {
		conditions = append(conditions, fmt.Sprintf("section_id = $%d", argIdx))
		args = append(args, *f.SectionID)
		argIdx++
	}
	if f.SourceType != nil {
		conditions = append(conditions, fmt.Sprintf("source_type = $%d", argIdx))
		args = append(args, *f.SourceType)
		argIdx++
	}
	if f.Status != nil {
		conditions = append(conditions, fmt.Sprintf("status = $%d", argIdx))
		args = append(args, *f.Status)
		argIdx++
	}
	if f.Since != nil {
		conditions = append(conditions, fmt.Sprintf("ingested_at >= $%d", argIdx))
		args = append(args, *f.Since)
		argIdx++
	}
	if f.Until != nil {
		conditions = append(conditions, fmt.Sprintf("ingested_at <= $%d", argIdx))
		args = append(args, *f.Until)
		argIdx++
	}

	where := ""
	if len(conditions) > 0 {
		where = "WHERE " + strings.Join(conditions, " AND ")
	}

	limit := f.Limit
	if limit <= 0 {
		limit = 50
	}

	query := fmt.Sprintf(`
		SELECT id, source_type, source_id, section_id, url, title, content, summary,
			author, published_at, ingested_at, processed_at, relevance_score,
			categories, status, metadata
		FROM articles %s
		ORDER BY ingested_at DESC
		LIMIT $%d OFFSET $%d`, where, argIdx, argIdx+1)

	args = append(args, limit, f.Offset)

	rows, err := s.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("listing articles: %w", err)
	}
	defer rows.Close()

	var articles []*models.Article
	for rows.Next() {
		a := &models.Article{}
		if err := rows.Scan(
			&a.ID, &a.SourceType, &a.SourceID, &a.SectionID, &a.URL, &a.Title, &a.Content,
			&a.Summary, &a.Author, &a.PublishedAt, &a.IngestedAt, &a.ProcessedAt,
			&a.RelevanceScore, &a.Categories, &a.Status, &a.Metadata,
		); err != nil {
			return nil, fmt.Errorf("scanning article: %w", err)
		}
		articles = append(articles, a)
	}
	return articles, rows.Err()
}

// UpdateArticleStatus updates the status of an article.
func (s *Store) UpdateArticleStatus(ctx context.Context, id, status string) error {
	var processedAt *time.Time
	if status == models.StatusProcessed || status == models.StatusBriefed {
		now := time.Now()
		processedAt = &now
	}

	_, err := s.pool.Exec(ctx,
		`UPDATE articles SET status = $1, processed_at = COALESCE($2, processed_at) WHERE id = $3`,
		status, processedAt, id)
	return err
}

// UpdateArticleEmbedding sets the embedding vector for an article.
func (s *Store) UpdateArticleEmbedding(ctx context.Context, id string, embedding []float32) error {
	v := pgvector.NewVector(embedding)
	_, err := s.pool.Exec(ctx,
		`UPDATE articles SET embedding = $1 WHERE id = $2`, v, id)
	return err
}

// UpdateArticleSection assigns an article to a section with a relevance score.
func (s *Store) UpdateArticleSection(ctx context.Context, id, sectionID string, score float64) error {
	_, err := s.pool.Exec(ctx,
		`UPDATE articles SET section_id = $1, relevance_score = $2 WHERE id = $3`,
		sectionID, score, id)
	return err
}

// UpdateArticleSummary stores the LLM-generated summary.
func (s *Store) UpdateArticleSummary(ctx context.Context, id, summary string, categories []string) error {
	_, err := s.pool.Exec(ctx,
		`UPDATE articles SET summary = $1, categories = $2 WHERE id = $3`,
		summary, categories, id)
	return err
}
