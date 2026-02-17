package dedup

import (
	"crypto/rand"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"
)

const (
	// SemanticSimilarityThreshold defines duplicate threshold using cosine similarity.
	SemanticSimilarityThreshold = 0.85
	// SemanticNeighborsLimit controls how many nearest neighbors are checked.
	SemanticNeighborsLimit = 5
)

// SemanticArticle is the minimal shape used by semantic dedup clustering.
type SemanticArticle struct {
	ID         string
	Title      string
	SourceType string
	Similarity float64
	IngestedAt time.Time
	Metadata   json.RawMessage
}

// SemanticClusterResult describes a semantic dedup cluster update.
type SemanticClusterResult struct {
	ClusterID       string
	PrimaryID       string
	MemberIDs       []string
	MatchedIDs      []string
	MetadataUpdates map[string]json.RawMessage
}

// SemanticClusterer performs semantic duplicate clustering and metadata updates.
type SemanticClusterer struct {
	threshold float64
}

// NewSemanticClusterer creates a clusterer with default threshold.
func NewSemanticClusterer() *SemanticClusterer {
	return &SemanticClusterer{threshold: SemanticSimilarityThreshold}
}

// Cluster builds metadata updates for articles in a semantic cluster.
// Returns (result, true, nil) when clustering was applied.
func (c *SemanticClusterer) Cluster(current SemanticArticle, neighbors []SemanticArticle) (*SemanticClusterResult, bool, error) {
	if strings.TrimSpace(current.ID) == "" {
		return nil, false, fmt.Errorf("semantic dedup requires current article ID")
	}

	matches := make([]SemanticArticle, 0, len(neighbors))
	for _, candidate := range neighbors {
		if strings.TrimSpace(candidate.ID) == "" {
			continue
		}
		if candidate.Similarity <= c.threshold {
			continue
		}
		matches = append(matches, candidate)
	}
	if len(matches) == 0 {
		return nil, false, nil
	}

	members := make([]SemanticArticle, 0, len(matches)+1)
	members = append(members, current)
	members = append(members, matches...)

	clusterID := pickClusterID(members)
	if clusterID == "" {
		clusterID = newClusterID()
	}

	primary := pickPrimaryArticle(members)
	if primary.ID == "" {
		return nil, false, fmt.Errorf("semantic dedup could not resolve primary article")
	}

	metadataUpdates := make(map[string]json.RawMessage, len(members))
	memberIDs := make([]string, 0, len(members))
	matchedIDs := make([]string, 0, len(matches))
	seen := make(map[string]struct{}, len(members))

	for _, match := range matches {
		matchedIDs = append(matchedIDs, match.ID)
	}

	for _, member := range members {
		if _, ok := seen[member.ID]; ok {
			continue
		}
		seen[member.ID] = struct{}{}
		memberIDs = append(memberIDs, member.ID)

		meta := decodeMetadata(member.Metadata)
		meta["cluster_id"] = clusterID
		meta["cluster_primary_id"] = primary.ID
		meta["is_duplicate"] = member.ID != primary.ID

		raw, err := json.Marshal(meta)
		if err != nil {
			return nil, false, fmt.Errorf("marshalling semantic cluster metadata for %s: %w", member.ID, err)
		}
		metadataUpdates[member.ID] = raw
	}

	sort.Strings(memberIDs)
	sort.Strings(matchedIDs)

	return &SemanticClusterResult{
		ClusterID:       clusterID,
		PrimaryID:       primary.ID,
		MemberIDs:       memberIDs,
		MatchedIDs:      matchedIDs,
		MetadataUpdates: metadataUpdates,
	}, true, nil
}

func pickClusterID(candidates []SemanticArticle) string {
	type existingCluster struct {
		id         string
		ingestedAt time.Time
	}
	clusters := make([]existingCluster, 0, len(candidates))

	for _, candidate := range candidates {
		meta := decodeMetadata(candidate.Metadata)
		clusterID := strings.TrimSpace(stringFromMap(meta, "cluster_id"))
		if clusterID == "" {
			continue
		}
		clusters = append(clusters, existingCluster{id: clusterID, ingestedAt: candidate.IngestedAt})
	}

	if len(clusters) == 0 {
		return ""
	}

	sort.Slice(clusters, func(i, j int) bool {
		if clusters[i].ingestedAt.Equal(clusters[j].ingestedAt) {
			return clusters[i].id < clusters[j].id
		}
		return clusters[i].ingestedAt.Before(clusters[j].ingestedAt)
	})

	return clusters[0].id
}

func pickPrimaryArticle(candidates []SemanticArticle) SemanticArticle {
	if len(candidates) == 0 {
		return SemanticArticle{}
	}

	best := candidates[0]
	bestSignal := signalScore(best)

	for i := 1; i < len(candidates); i++ {
		candidate := candidates[i]
		candidateSignal := signalScore(candidate)

		if candidateSignal > bestSignal {
			best = candidate
			bestSignal = candidateSignal
			continue
		}
		if candidateSignal < bestSignal {
			continue
		}

		if candidate.IngestedAt.Before(best.IngestedAt) {
			best = candidate
			continue
		}
		if candidate.IngestedAt.Equal(best.IngestedAt) && candidate.ID < best.ID {
			best = candidate
		}
	}

	return best
}

func signalScore(article SemanticArticle) float64 {
	meta := decodeMetadata(article.Metadata)
	hn := floatFromMap(meta, "hn_score")
	reddit := floatFromMap(meta, "reddit_score")
	if hn > reddit {
		return hn
	}
	return reddit
}

func decodeMetadata(raw json.RawMessage) map[string]interface{} {
	if len(raw) == 0 || string(raw) == "null" {
		return map[string]interface{}{}
	}

	out := map[string]interface{}{}
	if err := json.Unmarshal(raw, &out); err != nil {
		return map[string]interface{}{}
	}
	return out
}

func stringFromMap(meta map[string]interface{}, key string) string {
	if meta == nil {
		return ""
	}
	value, ok := meta[key]
	if !ok {
		return ""
	}
	str, _ := value.(string)
	return strings.TrimSpace(str)
}

func floatFromMap(meta map[string]interface{}, key string) float64 {
	if meta == nil {
		return 0
	}
	value, ok := meta[key]
	if !ok {
		return 0
	}

	switch typed := value.(type) {
	case float64:
		return typed
	case float32:
		return float64(typed)
	case int:
		return float64(typed)
	case int64:
		return float64(typed)
	case int32:
		return float64(typed)
	default:
		return 0
	}
}

func newClusterID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return fmt.Sprintf("cluster-%d", time.Now().UnixNano())
	}

	// RFC 4122 v4
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80

	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}
