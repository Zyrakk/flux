package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"net/http"
	nurl "net/url"
	"os"
	"os/signal"
	"regexp"
	"strings"
	"syscall"
	"time"

	"github.com/go-shiori/go-readability"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/mmcdole/gofeed"
	"github.com/redis/go-redis/v9"
	log "github.com/sirupsen/logrus"
	"github.com/zyrak/flux/internal/config"
	"github.com/zyrak/flux/internal/dedup"
	"github.com/zyrak/flux/internal/models"
	"github.com/zyrak/flux/internal/queue"
	"github.com/zyrak/flux/internal/ratelimit"
	"github.com/zyrak/flux/internal/store"
)

const (
	workerModeCronjob = "cronjob"
	workerModeDaemon  = "daemon"
	sourceTypeRSS     = "rss"
	runInterval       = 30 * time.Minute
	requestTimeout    = 30 * time.Second
)

var htmlTagPattern = regexp.MustCompile(`<[^>]*>`)

type rssSourceConfig struct {
	URL    string `json:"url"`
	Format string `json:"format,omitempty"`
}

type newArticleEvent struct {
	ArticleID string `json:"article_id"`
}

type rssWorker struct {
	store      *store.Store
	queue      *queue.Queue
	checker    *dedup.Checker
	httpClient *http.Client
}

type rssRunStats struct {
	FeedsProcessed int
	ItemsSeen      int
	NewArticles    int
	FeedErrors     int
}

type feedStats struct {
	ItemsSeen   int
	NewArticles int
}

func main() {
	cfg := config.Load()
	setupLogging(cfg.LogLevel)

	log.Info("Starting Flux RSS worker")

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

	redisOpts, err := redis.ParseURL(cfg.RedisURL)
	if err != nil {
		log.WithError(err).Fatal("Failed to parse REDIS_URL")
	}
	rdb := redis.NewClient(redisOpts)
	defer func() { _ = rdb.Close() }()

	if err := rdb.Ping(ctx).Err(); err != nil {
		log.WithError(err).Fatal("Failed to connect to Redis")
	}

	limiter, err := ratelimit.New(rdb, ratelimit.Config{
		Limits:    cfg.RateLimits,
		UserAgent: cfg.UserAgent,
	})
	if err != nil {
		log.WithError(err).Fatal("Failed to initialize rate limiter")
	}

	worker := &rssWorker{
		store:      db,
		queue:      q,
		checker:    dedup.NewChecker(rdb),
		httpClient: ratelimit.NewHTTPClient(limiter, requestTimeout),
	}

	mode := parseWorkerMode()
	for {
		runStart := time.Now()
		stats, err := worker.runOnce(ctx)
		if err != nil {
			log.WithError(err).Error("RSS worker run failed")
		}

		log.WithFields(log.Fields{
			"mode":            mode,
			"feeds_processed": stats.FeedsProcessed,
			"items_seen":      stats.ItemsSeen,
			"new_articles":    stats.NewArticles,
			"feed_errors":     stats.FeedErrors,
			"elapsed_ms":      time.Since(runStart).Milliseconds(),
		}).Info("RSS worker run completed")

		if mode != workerModeDaemon {
			break
		}

		log.WithField("sleep", runInterval.String()).Info("RSS daemon sleeping")
		select {
		case <-ctx.Done():
			log.Info("RSS worker shutting down")
			return
		case <-time.After(runInterval):
		}
	}

	log.Info("RSS worker finished")
}

func (w *rssWorker) runOnce(ctx context.Context) (rssRunStats, error) {
	stats := rssRunStats{}

	sources, err := w.store.ListSourcesByTypeWithSectionIDs(ctx, sourceTypeRSS, true)
	if err != nil {
		return stats, fmt.Errorf("listing enabled rss sources: %w", err)
	}

	for _, source := range sources {
		feedStats, err := w.processFeed(ctx, source)
		stats.FeedsProcessed++
		stats.ItemsSeen += feedStats.ItemsSeen
		stats.NewArticles += feedStats.NewArticles
		if err != nil {
			stats.FeedErrors++
			log.WithFields(log.Fields{
				"source_id": source.Source.ID,
				"source":    source.Source.Name,
				"error":     err.Error(),
			}).Error("Failed to process RSS feed")
			continue
		}
	}

	return stats, nil
}

func (w *rssWorker) processFeed(ctx context.Context, src *store.SourceWithSectionIDs) (feedStats, error) {
	stats := feedStats{}

	cfg, err := parseRSSSourceConfig(src.Source.Config)
	if err != nil {
		_ = w.store.UpdateSourceFetchStatus(ctx, src.Source.ID, err)
		return stats, err
	}
	feedURL := normalizeFeedURL(cfg.URL)
	if feedURL == "" {
		parseErr := errors.New("rss source config missing url")
		_ = w.store.UpdateSourceFetchStatus(ctx, src.Source.ID, parseErr)
		return stats, parseErr
	}

	parser := gofeed.NewParser()
	parser.Client = w.httpClient

	feed, err := parser.ParseURL(feedURL)
	if err != nil {
		_ = w.store.UpdateSourceFetchStatus(ctx, src.Source.ID, err)
		return stats, fmt.Errorf("parsing feed %s: %w", feedURL, err)
	}

	var sectionID *string
	if len(src.SectionIDs) == 1 {
		sectionID = &src.SectionIDs[0]
	}

	for _, item := range feed.Items {
		stats.ItemsSeen++

		rawURL := strings.TrimSpace(item.Link)
		if rawURL == "" {
			rawURL = strings.TrimSpace(item.GUID)
		}
		if rawURL == "" {
			continue
		}

		normalizedURL := dedup.NormalizeURL(rawURL)
		urlHash := dedup.HashURL(normalizedURL)

		isNew, err := w.checker.IsNew(ctx, normalizedURL)
		if err != nil {
			log.WithFields(log.Fields{
				"source_id": src.Source.ID,
				"source":    src.Source.Name,
				"url":       normalizedURL,
			}).WithError(err).Error("Dedup check failed")
			continue
		}
		if !isNew {
			continue
		}

		content, contentErr := w.fetchArticleContent(ctx, normalizedURL)
		if contentErr != nil {
			log.WithFields(log.Fields{
				"source_id": src.Source.ID,
				"source":    src.Source.Name,
				"url":       normalizedURL,
			}).WithError(contentErr).Warn("Failed to fetch readable content, using feed fallback")

			content = cleanText(strings.TrimSpace(item.Content))
			if content == "" {
				content = cleanText(strings.TrimSpace(item.Description))
			}
		}

		var contentPtr *string
		if content != "" {
			contentPtr = &content
		}

		title := strings.TrimSpace(item.Title)
		if title == "" {
			title = normalizedURL
		}

		metadataMap := map[string]interface{}{
			"source_name":    src.Source.Name,
			"source_ref":     src.Source.ID,
			"feed_url":       feedURL,
			"normalized_url": normalizedURL,
			"url_hash":       urlHash,
		}
		if guid := strings.TrimSpace(item.GUID); guid != "" {
			metadataMap["guid"] = guid
		}

		metadata, err := json.Marshal(metadataMap)
		if err != nil {
			log.WithError(err).Warn("Failed to marshal RSS metadata")
			metadata = []byte("{}")
		}

		article := &models.Article{
			SourceType:  sourceTypeRSS,
			SourceID:    urlHash,
			SectionID:   sectionID,
			URL:         normalizedURL,
			Title:       title,
			Content:     contentPtr,
			Author:      extractAuthor(item),
			PublishedAt: extractPublishedAt(item),
			Status:      models.StatusPending,
			Metadata:    metadata,
		}

		if err := w.store.CreateArticle(ctx, article); err != nil {
			if isUniqueViolation(err) {
				continue
			}
			log.WithFields(log.Fields{
				"source_id": src.Source.ID,
				"source":    src.Source.Name,
				"url":       normalizedURL,
			}).WithError(err).Error("Failed to insert RSS article")
			continue
		}

		if err := w.queue.Publish(queue.SubjectArticlesNew, newArticleEvent{ArticleID: article.ID}); err != nil {
			log.WithField("article_id", article.ID).WithError(err).Error("Failed to publish articles.new")
			continue
		}

		stats.NewArticles++
	}

	if err := w.store.UpdateSourceFetchStatus(ctx, src.Source.ID, nil); err != nil {
		log.WithFields(log.Fields{
			"source_id": src.Source.ID,
			"source":    src.Source.Name,
		}).WithError(err).Warn("Failed to update source fetch status")
	}

	log.WithFields(log.Fields{
		"source_id":     src.Source.ID,
		"source":        src.Source.Name,
		"feed_url":      feedURL,
		"items_seen":    stats.ItemsSeen,
		"new_articles":  stats.NewArticles,
		"section_links": len(src.SectionIDs),
	}).Info("RSS feed processed")

	return stats, nil
}

func (w *rssWorker) fetchArticleContent(ctx context.Context, url string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}

	resp, err := w.httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("unexpected status code %d", resp.StatusCode)
	}

	parsedURL, err := nurl.Parse(url)
	if err != nil {
		return "", err
	}

	article, err := readability.FromReader(resp.Body, parsedURL)
	if err != nil {
		return "", err
	}

	return cleanText(article.TextContent), nil
}

func cleanText(raw string) string {
	raw = htmlTagPattern.ReplaceAllString(raw, " ")
	raw = html.UnescapeString(raw)
	return strings.TrimSpace(strings.Join(strings.Fields(raw), " "))
}

func parseRSSSourceConfig(raw json.RawMessage) (*rssSourceConfig, error) {
	cfg := &rssSourceConfig{}
	if err := json.Unmarshal(raw, cfg); err != nil {
		return nil, fmt.Errorf("parsing source config: %w", err)
	}
	return cfg, nil
}

func normalizeFeedURL(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	if strings.HasPrefix(raw, "http://") || strings.HasPrefix(raw, "https://") {
		return raw
	}
	return "https://" + raw
}

func extractAuthor(item *gofeed.Item) *string {
	if item.Author != nil {
		name := strings.TrimSpace(item.Author.Name)
		if name != "" {
			return &name
		}
	}
	if len(item.Authors) > 0 {
		name := strings.TrimSpace(item.Authors[0].Name)
		if name != "" {
			return &name
		}
	}
	return nil
}

func extractPublishedAt(item *gofeed.Item) *time.Time {
	if item.PublishedParsed != nil {
		return item.PublishedParsed
	}
	if item.UpdatedParsed != nil {
		return item.UpdatedParsed
	}
	return nil
}

func parseWorkerMode() string {
	mode := strings.ToLower(strings.TrimSpace(os.Getenv("WORKER_MODE")))
	if mode == "" {
		mode = strings.ToLower(strings.TrimSpace(os.Getenv("MODE")))
	}
	if mode == "" {
		return workerModeCronjob
	}
	if mode != workerModeCronjob && mode != workerModeDaemon {
		log.WithField("worker_mode", mode).Warn("Unknown WORKER_MODE, falling back to cronjob")
		return workerModeCronjob
	}
	return mode
}

func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}

func setupLogging(level string) {
	log.SetFormatter(&log.JSONFormatter{})
	lvl, err := log.ParseLevel(level)
	if err != nil {
		lvl = log.InfoLevel
	}
	log.SetLevel(lvl)
}
