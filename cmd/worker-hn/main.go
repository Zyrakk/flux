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
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/go-shiori/go-readability"
	"github.com/jackc/pgx/v5/pgconn"
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
	sourceTypeHN      = "hn"
	hnBaseURL         = "https://hacker-news.firebaseio.com/v0"
	runInterval       = 15 * time.Minute
	requestTimeout    = 30 * time.Second
	defaultMinScore   = 10
)

var htmlTagPattern = regexp.MustCompile(`<[^>]*>`)

type newArticleEvent struct {
	ArticleID string `json:"article_id"`
}

type hnItem struct {
	ID          int64  `json:"id"`
	Type        string `json:"type"`
	By          string `json:"by"`
	Time        int64  `json:"time"`
	Text        string `json:"text"`
	URL         string `json:"url"`
	Title       string `json:"title"`
	Score       int    `json:"score"`
	Descendants int    `json:"descendants"`
}

type hnWorker struct {
	store      *store.Store
	queue      *queue.Queue
	checker    *dedup.Checker
	httpClient *http.Client
	minScore   int
	sourceID   string
}

type hnRunStats struct {
	ListsFetched     int
	StoriesProcessed int
	NewArticles      int
	SkippedLowScore  int
	SkippedSeen      int
	Errors           int
}

func main() {
	cfg := config.Load()
	setupLogging(cfg.LogLevel)

	log.Info("Starting Flux Hacker News worker")

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

	limits := copyRateLimits(cfg.RateLimits)
	if _, ok := limits["hacker-news.firebaseio.com"]; !ok {
		limits["hacker-news.firebaseio.com"] = "30/min"
	}

	limiter, err := ratelimit.New(rdb, ratelimit.Config{
		Limits:    limits,
		UserAgent: cfg.UserAgent,
	})
	if err != nil {
		log.WithError(err).Fatal("Failed to initialize rate limiter")
	}

	sourceID, err := resolveHNSourceID(ctx, db)
	if err != nil {
		log.WithError(err).Fatal("Failed to resolve HN source from database")
	}
	if sourceID == "" {
		log.Warn("No enabled HN source found in sources table, skipping run")
		return
	}

	worker := &hnWorker{
		store:      db,
		queue:      q,
		checker:    dedup.NewChecker(rdb),
		httpClient: ratelimit.NewHTTPClient(limiter, requestTimeout),
		minScore:   parseMinScore(),
		sourceID:   sourceID,
	}

	mode := parseWorkerMode()
	for {
		runStart := time.Now()
		stats, err := worker.runOnce(ctx)
		if err != nil {
			log.WithError(err).Error("HN worker run failed")
		}

		log.WithFields(log.Fields{
			"mode":              mode,
			"lists_fetched":     stats.ListsFetched,
			"stories_processed": stats.StoriesProcessed,
			"new_articles":      stats.NewArticles,
			"skipped_low_score": stats.SkippedLowScore,
			"skipped_seen":      stats.SkippedSeen,
			"errors":            stats.Errors,
			"elapsed_ms":        time.Since(runStart).Milliseconds(),
		}).Info("HN worker run completed")

		if mode != workerModeDaemon {
			break
		}

		log.WithField("sleep", runInterval.String()).Info("HN daemon sleeping")
		select {
		case <-ctx.Done():
			log.Info("HN worker shutting down")
			return
		case <-time.After(runInterval):
		}
	}

	log.Info("HN worker finished")
}

func (w *hnWorker) runOnce(ctx context.Context) (hnRunStats, error) {
	stats := hnRunStats{}

	endpoints := []string{
		hnBaseURL + "/topstories.json",
		hnBaseURL + "/beststories.json",
		hnBaseURL + "/newstories.json",
	}

	seenIDs := make(map[int64]struct{})
	storyIDs := make([]int64, 0, 1500)

	for _, endpoint := range endpoints {
		var ids []int64
		if err := w.fetchJSON(ctx, endpoint, &ids); err != nil {
			_ = w.store.UpdateSourceFetchStatus(ctx, w.sourceID, err)
			return stats, fmt.Errorf("fetching story ids from %s: %w", endpoint, err)
		}
		stats.ListsFetched++
		for _, id := range ids {
			if _, exists := seenIDs[id]; exists {
				continue
			}
			seenIDs[id] = struct{}{}
			storyIDs = append(storyIDs, id)
		}
	}

	for _, storyID := range storyIDs {
		itemURL := fmt.Sprintf("%s/item/%d.json", hnBaseURL, storyID)
		item := &hnItem{}
		if err := w.fetchJSON(ctx, itemURL, item); err != nil {
			stats.Errors++
			log.WithFields(log.Fields{
				"story_id": storyID,
				"url":      itemURL,
			}).WithError(err).Error("Failed to fetch HN item")
			continue
		}
		if item.ID == 0 || item.Type != "story" {
			continue
		}

		stats.StoriesProcessed++

		if item.Score <= w.minScore {
			stats.SkippedLowScore++
			continue
		}

		articleURL := strings.TrimSpace(item.URL)
		if articleURL == "" {
			articleURL = fmt.Sprintf("https://news.ycombinator.com/item?id=%d", item.ID)
		}
		articleURL = dedup.NormalizeURL(articleURL)

		isNew, err := w.checker.IsNew(ctx, articleURL)
		if err != nil {
			stats.Errors++
			log.WithFields(log.Fields{
				"story_id": item.ID,
				"url":      articleURL,
			}).WithError(err).Error("Dedup check failed for HN story")
			continue
		}
		if !isNew {
			stats.SkippedSeen++
			continue
		}

		content := ""
		if strings.TrimSpace(item.URL) != "" {
			content, err = w.fetchReadableContent(ctx, articleURL)
			if err != nil {
				log.WithFields(log.Fields{
					"story_id": item.ID,
					"url":      articleURL,
				}).WithError(err).Warn("Failed to fetch readable content, using HN text fallback")
				content = cleanText(item.Text)
			}
		} else {
			content = cleanText(item.Text)
		}

		var contentPtr *string
		if content != "" {
			contentPtr = &content
		}

		var author *string
		authorName := strings.TrimSpace(item.By)
		if authorName != "" {
			author = &authorName
		}

		published := time.Unix(item.Time, 0).UTC()
		publishedPtr := &published

		title := strings.TrimSpace(item.Title)
		if title == "" {
			title = fmt.Sprintf("HN story %d", item.ID)
		}

		metadata, err := json.Marshal(map[string]interface{}{
			"hn_score":    item.Score,
			"hn_comments": item.Descendants,
			"hn_id":       item.ID,
			"hn_type":     item.Type,
		})
		if err != nil {
			stats.Errors++
			log.WithError(err).WithField("story_id", item.ID).Error("Failed to marshal HN metadata")
			continue
		}

		article := &models.Article{
			SourceType:  sourceTypeHN,
			SourceID:    strconv.FormatInt(item.ID, 10),
			URL:         articleURL,
			Title:       title,
			Content:     contentPtr,
			Author:      author,
			PublishedAt: publishedPtr,
			Status:      models.StatusPending,
			Metadata:    metadata,
		}

		if err := w.store.CreateArticle(ctx, article); err != nil {
			if isUniqueViolation(err) {
				stats.SkippedSeen++
				continue
			}
			stats.Errors++
			log.WithFields(log.Fields{
				"story_id": item.ID,
				"url":      articleURL,
			}).WithError(err).Error("Failed to insert HN article")
			continue
		}

		if err := w.queue.Publish(queue.SubjectArticlesNew, newArticleEvent{ArticleID: article.ID}); err != nil {
			stats.Errors++
			log.WithField("article_id", article.ID).WithError(err).Error("Failed to publish articles.new")
			continue
		}

		stats.NewArticles++
	}

	if err := w.store.UpdateSourceFetchStatus(ctx, w.sourceID, nil); err != nil {
		log.WithField("source_id", w.sourceID).WithError(err).Warn("Failed to update HN source fetch status")
	}

	return stats, nil
}

func (w *hnWorker) fetchReadableContent(ctx context.Context, url string) (string, error) {
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

func (w *hnWorker) fetchJSON(ctx context.Context, url string, out interface{}) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}

	resp, err := w.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("unexpected status code %d", resp.StatusCode)
	}

	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		return fmt.Errorf("decoding response: %w", err)
	}
	return nil
}

func resolveHNSourceID(ctx context.Context, db *store.Store) (string, error) {
	sources, err := db.ListSourcesByTypeWithSectionIDs(ctx, sourceTypeHN, true)
	if err != nil {
		return "", err
	}
	if len(sources) == 0 {
		return "", nil
	}
	if len(sources) > 1 {
		log.WithField("count", len(sources)).Warn("Multiple enabled HN sources found; using the first one")
	}
	return sources[0].Source.ID, nil
}

func copyRateLimits(in map[string]string) map[string]string {
	out := make(map[string]string, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

func cleanText(raw string) string {
	raw = htmlTagPattern.ReplaceAllString(raw, " ")
	raw = html.UnescapeString(raw)
	return strings.TrimSpace(strings.Join(strings.Fields(raw), " "))
}

func parseMinScore() int {
	raw := strings.TrimSpace(os.Getenv("HN_MIN_SCORE"))
	if raw == "" {
		return defaultMinScore
	}
	score, err := strconv.Atoi(raw)
	if err != nil {
		log.WithField("HN_MIN_SCORE", raw).Warn("Invalid HN_MIN_SCORE, using default")
		return defaultMinScore
	}
	return score
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
