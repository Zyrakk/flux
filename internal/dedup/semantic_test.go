package dedup

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSemanticClusterer_NoMatchesAboveThreshold(t *testing.T) {
	clusterer := NewSemanticClusterer()

	result, clustered, err := clusterer.Cluster(
		SemanticArticle{ID: "a1", IngestedAt: time.Now()},
		[]SemanticArticle{{ID: "a2", Similarity: 0.80, IngestedAt: time.Now()}},
	)

	require.NoError(t, err)
	assert.False(t, clustered)
	assert.Nil(t, result)
}

func TestSemanticClusterer_PrimaryBySignal(t *testing.T) {
	clusterer := NewSemanticClusterer()
	now := time.Now().UTC()

	hnMeta, _ := json.Marshal(map[string]interface{}{"hn_score": 142})
	redditMeta, _ := json.Marshal(map[string]interface{}{"reddit_score": 89, "subreddit": "netsec"})

	result, clustered, err := clusterer.Cluster(
		SemanticArticle{ID: "current", IngestedAt: now, Metadata: json.RawMessage(`{"reddit_score": 40}`)},
		[]SemanticArticle{
			{ID: "hn", Similarity: 0.96, IngestedAt: now.Add(-20 * time.Minute), Metadata: hnMeta},
			{ID: "reddit", Similarity: 0.91, IngestedAt: now.Add(-10 * time.Minute), Metadata: redditMeta},
		},
	)

	require.NoError(t, err)
	assert.True(t, clustered)
	require.NotNil(t, result)
	assert.NotEmpty(t, result.ClusterID)
	assert.Equal(t, "hn", result.PrimaryID)
	require.Len(t, result.MemberIDs, 3)

	for _, id := range []string{"current", "hn", "reddit"} {
		raw, ok := result.MetadataUpdates[id]
		require.True(t, ok)
		meta := map[string]interface{}{}
		require.NoError(t, json.Unmarshal(raw, &meta))
		assert.Equal(t, result.ClusterID, meta["cluster_id"])
		assert.Equal(t, result.PrimaryID, meta["cluster_primary_id"])
	}

	hnUpdate := map[string]interface{}{}
	require.NoError(t, json.Unmarshal(result.MetadataUpdates["hn"], &hnUpdate))
	assert.Equal(t, false, hnUpdate["is_duplicate"])

	currentUpdate := map[string]interface{}{}
	require.NoError(t, json.Unmarshal(result.MetadataUpdates["current"], &currentUpdate))
	assert.Equal(t, true, currentUpdate["is_duplicate"])
}

func TestSemanticClusterer_ReusesExistingClusterID(t *testing.T) {
	clusterer := NewSemanticClusterer()
	now := time.Now().UTC()

	existing := "11111111-2222-4333-8444-555555555555"
	metaWithCluster, _ := json.Marshal(map[string]interface{}{
		"cluster_id":     existing,
		"reddit_score":   100,
		"is_duplicate":   true,
		"cluster_origin": "legacy",
	})

	result, clustered, err := clusterer.Cluster(
		SemanticArticle{ID: "new", IngestedAt: now, Metadata: json.RawMessage(`{"reddit_score": 10}`)},
		[]SemanticArticle{{
			ID:         "old",
			Similarity: 0.90,
			IngestedAt: now.Add(-2 * time.Hour),
			Metadata:   metaWithCluster,
		}},
	)

	require.NoError(t, err)
	assert.True(t, clustered)
	require.NotNil(t, result)
	assert.Equal(t, existing, result.ClusterID)
}
