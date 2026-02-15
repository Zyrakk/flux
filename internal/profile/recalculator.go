package profile

import (
	"context"
	"fmt"
	"strings"

	"github.com/zyrak/flux/internal/embeddings"
	"github.com/zyrak/flux/internal/models"
	"github.com/zyrak/flux/internal/store"
)

// Recalculator computes section profile vectors from user feedback.
type Recalculator struct {
	store        *store.Store
	embedClient  *embeddings.Client
	recentWeight float32
}

// NewRecalculator creates a new section profile recalculator.
func NewRecalculator(st *store.Store, embedClient *embeddings.Client, recentWeight float32) *Recalculator {
	if recentWeight <= 0 || recentWeight >= 1 {
		recentWeight = 0.7
	}
	return &Recalculator{
		store:        st,
		embedClient:  embedClient,
		recentWeight: recentWeight,
	}
}

// RecalculateSection refreshes one section profile using current feedback and EMA blending.
func (r *Recalculator) RecalculateSection(ctx context.Context, sectionID string) error {
	sec, err := r.store.GetSectionByID(ctx, sectionID)
	if err != nil {
		return fmt.Errorf("loading section %s: %w", sectionID, err)
	}
	if sec == nil {
		return fmt.Errorf("section %s not found", sectionID)
	}

	profile, err := r.store.GetSectionProfile(ctx, sectionID)
	if err != nil {
		return fmt.Errorf("loading section profile %s: %w", sectionID, err)
	}
	if profile == nil {
		profile = &models.SectionProfile{SectionID: sectionID}
	}

	likeVectors, err := r.store.ListSectionEmbeddingsByFeedbackAction(ctx, sectionID, models.ActionLike)
	if err != nil {
		return fmt.Errorf("listing like embeddings for section %s: %w", sectionID, err)
	}
	dislikeVectors, err := r.store.ListSectionEmbeddingsByFeedbackAction(ctx, sectionID, models.ActionDislike)
	if err != nil {
		return fmt.Errorf("listing dislike embeddings for section %s: %w", sectionID, err)
	}

	var seedEmbedding []float32
	if len(likeVectors) == 0 || len(profile.PositiveEmbedding) == 0 {
		seedEmbedding, err = r.embedSeedKeywords(ctx, sec)
		if err != nil {
			return fmt.Errorf("embedding seed keywords for section %s: %w", sectionID, err)
		}
	}

	positive := r.recalculatePositive(profile.PositiveEmbedding, seedEmbedding, likeVectors)
	negative := r.recalculateNegative(profile.NegativeEmbedding, dislikeVectors)

	likes, dislikes, err := r.store.CountFeedbackBySection(ctx, sectionID)
	if err != nil {
		return fmt.Errorf("counting feedback for section %s: %w", sectionID, err)
	}

	updated := &models.SectionProfile{
		SectionID:         sectionID,
		PositiveEmbedding: positive,
		NegativeEmbedding: negative,
		LikeCount:         likes,
		DislikeCount:      dislikes,
	}
	if err := r.store.UpsertSectionProfile(ctx, updated); err != nil {
		return fmt.Errorf("upserting section profile %s: %w", sectionID, err)
	}

	return nil
}

// RecalculateAllSections refreshes section profiles for every configured section.
func (r *Recalculator) RecalculateAllSections(ctx context.Context) error {
	sections, err := r.store.ListSections(ctx)
	if err != nil {
		return fmt.Errorf("listing sections for profile recalculation: %w", err)
	}
	for _, sec := range sections {
		if err := r.RecalculateSection(ctx, sec.ID); err != nil {
			return err
		}
	}
	return nil
}

func (r *Recalculator) recalculatePositive(existing, seed []float32, likeVectors [][]float32) []float32 {
	if len(likeVectors) == 0 {
		if len(seed) > 0 {
			return seed
		}
		return existing
	}

	recent := averageVector(likeVectors)
	history := existing
	if len(history) == 0 {
		history = seed
	}
	return blendVectors(recent, history, r.recentWeight)
}

func (r *Recalculator) recalculateNegative(existing []float32, dislikeVectors [][]float32) []float32 {
	if len(dislikeVectors) == 0 {
		return existing
	}

	recent := averageVector(dislikeVectors)
	return blendVectors(recent, existing, r.recentWeight)
}

func (r *Recalculator) embedSeedKeywords(ctx context.Context, section *models.Section) ([]float32, error) {
	keywords := make([]string, 0, len(section.SeedKeywords))
	for _, kw := range section.SeedKeywords {
		kw = strings.TrimSpace(kw)
		if kw != "" {
			keywords = append(keywords, kw)
		}
	}
	if len(keywords) == 0 {
		return nil, nil
	}

	embs, err := r.embedClient.Embed(ctx, keywords)
	if err != nil {
		return nil, err
	}
	if len(embs) == 0 {
		return nil, nil
	}
	return averageVector(embs), nil
}

func averageVector(vectors [][]float32) []float32 {
	if len(vectors) == 0 {
		return nil
	}
	dim := len(vectors[0])
	if dim == 0 {
		return nil
	}

	out := make([]float32, dim)
	count := float32(0)
	for _, v := range vectors {
		if len(v) != dim {
			continue
		}
		for i := 0; i < dim; i++ {
			out[i] += v[i]
		}
		count++
	}
	if count == 0 {
		return nil
	}
	for i := 0; i < dim; i++ {
		out[i] /= count
	}
	return out
}

func blendVectors(recent, historical []float32, recentWeight float32) []float32 {
	if len(recent) == 0 {
		return historical
	}
	if len(historical) == 0 || len(historical) != len(recent) {
		return recent
	}

	historicalWeight := 1 - recentWeight
	out := make([]float32, len(recent))
	for i := range recent {
		out[i] = (recent[i] * recentWeight) + (historical[i] * historicalWeight)
	}
	return out
}
