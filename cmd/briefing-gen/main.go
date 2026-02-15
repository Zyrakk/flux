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
	Total      int
	Filtered   int
}

type sectionMeta struct {
	Total    int `json:"total"`
	Filtered int `json:"filtered"`
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
		candidates, total, err := db.ListPendingArticlesForSection(ctx, sec.ID, threshold, sec.MaxBriefingArticles)
		if err != nil {
			return fmt.Errorf("listing pending section articles (%s): %w", sec.Name, err)
		}
		sectionRuns[sec.ID] = &sectionRun{
			Section:    sec,
			Threshold:  threshold,
			Candidates: candidates,
			Total:      total,
		}
		log.WithFields(log.Fields{
			"section":        sec.Name,
			"threshold":      threshold,
			"pending_total":  total,
			"selected_count": len(candidates),
		}).Info("Collected candidate articles for section")
		totalCandidates += len(candidates)
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
			})
			summarizedCount++
			briefedIDs[article.ID] = struct{}{}
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
			sb.WriteString("  " + article.URL + "\n\n")
		}
	}
	return strings.TrimSpace(sb.String())
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
