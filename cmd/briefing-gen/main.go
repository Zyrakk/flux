package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/signal"
	"sort"
	"strings"
	"syscall"
	"time"

	"github.com/robfig/cron/v3"
	log "github.com/sirupsen/logrus"
	"github.com/zyrak/flux/internal/config"
	"github.com/zyrak/flux/internal/llm"
	"github.com/zyrak/flux/internal/models"
	"github.com/zyrak/flux/internal/store"
)

const (
	briefingModeCronjob = "cronjob"
	briefingModeDaemon  = "daemon"
	llmTimeout          = 120 * time.Second
)

type sectionRun struct {
	Section    *models.Section
	Threshold  float64
	Candidates []*models.Article
	ClusterMap map[string]clusterInfo
	Total      int
	Filtered   int
}

type sectionMeta struct {
	Total    int `json:"total"`
	Filtered int `json:"filtered"`
}

type clusterInfo struct {
	SeenIn       []string
	ReportedBy   []string
	SuppressedID []string
	Bonus        float64
}

func main() {
	cfg := config.Load()
	setupLogging(cfg.LogLevel)

	log.Info("Starting Flux briefing generator")

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	db, err := store.New(ctx, cfg.DatabaseURL)
	if err != nil {
		log.WithError(err).Fatal("Failed to connect to PostgreSQL")
	}
	defer db.Close()

	analyzer, err := llm.NewAnalyzer(cfg.LLMProvider, cfg.LLMEndpoint, cfg.LLMModel, cfg.LLMAPIKey)
	if err != nil {
		log.WithError(err).Fatal("Failed to initialize LLM analyzer")
	}
	log.WithField("provider", analyzer.Provider()).Info("LLM analyzer ready")

	mode := parseBriefingMode()
	if mode == briefingModeDaemon {
		runDaemon(ctx, cfg, db, analyzer)
		return
	}

	if err := runOnce(ctx, cfg, db, analyzer); err != nil {
		log.WithError(err).Fatal("Briefing generation failed")
	}

	log.Info("Briefing generator finished")
}

func runDaemon(ctx context.Context, cfg *config.Config, db *store.Store, analyzer llm.Analyzer) {
	schedule, err := cron.ParseStandard(cfg.BriefingSchedule)
	if err != nil {
		log.WithError(err).WithField("schedule", cfg.BriefingSchedule).Fatal("Invalid BRIEFING_SCHEDULE")
	}

	log.WithField("schedule", cfg.BriefingSchedule).Info("Briefing daemon scheduler active")
	for {
		next := schedule.Next(time.Now().UTC())
		wait := time.Until(next)
		log.WithFields(log.Fields{
			"next_run_utc": next.Format(time.RFC3339),
			"wait":         wait.String(),
		}).Info("Waiting for next briefing run")

		timer := time.NewTimer(wait)
		select {
		case <-ctx.Done():
			timer.Stop()
			log.Info("Briefing daemon shutting down")
			return
		case <-timer.C:
		}

		runCtx, cancel := context.WithTimeout(ctx, 30*time.Minute)
		err := runOnce(runCtx, cfg, db, analyzer)
		cancel()
		if err != nil {
			log.WithError(err).Error("Scheduled briefing run failed")
		}
	}
}

func runOnce(ctx context.Context, cfg *config.Config, db *store.Store, analyzer llm.Analyzer) error {
	start := time.Now()

	sections, err := db.ListSections(ctx)
	if err != nil {
		return fmt.Errorf("listing sections: %w", err)
	}

	enabledSections := make([]*models.Section, 0, len(sections))
	sectionsByName := make(map[string]*models.Section)
	for _, sec := range sections {
		if !sec.Enabled {
			continue
		}
		enabledSections = append(enabledSections, sec)
		sectionsByName[sec.Name] = sec
	}
	if len(enabledSections) == 0 {
		log.Info("No enabled sections, skipping briefing generation")
		return nil
	}

	sectionRuns := make(map[string]*sectionRun, len(enabledSections))
	totalCandidates := 0
	for _, sec := range enabledSections {
		threshold := thresholdFromSection(sec, cfg)
		fetchLimit := sec.MaxBriefingArticles * 6
		if fetchLimit < sec.MaxBriefingArticles {
			fetchLimit = sec.MaxBriefingArticles
		}
		if fetchLimit < 20 {
			fetchLimit = 20
		}

		candidates, total, err := db.ListPendingArticlesForSection(ctx, sec.ID, threshold, fetchLimit)
		if err != nil {
			return fmt.Errorf("listing pending section articles (%s): %w", sec.Name, err)
		}

		clusteredCandidates, clusterMap := collapseClusteredCandidates(candidates, sec.MaxBriefingArticles)
		sectionRuns[sec.ID] = &sectionRun{
			Section:    sec,
			Threshold:  threshold,
			Candidates: clusteredCandidates,
			ClusterMap: clusterMap,
			Total:      total,
		}
		log.WithFields(log.Fields{
			"section":        sec.Name,
			"threshold":      threshold,
			"pending_total":  total,
			"fetched_count":  len(candidates),
			"selected_count": len(clusteredCandidates),
		}).Info("Collected candidate articles for section")
		totalCandidates += len(clusteredCandidates)
	}

	if totalCandidates == 0 {
		log.Info("No pending relevant articles found for briefing generation")
		return nil
	}

	briefedIDs := make(map[string]struct{})
	processedIDs := make(map[string]struct{})
	summarizedBySection := make(map[string][]llm.SummarizedArticle)
	partial := false
	pendingCount := 0
	tokensClassify := 0
	tokensSummarize := 0
	tokensBriefing := 0

	for _, sec := range enabledSections {
		run := sectionRuns[sec.ID]
		if len(run.Candidates) == 0 {
			continue
		}

		classifyInputs := make([]llm.ArticleInput, 0, len(run.Candidates))
		for _, article := range run.Candidates {
			classifyInputs = append(classifyInputs, toClassifyInput(article, run.Section))
		}
		tokensClassify += estimateTokens(llm.BuildClassifyPrompt(classifyInputs))

		classifications, err := classifyWithTimeout(ctx, analyzer, classifyInputs)
		if err != nil {
			partial = true
			pendingCount += len(run.Candidates)
			log.WithFields(log.Fields{
				"section": run.Section.Name,
				"count":   len(run.Candidates),
			}).WithError(err).Warn("LLM classification failed, leaving section articles pending")
			continue
		}
		log.WithFields(log.Fields{
			"section": sec.Name,
			"count":   len(classifications),
		}).Info("LLM classification completed for section")

		classByID := indexClassifications(classifyInputs, classifications)
		summarizedCount := 0
		for _, article := range run.Candidates {
			cluster := run.ClusterMap[article.ID]

			classification, ok := classByID[article.ID]
			if !ok {
				partial = true
				pendingCount++
				log.WithFields(log.Fields{
					"article_id": article.ID,
					"section":    run.Section.Name,
				}).Warn("Missing classification for article, leaving pending")
				continue
			}

			if !classification.Relevant || classification.Clickbait {
				run.Filtered++
				processedIDs[article.ID] = struct{}{}
				for _, suppressedID := range cluster.SuppressedID {
					processedIDs[suppressedID] = struct{}{}
				}
				continue
			}

			targetSection := resolveClassificationSection(classification.Section, run.Section, sectionsByName)
			if targetSection.ID != run.Section.ID && article.RelevanceScore != nil {
				if err := db.UpdateArticleSection(ctx, article.ID, targetSection.ID, *article.RelevanceScore); err != nil {
					log.WithFields(log.Fields{
						"article_id":   article.ID,
						"from_section": run.Section.Name,
						"to_section":   targetSection.Name,
					}).WithError(err).Warn("Failed to persist section correction from classifier")
				} else {
					article.SectionID = &targetSection.ID
				}
			}

			// Keep per-section cap even if classifier reassigns section.
			if len(summarizedBySection[targetSection.Name]) >= targetSection.MaxBriefingArticles {
				run.Filtered++
				processedIDs[article.ID] = struct{}{}
				for _, suppressedID := range cluster.SuppressedID {
					processedIDs[suppressedID] = struct{}{}
				}
				continue
			}

			summarizeInput := toSummarizeInput(article, targetSection)
			tokensSummarize += estimateTokens(llm.BuildSummarizePrompt(summarizeInput))

			summary, err := summarizeWithTimeout(ctx, analyzer, summarizeInput)
			if err != nil {
				partial = true
				pendingCount++
				log.WithFields(log.Fields{
					"article_id": article.ID,
					"section":    targetSection.Name,
				}).WithError(err).Warn("LLM summarization failed, leaving article pending")
				continue
			}
			tokensSummarize += estimateTokens(summary)

			if err := db.UpdateArticleSummary(ctx, article.ID, summary, nil); err != nil {
				log.WithField("article_id", article.ID).WithError(err).Warn("Failed to persist article summary")
			}

			summarizedBySection[targetSection.Name] = append(summarizedBySection[targetSection.Name], llm.SummarizedArticle{
				ID:         article.ID,
				Title:      article.Title,
				Summary:    summary,
				URL:        article.URL,
				SourceType: article.SourceType,
				SeenIn:     cluster.SeenIn,
				ReportedBy: cluster.ReportedBy,
			})
			summarizedCount++
			briefedIDs[article.ID] = struct{}{}
			for _, suppressedID := range cluster.SuppressedID {
				processedIDs[suppressedID] = struct{}{}
			}
		}
		log.WithFields(log.Fields{
			"section":          sec.Name,
			"summaries_stored": summarizedCount,
		}).Info("LLM summaries generated for section")
	}

	briefingSections := buildBriefingSections(enabledSections, summarizedBySection)
	var content string
	if len(briefingSections) > 0 {
		tokensBriefing += estimateTokens(llm.BuildBriefingPrompt(briefingSections))
		content, err = generateBriefingWithTimeout(ctx, analyzer, briefingSections)
		if err != nil {
			partial = true
			log.WithError(err).Warn("LLM briefing synthesis failed, generating local partial briefing")
			content = buildFallbackBriefing(briefingSections)
		} else {
			tokensBriefing += estimateTokens(content)
			log.WithField("sections_included", len(briefingSections)).Info("LLM briefing synthesized")
		}
		content = appendMultiSourceCoverage(content, briefingSections)
	} else {
		partial = true
		content = buildFallbackBriefing(nil)
	}

	tokensEstimated := tokensClassify + tokensSummarize + tokensBriefing

	briefingArticleIDs := sortedIDs(briefedIDs)
	for _, id := range briefingArticleIDs {
		delete(processedIDs, id)
	}
	processedArticleIDs := sortedIDs(processedIDs)

	for _, id := range briefingArticleIDs {
		if err := db.UpdateArticleStatus(ctx, id, models.StatusBriefed); err != nil {
			log.WithField("article_id", id).WithError(err).Warn("Failed to update article status to briefed")
		}
	}
	for _, id := range processedArticleIDs {
		if err := db.UpdateArticleStatus(ctx, id, models.StatusProcessed); err != nil {
			log.WithField("article_id", id).WithError(err).Warn("Failed to update article status to processed")
		}
	}

	sectionsMetadata := make(map[string]sectionMeta, len(enabledSections))
	for _, sec := range enabledSections {
		run := sectionRuns[sec.ID]
		if run == nil {
			continue
		}
		sectionsMetadata[sec.Name] = sectionMeta{
			Total:    run.Total,
			Filtered: run.Filtered,
		}
	}

	metadataMap := map[string]interface{}{
		"sections":         sectionsMetadata,
		"tokens_estimated": tokensEstimated,
		"token_breakdown": map[string]int{
			"classify":  tokensClassify,
			"summarize": tokensSummarize,
			"briefing":  tokensBriefing,
		},
	}
	if partial {
		metadataMap["partial"] = true
		metadataMap["pending_count"] = pendingCount
	}
	metadata, err := json.Marshal(metadataMap)
	if err != nil {
		return fmt.Errorf("marshalling briefing metadata: %w", err)
	}

	briefing := &models.Briefing{
		Content:    content,
		ArticleIDs: briefingArticleIDs,
		Metadata:   metadata,
	}
	if err := db.CreateBriefing(ctx, briefing); err != nil {
		return fmt.Errorf("creating briefing: %w", err)
	}

	log.WithFields(log.Fields{
		"briefing_id":        briefing.ID,
		"included_articles":  len(briefingArticleIDs),
		"processed_articles": len(processedArticleIDs),
		"partial":            partial,
		"pending_count":      pendingCount,
		"tokens_estimated":   tokensEstimated,
		"tokens_classify":    tokensClassify,
		"tokens_summarize":   tokensSummarize,
		"tokens_briefing":    tokensBriefing,
		"duration_ms":        time.Since(start).Milliseconds(),
	}).Info("Briefing generated")

	return nil
}

func parseBriefingMode() string {
	mode := strings.ToLower(strings.TrimSpace(os.Getenv("BRIEFING_MODE")))
	if mode == "" {
		return briefingModeCronjob
	}
	if mode != briefingModeDaemon {
		return briefingModeCronjob
	}
	return mode
}

func thresholdFromSection(section *models.Section, cfg *config.Config) float64 {
	threshold := cfg.RelevanceThresholdDefault
	if len(section.Config) > 0 && string(section.Config) != "null" {
		var m map[string]interface{}
		if err := json.Unmarshal(section.Config, &m); err == nil {
			if val, ok := m["relevance_threshold"].(float64); ok {
				threshold = val
			} else if val, ok := m["threshold"].(float64); ok {
				threshold = val
			}
		}
	}

	if threshold < cfg.RelevanceThresholdMin {
		threshold = cfg.RelevanceThresholdMin
	}
	if threshold > cfg.RelevanceThresholdMax {
		threshold = cfg.RelevanceThresholdMax
	}
	return threshold
}

func toClassifyInput(article *models.Article, sec *models.Section) llm.ArticleInput {
	return llm.ArticleInput{
		ID:         article.ID,
		Title:      article.Title,
		Content:    firstParagraph(article.Content, 200),
		Section:    sec.Name,
		SourceType: article.SourceType,
		URL:        article.URL,
	}
}

func toSummarizeInput(article *models.Article, sec *models.Section) llm.ArticleInput {
	content := ""
	if article.Content != nil {
		content = *article.Content
	}
	return llm.ArticleInput{
		ID:         article.ID,
		Title:      article.Title,
		Content:    content,
		Section:    sec.Name,
		SourceType: article.SourceType,
		URL:        article.URL,
	}
}

func classifyWithTimeout(ctx context.Context, analyzer llm.Analyzer, inputs []llm.ArticleInput) ([]llm.Classification, error) {
	callCtx, cancel := context.WithTimeout(ctx, llmTimeout)
	defer cancel()
	return analyzer.Classify(callCtx, inputs)
}

func summarizeWithTimeout(ctx context.Context, analyzer llm.Analyzer, input llm.ArticleInput) (string, error) {
	callCtx, cancel := context.WithTimeout(ctx, llmTimeout)
	defer cancel()
	return analyzer.Summarize(callCtx, input)
}

func generateBriefingWithTimeout(ctx context.Context, analyzer llm.Analyzer, sections []llm.BriefingSection) (string, error) {
	callCtx, cancel := context.WithTimeout(ctx, llmTimeout)
	defer cancel()
	return analyzer.GenerateBriefing(callCtx, sections)
}

func indexClassifications(inputs []llm.ArticleInput, classifications []llm.Classification) map[string]llm.Classification {
	out := make(map[string]llm.Classification, len(classifications))
	for i, cls := range classifications {
		id := strings.TrimSpace(cls.ArticleID)
		if id == "" && i < len(inputs) {
			id = inputs[i].ID
			cls.ArticleID = id
		}
		if id == "" {
			continue
		}
		out[id] = cls
	}
	return out
}

func resolveClassificationSection(sectionName string, fallback *models.Section, sectionsByName map[string]*models.Section) *models.Section {
	name := strings.ToLower(strings.TrimSpace(sectionName))
	if name == "" {
		return fallback
	}
	if sec, ok := sectionsByName[name]; ok {
		return sec
	}
	return fallback
}

func buildBriefingSections(enabledSections []*models.Section, summarizedBySection map[string][]llm.SummarizedArticle) []llm.BriefingSection {
	out := make([]llm.BriefingSection, 0, len(enabledSections))
	for _, sec := range enabledSections {
		articles := summarizedBySection[sec.Name]
		if len(articles) == 0 {
			continue
		}
		out = append(out, llm.BriefingSection{
			Name:        sec.Name,
			DisplayName: sec.DisplayName,
			MaxArticles: sec.MaxBriefingArticles,
			Articles:    articles,
		})
	}
	return out
}

func collapseClusteredCandidates(candidates []*models.Article, maxArticles int) ([]*models.Article, map[string]clusterInfo) {
	if len(candidates) == 0 {
		return []*models.Article{}, map[string]clusterInfo{}
	}
	if maxArticles <= 0 {
		maxArticles = len(candidates)
	}

	type clusterEntry struct {
		primary *models.Article
		info    clusterInfo
		score   float64
		base    float64
	}

	buckets := make(map[string][]*models.Article)
	order := make([]string, 0, len(candidates))

	for _, article := range candidates {
		clusterID := clusterIDForArticle(article)
		if _, exists := buckets[clusterID]; !exists {
			order = append(order, clusterID)
		}
		buckets[clusterID] = append(buckets[clusterID], article)
	}

	entries := make([]clusterEntry, 0, len(buckets))
	for _, clusterID := range order {
		members := buckets[clusterID]
		if len(members) == 0 {
			continue
		}

		primary := pickClusterPrimary(members)
		seenIn, reportedBy := collectClusterCoverage(members)
		suppressed := make([]string, 0, len(members)-1)
		for _, member := range members {
			if member.ID == primary.ID {
				continue
			}
			suppressed = append(suppressed, member.ID)
		}
		sort.Strings(suppressed)

		sourceCount := len(seenIn)
		bonus := 0.0
		if sourceCount > 1 {
			bonus = float64(sourceCount-1) * 0.1
		}

		base := relevanceScore(primary)
		entries = append(entries, clusterEntry{
			primary: primary,
			info: clusterInfo{
				SeenIn:       seenIn,
				ReportedBy:   reportedBy,
				SuppressedID: suppressed,
				Bonus:        bonus,
			},
			score: base + bonus,
			base:  base,
		})
	}

	sort.Slice(entries, func(i, j int) bool {
		if entries[i].score != entries[j].score {
			return entries[i].score > entries[j].score
		}
		if entries[i].base != entries[j].base {
			return entries[i].base > entries[j].base
		}
		if !entries[i].primary.IngestedAt.Equal(entries[j].primary.IngestedAt) {
			return entries[i].primary.IngestedAt.After(entries[j].primary.IngestedAt)
		}
		return entries[i].primary.ID < entries[j].primary.ID
	})

	limit := maxArticles
	if limit > len(entries) {
		limit = len(entries)
	}

	selected := make([]*models.Article, 0, limit)
	infoByArticle := make(map[string]clusterInfo, limit)
	for i := 0; i < limit; i++ {
		selected = append(selected, entries[i].primary)
		infoByArticle[entries[i].primary.ID] = entries[i].info
	}

	return selected, infoByArticle
}

func clusterIDForArticle(article *models.Article) string {
	meta := parseArticleMetadata(article.Metadata)
	clusterID := metadataString(meta, "cluster_id")
	if clusterID != "" {
		return clusterID
	}
	return article.ID
}

func pickClusterPrimary(members []*models.Article) *models.Article {
	if len(members) == 0 {
		return nil
	}

	for _, member := range members {
		primaryID := metadataString(parseArticleMetadata(member.Metadata), "cluster_primary_id")
		if primaryID == "" {
			continue
		}
		for _, candidate := range members {
			if candidate.ID == primaryID {
				return candidate
			}
		}
	}

	best := members[0]
	bestSignal := articleSignal(best)
	for i := 1; i < len(members); i++ {
		candidate := members[i]
		candidateSignal := articleSignal(candidate)
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

func collectClusterCoverage(members []*models.Article) ([]string, []string) {
	type coverage struct {
		plain    string
		detailed string
		signal   float64
		order    int
	}

	seen := make(map[string]coverage)
	for i, member := range members {
		plain, detailed, signal := sourceCoverage(member)
		if plain == "" {
			continue
		}

		existing, ok := seen[plain]
		if !ok {
			seen[plain] = coverage{
				plain:    plain,
				detailed: detailed,
				signal:   signal,
				order:    i,
			}
			continue
		}

		if signal > existing.signal {
			existing.detailed = detailed
			existing.signal = signal
		}
		seen[plain] = existing
	}

	items := make([]coverage, 0, len(seen))
	for _, item := range seen {
		items = append(items, item)
	}

	sort.Slice(items, func(i, j int) bool {
		if items[i].signal != items[j].signal {
			return items[i].signal > items[j].signal
		}
		if items[i].order != items[j].order {
			return items[i].order < items[j].order
		}
		return items[i].plain < items[j].plain
	})

	seenIn := make([]string, 0, len(items))
	reportedBy := make([]string, 0, len(items))
	for _, item := range items {
		seenIn = append(seenIn, item.plain)
		reportedBy = append(reportedBy, item.detailed)
	}
	return seenIn, reportedBy
}

func sourceCoverage(article *models.Article) (plain string, detailed string, signal float64) {
	meta := parseArticleMetadata(article.Metadata)
	sourceType := strings.ToLower(strings.TrimSpace(article.SourceType))

	switch sourceType {
	case "hn":
		score := metadataFloat(meta, "hn_score")
		if score > 0 {
			return "HN", fmt.Sprintf("HN (%d pts)", int(score)), score
		}
		return "HN", "HN", 0
	case "reddit":
		sub := metadataString(meta, "subreddit")
		sub = strings.TrimPrefix(strings.ToLower(strings.TrimSpace(sub)), "r/")
		if sub == "" {
			sub = "reddit"
		}
		score := metadataFloat(meta, "reddit_score")
		plain = "r/" + sub
		if score > 0 {
			return plain, fmt.Sprintf("Reddit %s (%d pts)", plain, int(score)), score
		}
		return plain, "Reddit " + plain, 0
	default:
		name := metadataString(meta, "source_name")
		if name == "" {
			if sourceType == "github" {
				name = metadataString(meta, "repo")
			}
		}
		if name == "" {
			name = article.SourceType
		}
		return name, name, 0
	}
}

func articleSignal(article *models.Article) float64 {
	meta := parseArticleMetadata(article.Metadata)
	hn := metadataFloat(meta, "hn_score")
	reddit := metadataFloat(meta, "reddit_score")
	if hn > reddit {
		return hn
	}
	return reddit
}

func relevanceScore(article *models.Article) float64 {
	if article == nil || article.RelevanceScore == nil {
		return 0
	}
	return *article.RelevanceScore
}

func parseArticleMetadata(raw json.RawMessage) map[string]interface{} {
	if len(raw) == 0 || string(raw) == "null" {
		return map[string]interface{}{}
	}

	out := map[string]interface{}{}
	if err := json.Unmarshal(raw, &out); err != nil {
		return map[string]interface{}{}
	}
	return out
}

func metadataString(meta map[string]interface{}, key string) string {
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

func metadataFloat(meta map[string]interface{}, key string) float64 {
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

func buildFallbackBriefing(sections []llm.BriefingSection) string {
	if len(sections) == 0 {
		return "# Briefing parcial\n\nNo hubo artículos listos para sintetizar en este ciclo."
	}

	var sb strings.Builder
	sb.WriteString("# Briefing parcial\n\n")
	for _, sec := range sections {
		sb.WriteString("## " + sec.DisplayName + "\n\n")
		for _, article := range sec.Articles {
			sb.WriteString("- **" + article.Title + "**\n")
			sb.WriteString("  " + article.Summary + "\n")
			if len(article.ReportedBy) > 1 {
				sb.WriteString("  Reportado por: " + strings.Join(article.ReportedBy, ", ") + "\n")
			}
			if len(article.SeenIn) > 1 {
				sb.WriteString("  📡 Visto en: " + strings.Join(article.SeenIn, ", ") + "\n")
			}
			sb.WriteString("  " + article.URL + "\n\n")
		}
	}
	return strings.TrimSpace(sb.String())
}

func appendMultiSourceCoverage(content string, sections []llm.BriefingSection) string {
	lines := make([]string, 0)
	seen := make(map[string]struct{})

	for _, section := range sections {
		for _, article := range section.Articles {
			if len(article.SeenIn) <= 1 {
				continue
			}

			key := strings.TrimSpace(article.ID)
			if key == "" {
				key = strings.TrimSpace(article.Title)
			}
			if key == "" {
				continue
			}
			if _, exists := seen[key]; exists {
				continue
			}
			seen[key] = struct{}{}

			title := strings.TrimSpace(article.Title)
			if title == "" {
				title = "Historia sin titulo"
			}
			lines = append(lines, fmt.Sprintf("- %s\n  📡 Visto en: %s", title, strings.Join(article.SeenIn, ", ")))
		}
	}

	if len(lines) == 0 {
		return content
	}

	base := strings.TrimSpace(content)
	if base == "" {
		base = "# Briefing parcial"
	}
	return base + "\n\n### 📡 Cobertura Multi-fuente\n" + strings.Join(lines, "\n")
}

func firstParagraph(content *string, maxChars int) string {
	if content == nil {
		return ""
	}
	trimmed := strings.TrimSpace(*content)
	if trimmed == "" {
		return ""
	}

	for _, sep := range []string{"\n\n", "\n"} {
		if idx := strings.Index(trimmed, sep); idx > 0 {
			trimmed = trimmed[:idx]
			break
		}
	}

	if len(trimmed) > maxChars {
		trimmed = trimmed[:maxChars]
	}
	return strings.TrimSpace(trimmed)
}

func estimateTokens(text string) int {
	if text == "" {
		return 0
	}
	// Heurística común: ~4 caracteres por token.
	return (len(text) + 3) / 4
}

func sortedIDs(m map[string]struct{}) []string {
	out := make([]string, 0, len(m))
	for id := range m {
		out = append(out, id)
	}
	sort.Strings(out)
	return out
}

func setupLogging(level string) {
	log.SetFormatter(&log.JSONFormatter{})
	lvl, err := log.ParseLevel(level)
	if err != nil {
		lvl = log.InfoLevel
	}
	log.SetLevel(lvl)
}
