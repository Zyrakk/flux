package store

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zyrak/flux/internal/models"
)

func TestSelectArticlesForBriefing_OnlyReadyPending(t *testing.T) {
	ctx, s := setupStoreTestDB(t)

	sectionID := "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa"
	now := time.Now().UTC()

	scoreProcessed := 0.95
	scoreReady := 0.91

	fixtures := []struct {
		id             string
		sourceID       string
		status         string
		relevanceScore *float64
		ingestedAt     time.Time
	}{
		{
			id:             "11111111-1111-1111-1111-111111111111",
			sourceID:       "case-1-pending-without-score",
			status:         models.StatusPending,
			relevanceScore: nil,
			ingestedAt:     now.Add(-3 * time.Minute),
		},
		{
			id:             "22222222-2222-2222-2222-222222222222",
			sourceID:       "case-2-processed-with-score",
			status:         models.StatusProcessed,
			relevanceScore: &scoreProcessed,
			ingestedAt:     now.Add(-2 * time.Minute),
		},
		{
			id:             "33333333-3333-3333-3333-333333333333",
			sourceID:       "case-3-pending-with-score",
			status:         models.StatusPending,
			relevanceScore: &scoreReady,
			ingestedAt:     now.Add(-1 * time.Minute),
		},
	}

	for _, fixture := range fixtures {
		var score any
		if fixture.relevanceScore != nil {
			score = *fixture.relevanceScore
		}

		_, err := s.pool.Exec(ctx, `
			INSERT INTO articles (
				id, source_type, source_id, section_id, url, title, ingested_at, relevance_score, status
			)
			VALUES ($1::uuid, 'rss', $2, $3::uuid, $4, $5, $6, $7, $8)
		`,
			fixture.id,
			fixture.sourceID,
			sectionID,
			"https://example.com/"+fixture.sourceID,
			"title-"+fixture.sourceID,
			fixture.ingestedAt,
			score,
			fixture.status,
		)
		require.NoError(t, err)
	}

	selected, total, err := s.ListPendingArticlesForSection(ctx, sectionID, 0, 10, 0)
	require.NoError(t, err)
	require.Equal(t, 1, total)
	require.Len(t, selected, 1)
	assert.Equal(t, fixtures[2].id, selected[0].ID)
	assert.Equal(t, models.StatusPending, selected[0].Status)
	require.NotNil(t, selected[0].RelevanceScore)
	assert.InDelta(t, scoreReady, *selected[0].RelevanceScore, 1e-9)
}
