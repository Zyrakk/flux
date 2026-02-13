package store

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/pgvector/pgvector-go"
	"github.com/zyrak/flux/internal/models"
)

// GetSectionProfile retrieves the relevance profile for a section.
func (s *Store) GetSectionProfile(ctx context.Context, sectionID string) (*models.SectionProfile, error) {
	sp := &models.SectionProfile{}

	var posVec, negVec *pgvector.Vector

	err := s.pool.QueryRow(ctx, `
		SELECT section_id, positive_embedding, negative_embedding, like_count, dislike_count, updated_at
		FROM section_profiles WHERE section_id = $1`, sectionID).
		Scan(&sp.SectionID, &posVec, &negVec, &sp.LikeCount, &sp.DislikeCount, &sp.UpdatedAt)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("getting section profile %s: %w", sectionID, err)
	}

	if posVec != nil {
		sp.PositiveEmbedding = posVec.Slice()
	}
	if negVec != nil {
		sp.NegativeEmbedding = negVec.Slice()
	}

	return sp, nil
}

// UpsertSectionProfile creates or updates the relevance profile for a section.
func (s *Store) UpsertSectionProfile(ctx context.Context, sp *models.SectionProfile) error {
	var posVec, negVec *pgvector.Vector
	if len(sp.PositiveEmbedding) > 0 {
		v := pgvector.NewVector(sp.PositiveEmbedding)
		posVec = &v
	}
	if len(sp.NegativeEmbedding) > 0 {
		v := pgvector.NewVector(sp.NegativeEmbedding)
		negVec = &v
	}

	_, err := s.pool.Exec(ctx, `
		INSERT INTO section_profiles (section_id, positive_embedding, negative_embedding, like_count, dislike_count, updated_at)
		VALUES ($1, $2, $3, $4, $5, NOW())
		ON CONFLICT (section_id)
		DO UPDATE SET
			positive_embedding = EXCLUDED.positive_embedding,
			negative_embedding = EXCLUDED.negative_embedding,
			like_count = EXCLUDED.like_count,
			dislike_count = EXCLUDED.dislike_count,
			updated_at = NOW()`,
		sp.SectionID, posVec, negVec, sp.LikeCount, sp.DislikeCount)
	return err
}
