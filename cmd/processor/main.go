package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os/signal"
	"strings"
	"syscall"
	"time"

	log "github.com/sirupsen/logrus"
	"github.com/zyrak/flux/internal/config"
	"github.com/zyrak/flux/internal/embeddings"
	"github.com/zyrak/flux/internal/models"
	"github.com/zyrak/flux/internal/queue"
	"github.com/zyrak/flux/internal/relevance"
	"github.com/zyrak/flux/internal/store"
)

type newArticleEvent struct {
	ArticleID string `json:"article_id"`
}

type processor struct {
	store     *store.Store
	embed     *embeddings.Client
	relevance *relevance.Engine
}

func main() {
	cfg := config.Load()
	setupLogging(cfg.LogLevel)

	log.Info("Starting Flux processor")

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	db, err := store.New(ctx, cfg.DatabaseURL)
	if err != nil {
		log.WithError(err).Fatal("Failed to connect to PostgreSQL")
	}
	defer db.Close()

	q, err := queue.New(cfg.NatsURL)
	if err != nil {
		log.WithError(err).Fatal("Failed to connect to NATS")
	}
	defer q.Close()

	embedClient := embeddings.NewClient(cfg.EmbeddingsURL)
	relEngine, err := waitForRelevanceEngine(ctx, db, embedClient, relevance.Config{
		DefaultThreshold: cfg.RelevanceThresholdDefault,
		MinThreshold:     cfg.RelevanceThresholdMin,
		MaxThreshold:     cfg.RelevanceThresholdMax,
		ThresholdStep:    cfg.RelevanceThresholdStep,
		SourceBoosts:     cfg.SourceBoosts,
	})
	if err != nil {
		log.WithError(err).Fatal("Failed to initialize relevance engine")
	}

	proc := &processor{
		store:     db,
		embed:     embedClient,
		relevance: relEngine,
	}

	if err := q.Subscribe(ctx, queue.SubjectArticlesNew, "flux-processor", proc.handleNewArticle); err != nil {
		log.WithError(err).Fatal("Failed to subscribe to articles.new")
	}

	log.WithFields(log.Fields{
		"subject":        queue.SubjectArticlesNew,
		"embeddings_url": cfg.EmbeddingsURL,
	}).Info("Processor subscribed and ready")

	<-ctx.Done()

	log.Info("Processor shutting down")
}

func waitForRelevanceEngine(ctx context.Context, db *store.Store, embedClient *embeddings.Client, cfg relevance.Config) (*relevance.Engine, error) {
	backoff := 2 * time.Second
	for {
		engine, err := relevance.NewEngine(ctx, db, embedClient, cfg)
		if err == nil {
			return engine, nil
		}

		log.WithError(err).Warn("Relevance engine initialization failed, retrying")
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(backoff):
		}

		backoff *= 2
		if backoff > 20*time.Second {
			backoff = 20 * time.Second
		}
	}
}

func (p *processor) handleNewArticle(data []byte) error {
	var evt newArticleEvent
	if err := json.Unmarshal(data, &evt); err != nil {
		return fmt.Errorf("invalid articles.new payload: %w", err)
	}
	if evt.ArticleID == "" {
		return fmt.Errorf("articles.new payload missing article_id")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	article, err := p.store.GetArticleByID(ctx, evt.ArticleID)
	if err != nil {
		return fmt.Errorf("loading article %s: %w", evt.ArticleID, err)
	}
	if article == nil {
		log.WithField("article_id", evt.ArticleID).Warn("Article not found, skipping")
		return nil
	}

	text := buildEmbeddingText(article)
	articleEmbedding, err := p.embed.EmbedSingle(ctx, text)
	if err != nil {
		return fmt.Errorf("embedding article %s: %w", article.ID, err)
	}
	if err := p.store.UpdateArticleEmbedding(ctx, article.ID, articleEmbedding); err != nil {
		return fmt.Errorf("updating embedding for article %s: %w", article.ID, err)
	}

	result, err := p.relevance.EvaluateArticle(ctx, article, articleEmbedding)
	if err != nil {
		return fmt.Errorf("evaluating relevance for article %s: %w", article.ID, err)
	}

	if err := p.store.UpdateArticleSectionAndStatus(ctx, article.ID, result.SectionID, result.RelevanceScore, result.Status); err != nil {
		return fmt.Errorf("updating section/score/status for article %s: %w", article.ID, err)
	}

	newThreshold, changed, err := p.relevance.AdjustThreshold(ctx, result.SectionID)
	if err != nil {
		log.WithField("section_id", result.SectionID).WithError(err).Warn("Failed to adjust section threshold")
	}

	logFields := log.Fields{
		"article_id":      article.ID,
		"section_id":      result.SectionID,
		"section":         result.SectionName,
		"relevance_score": result.RelevanceScore,
		"status":          result.Status,
		"threshold":       result.Threshold,
		"source_type":     article.SourceType,
	}
	if result.SourceID != "" {
		logFields["source_id"] = result.SourceID
	}
	if changed {
		logFields["new_threshold"] = newThreshold
	}
	log.WithFields(logFields).Info("Article processed")

	return nil
}

func buildEmbeddingText(article *models.Article) string {
	content := ""
	if article.Content != nil {
		content = *article.Content
	}
	content = strings.TrimSpace(content)
	if len(content) > 500 {
		content = content[:500]
	}

	title := strings.TrimSpace(article.Title)
	if content == "" {
		return title
	}
	return title + "\n\n" + content
}

func setupLogging(level string) {
	log.SetFormatter(&log.JSONFormatter{})
	lvl, err := log.ParseLevel(level)
	if err != nil {
		lvl = log.InfoLevel
	}
	log.SetLevel(lvl)
}
