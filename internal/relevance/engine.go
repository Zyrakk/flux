package relevance

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"sync"

	"github.com/zyrak/flux/internal/embeddings"
	"github.com/zyrak/flux/internal/models"
	"github.com/zyrak/flux/internal/store"
)

const (
	sectionThresholdConfigKey = "relevance_threshold"
)

// Config controls relevance scoring and threshold behavior.
type Config struct {
	DefaultThreshold float64
	MinThreshold     float64
	MaxThreshold     float64
	ThresholdStep    float64
	SourceBoosts     map[string]float64
}

// Result is the output of relevance evaluation for a single article.
type Result struct {
	SectionID      string
	SectionName    string
	RelevanceScore float64
	Threshold      float64
	Status         string
	SourceID       string
}

type sectionState struct {
	section       *models.Section
	seedEmbedding []float32
}

// Engine encapsulates section assignment and relevance scoring.
type Engine struct {
	store       *store.Store
	embedClient *embeddings.Client
	cfg         Config

	mu sync.RWMutex

	sectionsByID   map[string]*sectionState
	sectionsByName map[string]*sectionState
	sectionOrder   []string
	thresholds     map[string]float64

	sourceSections map[string][]string
	sourceByType   map[string][]string
	sourceNames    map[string]string
}

// NewEngine initializes section/seed caches and source mappings.
func NewEngine(ctx context.Context, st *store.Store, embedClient *embeddings.Client, cfg Config) (*Engine, error) {
	if cfg.DefaultThreshold <= 0 {
		cfg.DefaultThreshold = 0.30
	}
	if cfg.MinThreshold <= 0 {
		cfg.MinThreshold = 0.15
	}
	if cfg.MaxThreshold <= 0 {
		cfg.MaxThreshold = 0.60
	}
	if cfg.ThresholdStep <= 0 {
		cfg.ThresholdStep = 0.05
	}

	engine := &Engine{
		store:          st,
		embedClient:    embedClient,
		cfg:            cfg,
		sectionsByID:   make(map[string]*sectionState),
		sectionsByName: make(map[string]*sectionState),
		thresholds:     make(map[string]float64),
		sourceSections: make(map[string][]string),
		sourceByType:   make(map[string][]string),
		sourceNames:    make(map[string]string),
	}

	if err := engine.loadSections(ctx); err != nil {
		return nil, err
	}
	if err := engine.loadSources(ctx); err != nil {
		return nil, err
	}

	return engine, nil
}

func (e *Engine) loadSections(ctx context.Context) error {
	sections, err := e.store.ListSections(ctx)
	if err != nil {
		return fmt.Errorf("listing sections: %w", err)
	}

	type keywordRef struct {
		sectionID string
	}
	var allKeywords []string
	var refs []keywordRef

	for _, sec := range sections {
		if !sec.Enabled {
			continue
		}
		state := &sectionState{section: sec}
		e.sectionsByID[sec.ID] = state
		e.sectionsByName[sec.Name] = state
		e.sectionOrder = append(e.sectionOrder, sec.ID)
		e.thresholds[sec.ID] = e.thresholdFromConfig(sec.Config)

		for _, keyword := range sec.SeedKeywords {
			keyword = strings.TrimSpace(keyword)
			if keyword == "" {
				continue
			}
			allKeywords = append(allKeywords, keyword)
			refs = append(refs, keywordRef{sectionID: sec.ID})
		}
	}

	sort.SliceStable(e.sectionOrder, func(i, j int) bool {
		secI := e.sectionsByID[e.sectionOrder[i]].section
		secJ := e.sectionsByID[e.sectionOrder[j]].section
		return secI.SortOrder < secJ.SortOrder
	})

	if len(allKeywords) == 0 {
		return nil
	}

	embs, err := e.embedClient.Embed(ctx, allKeywords)
	if err != nil {
		return fmt.Errorf("embedding section seed keywords: %w", err)
	}
	if len(embs) != len(allKeywords) {
		return fmt.Errorf("seed embeddings count mismatch: expected=%d got=%d", len(allKeywords), len(embs))
	}

	bySection := make(map[string][][]float32)
	for i := range refs {
		bySection[refs[i].sectionID] = append(bySection[refs[i].sectionID], embs[i])
	}
	for sectionID, vectors := range bySection {
		state := e.sectionsByID[sectionID]
		if state == nil {
			continue
		}
		state.seedEmbedding = averageVector(vectors)
	}

	return nil
}

func (e *Engine) loadSources(ctx context.Context) error {
	sources, err := e.store.ListSourcesWithSections(ctx)
	if err != nil {
		return fmt.Errorf("listing sources with sections: %w", err)
	}

	for _, src := range sources {
		if src.Source == nil {
			continue
		}
		sourceID := src.Source.ID
		e.sourceNames[sourceID] = src.Source.Name
		e.sourceByType[src.Source.SourceType] = append(e.sourceByType[src.Source.SourceType], sourceID)

		sectionIDs := make([]string, 0, len(src.Sections))
		for _, sec := range src.Sections {
			sectionIDs = append(sectionIDs, sec.ID)
		}
		e.sourceSections[sourceID] = sectionIDs
	}

	return nil
}

// EvaluateArticle assigns section + relevance score for an article embedding.
func (e *Engine) EvaluateArticle(ctx context.Context, article *models.Article, articleEmbedding []float32) (*Result, error) {
	sectionID, sourceID, err := e.assignSection(article, articleEmbedding)
	if err != nil {
		return nil, err
	}

	state := e.sectionsByID[sectionID]
	if state == nil {
		return nil, fmt.Errorf("assigned unknown section_id=%s", sectionID)
	}

	profile, err := e.store.GetSectionProfile(ctx, sectionID)
	if err != nil {
		return nil, fmt.Errorf("loading section profile %s: %w", sectionID, err)
	}

	positiveEmbedding := state.seedEmbedding
	var negativeEmbedding []float32
	if profile != nil {
		if len(profile.PositiveEmbedding) > 0 {
			positiveEmbedding = profile.PositiveEmbedding
		}
		if len(profile.NegativeEmbedding) > 0 {
			negativeEmbedding = profile.NegativeEmbedding
		}
	}

	positiveScore := embeddings.CosineSimilarity(articleEmbedding, positiveEmbedding)
	negativeScore := embeddings.CosineSimilarity(articleEmbedding, negativeEmbedding)
	sourceBoost := e.resolveSourceBoost(sourceID, article.SourceType)

	relevanceScore := positiveScore - (negativeScore * 0.5) + sourceBoost
	threshold := e.ThresholdBySectionID(sectionID)

	status := models.StatusPending
	if relevanceScore < threshold {
		status = models.StatusArchived
	}

	return &Result{
		SectionID:      sectionID,
		SectionName:    state.section.Name,
		RelevanceScore: relevanceScore,
		Threshold:      threshold,
		Status:         status,
		SourceID:       sourceID,
	}, nil
}

func (e *Engine) assignSection(article *models.Article, articleEmbedding []float32) (sectionID, sourceID string, err error) {
	sourceID = e.resolveSourceID(article)
	var candidateSectionIDs []string
	if sourceID != "" {
		candidateSectionIDs = e.sourceSections[sourceID]
	}

	if len(candidateSectionIDs) == 1 {
		return candidateSectionIDs[0], sourceID, nil
	}
	if len(candidateSectionIDs) == 0 {
		candidateSectionIDs = append(candidateSectionIDs, e.sectionOrder...)
	}

	bestSectionID := ""
	bestScore := -2.0
	for _, secID := range candidateSectionIDs {
		state := e.sectionsByID[secID]
		if state == nil {
			continue
		}
		score := embeddings.CosineSimilarity(articleEmbedding, state.seedEmbedding)
		if score > bestScore {
			bestScore = score
			bestSectionID = secID
		}
	}

	if bestSectionID == "" {
		if len(e.sectionOrder) == 0 {
			return "", sourceID, fmt.Errorf("no enabled sections available")
		}
		bestSectionID = e.sectionOrder[0]
	}

	return bestSectionID, sourceID, nil
}

func (e *Engine) resolveSourceID(article *models.Article) string {
	ref := sourceRefFromMetadata(article.Metadata)
	if ref != "" {
		return ref
	}
	if ids := e.sourceByType[article.SourceType]; len(ids) == 1 {
		return ids[0]
	}
	return ""
}

// AdjustThreshold applies the dynamic threshold rules and persists changes.
func (e *Engine) AdjustThreshold(ctx context.Context, sectionID string) (float64, bool, error) {
	current := e.ThresholdBySectionID(sectionID)
	count, err := e.store.CountPendingAboveThreshold(ctx, sectionID, current)
	if err != nil {
		return current, false, err
	}

	next := current
	if count > 50 {
		next = clamp(current+e.cfg.ThresholdStep, e.cfg.MinThreshold, e.cfg.MaxThreshold)
	} else if count < 5 {
		next = clamp(current-e.cfg.ThresholdStep, e.cfg.MinThreshold, e.cfg.MaxThreshold)
	}

	if next == current {
		return current, false, nil
	}

	if err := e.store.UpdateSectionThreshold(ctx, sectionID, next); err != nil {
		return current, false, err
	}

	e.mu.Lock()
	e.thresholds[sectionID] = next
	e.mu.Unlock()

	return next, true, nil
}

// ThresholdBySectionID returns the current section threshold.
func (e *Engine) ThresholdBySectionID(sectionID string) float64 {
	e.mu.RLock()
	defer e.mu.RUnlock()

	threshold, ok := e.thresholds[sectionID]
	if !ok {
		return e.cfg.DefaultThreshold
	}
	return threshold
}

// ThresholdsBySectionName returns thresholds indexed by section name.
func (e *Engine) ThresholdsBySectionName() map[string]float64 {
	e.mu.RLock()
	defer e.mu.RUnlock()

	out := make(map[string]float64, len(e.sectionsByID))
	for secID, state := range e.sectionsByID {
		out[state.section.Name] = e.thresholds[secID]
	}
	return out
}

// Sections returns enabled sections sorted by sort_order.
func (e *Engine) Sections() []*models.Section {
	e.mu.RLock()
	defer e.mu.RUnlock()

	out := make([]*models.Section, 0, len(e.sectionOrder))
	for _, secID := range e.sectionOrder {
		state := e.sectionsByID[secID]
		if state == nil {
			continue
		}
		out = append(out, state.section)
	}
	return out
}

func (e *Engine) SectionByName(name string) *models.Section {
	e.mu.RLock()
	defer e.mu.RUnlock()
	state := e.sectionsByName[name]
	if state == nil {
		return nil
	}
	return state.section
}

func (e *Engine) resolveSourceBoost(sourceID, sourceType string) float64 {
	if len(e.cfg.SourceBoosts) == 0 {
		return 0
	}

	if sourceID != "" {
		if boost, ok := e.cfg.SourceBoosts["id:"+strings.ToLower(sourceID)]; ok {
			return boost
		}
		sourceName := strings.ToLower(strings.TrimSpace(e.sourceNames[sourceID]))
		if sourceName != "" {
			if boost, ok := e.cfg.SourceBoosts[sourceName]; ok {
				return boost
			}
		}
	}

	sourceTypeKey := strings.ToLower(strings.TrimSpace(sourceType))
	if boost, ok := e.cfg.SourceBoosts[sourceTypeKey]; ok {
		return boost
	}
	if boost, ok := e.cfg.SourceBoosts["source_type:"+sourceTypeKey]; ok {
		return boost
	}
	return 0
}

func (e *Engine) thresholdFromConfig(raw json.RawMessage) float64 {
	threshold := e.cfg.DefaultThreshold
	if len(raw) == 0 || string(raw) == "null" {
		return clamp(threshold, e.cfg.MinThreshold, e.cfg.MaxThreshold)
	}

	var cfg map[string]interface{}
	if err := json.Unmarshal(raw, &cfg); err != nil {
		return clamp(threshold, e.cfg.MinThreshold, e.cfg.MaxThreshold)
	}

	for _, key := range []string{sectionThresholdConfigKey, "threshold"} {
		val, ok := cfg[key]
		if !ok {
			continue
		}
		switch v := val.(type) {
		case float64:
			threshold = v
		case int:
			threshold = float64(v)
		}
		break
	}

	return clamp(threshold, e.cfg.MinThreshold, e.cfg.MaxThreshold)
}

func averageVector(vectors [][]float32) []float32 {
	if len(vectors) == 0 {
		return nil
	}
	dim := len(vectors[0])
	if dim == 0 {
		return nil
	}

	acc := make([]float64, dim)
	valid := 0
	for _, vec := range vectors {
		if len(vec) != dim {
			continue
		}
		valid++
		for i := range vec {
			acc[i] += float64(vec[i])
		}
	}
	if valid == 0 {
		return nil
	}

	out := make([]float32, dim)
	for i := range acc {
		out[i] = float32(acc[i] / float64(valid))
	}
	return out
}

func sourceRefFromMetadata(raw json.RawMessage) string {
	if len(raw) == 0 || string(raw) == "null" {
		return ""
	}
	var m map[string]interface{}
	if err := json.Unmarshal(raw, &m); err != nil {
		return ""
	}
	ref, _ := m["source_ref"].(string)
	return strings.TrimSpace(ref)
}

func clamp(v, minV, maxV float64) float64 {
	if v < minV {
		return minV
	}
	if v > maxV {
		return maxV
	}
	return v
}
