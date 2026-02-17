package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

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
	sourceTypeGitHub  = "github"

	githubAPIBase  = "https://api.github.com"
	requestTimeout = 30 * time.Second
	runInterval    = time.Hour
	releaseLimit   = 5
)

type newArticleEvent struct {
	ArticleID string `json:"article_id"`
}

type githubSourceConfig struct {
	Repo  string `json:"repo"`
	Owner string `json:"owner,omitempty"`
	Name  string `json:"name,omitempty"`
}

type githubRelease struct {
	TagName     string `json:"tag_name"`
	Name        string `json:"name"`
	Body        string `json:"body"`
	HTMLURL     string `json:"html_url"`
	Prerelease  bool   `json:"prerelease"`
	Draft       bool   `json:"draft"`
	PublishedAt string `json:"published_at"`
	CreatedAt   string `json:"created_at"`
	Author      *struct {
		Login string `json:"login"`
	} `json:"author"`
}

type githubWorker struct {
	store      *store.Store
	queue      *queue.Queue
	httpClient *http.Client
	token      string
}

type githubRunStats struct {
	SourcesProcessed int
	ReleasesSeen     int
	NewArticles      int
	SkippedSeen      int
	SourceErrors     int
}

type sourceRunStats struct {
	ReleasesSeen int
	NewArticles  int
	SkippedSeen  int
}

func main() {
	cfg := config.Load()
	setupLogging(cfg.LogLevel)

	log.Info("Starting Flux GitHub releases worker")

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
	if _, ok := limits["api.github.com"]; !ok {
		limits["api.github.com"] = "5000/hour"
	}

	limiter, err := ratelimit.New(rdb, ratelimit.Config{
		Limits:    limits,
		UserAgent: cfg.UserAgent,
	})
	if err != nil {
		log.WithError(err).Fatal("Failed to initialize rate limiter")
	}

	token := strings.TrimSpace(os.Getenv("GITHUB_TOKEN"))
	if token == "" {
		log.Fatal("GITHUB_TOKEN is required")
	}

	worker := &githubWorker{
		store:      db,
		queue:      q,
		httpClient: ratelimit.NewHTTPClient(limiter, requestTimeout),
		token:      token,
	}

	mode := parseWorkerMode()
	for {
		runStart := time.Now()
		stats, err := worker.runOnce(ctx)
		if err != nil {
			log.WithError(err).Error("GitHub worker run failed")
		}

		log.WithFields(log.Fields{
			"mode":              mode,
			"sources_processed": stats.SourcesProcessed,
			"releases_seen":     stats.ReleasesSeen,
			"new_articles":      stats.NewArticles,
			"skipped_seen":      stats.SkippedSeen,
			"source_errors":     stats.SourceErrors,
			"elapsed_ms":        time.Since(runStart).Milliseconds(),
		}).Info("GitHub worker run completed")

		if mode != workerModeDaemon {
			break
		}

		log.WithField("sleep", runInterval.String()).Info("GitHub daemon sleeping")
		select {
		case <-ctx.Done():
			log.Info("GitHub worker shutting down")
			return
		case <-time.After(runInterval):
		}
	}

	log.Info("GitHub worker finished")
}

func (w *githubWorker) runOnce(ctx context.Context) (githubRunStats, error) {
	stats := githubRunStats{}

	sources, err := w.store.ListSourcesByTypeWithSectionIDs(ctx, sourceTypeGitHub, true)
	if err != nil {
		return stats, fmt.Errorf("listing enabled github sources: %w", err)
	}

	for _, src := range sources {
		sourceStats, err := w.processSource(ctx, src)
		stats.SourcesProcessed++
		stats.ReleasesSeen += sourceStats.ReleasesSeen
		stats.NewArticles += sourceStats.NewArticles
		stats.SkippedSeen += sourceStats.SkippedSeen
		if err != nil {
			stats.SourceErrors++
			log.WithFields(log.Fields{
				"source_id": src.Source.ID,
				"source":    src.Source.Name,
				"error":     err.Error(),
			}).Error("Failed to process GitHub source")
			continue
		}
	}

	return stats, nil
}

func (w *githubWorker) processSource(ctx context.Context, src *store.SourceWithSectionIDs) (sourceRunStats, error) {
	stats := sourceRunStats{}

	cfg, err := parseGitHubSourceConfig(src.Source.Config)
	if err != nil {
		_ = w.store.UpdateSourceFetchStatus(ctx, src.Source.ID, err)
		return stats, err
	}

	releases, err := w.fetchReleases(ctx, cfg.Repo)
	if err != nil {
		_ = w.store.UpdateSourceFetchStatus(ctx, src.Source.ID, err)
		return stats, fmt.Errorf("fetching releases for %s: %w", cfg.Repo, err)
	}

	var sectionID *string
	if len(src.SectionIDs) == 1 {
		sectionID = &src.SectionIDs[0]
	}

	for _, rel := range releases {
		if rel.Draft {
			continue
		}
		tag := strings.TrimSpace(rel.TagName)
		if tag == "" {
			continue
		}

		stats.ReleasesSeen++

		sourceID := fmt.Sprintf("%s:%s", cfg.Repo, tag)
		title := strings.TrimSpace(rel.Name)
		if title == "" {
			title = fmt.Sprintf("%s %s", cfg.Repo, tag)
		}

		releaseURL := strings.TrimSpace(rel.HTMLURL)
		if releaseURL == "" {
			releaseURL = fmt.Sprintf("https://github.com/%s/releases/tag/%s", cfg.Repo, tag)
		}
		releaseURL = dedup.NormalizeURL(releaseURL)

		content := strings.TrimSpace(rel.Body)
		var contentPtr *string
		if content != "" {
			contentPtr = &content
		}

		var author *string
		if rel.Author != nil {
			login := strings.TrimSpace(rel.Author.Login)
			if login != "" {
				author = &login
			}
		}

		publishedAt := parseReleaseTime(rel.PublishedAt)
		if publishedAt == nil {
			publishedAt = parseReleaseTime(rel.CreatedAt)
		}

		metadata, err := json.Marshal(map[string]interface{}{
			"repo":        cfg.Repo,
			"tag":         tag,
			"prerelease":  rel.Prerelease,
			"source_name": cfg.Repo,
			"source_ref":  src.Source.ID,
		})
		if err != nil {
			log.WithError(err).Warn("Failed to marshal GitHub metadata")
			metadata = []byte("{}")
		}

		article := &models.Article{
			SourceType:  sourceTypeGitHub,
			SourceID:    sourceID,
			SectionID:   sectionID,
			URL:         releaseURL,
			Title:       title,
			Content:     contentPtr,
			Author:      author,
			PublishedAt: publishedAt,
			Status:      models.StatusPending,
			Metadata:    metadata,
		}

		if err := w.store.CreateArticle(ctx, article); err != nil {
			if isUniqueViolation(err) {
				stats.SkippedSeen++
				continue
			}
			log.WithFields(log.Fields{
				"source_id": src.Source.ID,
				"repo":      cfg.Repo,
				"tag":       tag,
			}).WithError(err).Error("Failed to insert GitHub release article")
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
		"repo":          cfg.Repo,
		"releases_seen": stats.ReleasesSeen,
		"new_articles":  stats.NewArticles,
		"section_links": len(src.SectionIDs),
	}).Info("GitHub source processed")

	return stats, nil
}

func parseGitHubSourceConfig(raw json.RawMessage) (*githubSourceConfig, error) {
	cfg := &githubSourceConfig{}
	if err := json.Unmarshal(raw, cfg); err != nil {
		return nil, fmt.Errorf("parsing source config: %w", err)
	}

	repo := strings.TrimSpace(cfg.Repo)
	if repo == "" && cfg.Owner != "" && cfg.Name != "" {
		repo = strings.TrimSpace(cfg.Owner) + "/" + strings.TrimSpace(cfg.Name)
	}
	repo = strings.Trim(repo, "/")
	parts := strings.Split(repo, "/")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return nil, errors.New("github source config requires repo in owner/repo format")
	}
	cfg.Repo = parts[0] + "/" + parts[1]
	return cfg, nil
}

func (w *githubWorker) fetchReleases(ctx context.Context, repo string) ([]githubRelease, error) {
	url := fmt.Sprintf("%s/repos/%s/releases?per_page=%d", githubAPIBase, repo, releaseLimit)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+w.token)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")

	resp, err := w.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return nil, fmt.Errorf("github api status %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	var releases []githubRelease
	if err := json.NewDecoder(resp.Body).Decode(&releases); err != nil {
		return nil, fmt.Errorf("decoding releases response: %w", err)
	}
	return releases, nil
}

func parseReleaseTime(raw string) *time.Time {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	ts, err := time.Parse(time.RFC3339, raw)
	if err != nil {
		return nil
	}
	t := ts.UTC()
	return &t
}

func copyRateLimits(in map[string]string) map[string]string {
	out := make(map[string]string, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
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
