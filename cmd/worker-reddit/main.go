package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"io"
	"net/http"
	nurl "net/url"
	"os"
	"os/signal"
	"regexp"
	"strings"
	"sync"
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
	sourceTypeReddit  = "reddit"

	redditOAuthURL = "https://www.reddit.com/api/v1/access_token"
	redditAPIBase  = "https://oauth.reddit.com"

	requestTimeout  = 30 * time.Second
	runInterval     = 30 * time.Minute
	defaultMinScore = 20
	defaultSort     = "hot"
	defaultLimit    = 50
)

var htmlTagPattern = regexp.MustCompile(`<[^>]*>`)

type newArticleEvent struct {
	ArticleID string `json:"article_id"`
}

type redditSourceConfig struct {
	Subreddit string `json:"subreddit"`
	MinScore  int    `json:"min_score"`
	Sort      string `json:"sort,omitempty"`
	Limit     int    `json:"limit,omitempty"`
}

type redditListingResponse struct {
	Data struct {
		Children []struct {
			Data redditPost `json:"data"`
		} `json:"children"`
	} `json:"data"`
}

type redditPost struct {
	ID          string  `json:"id"`
	Name        string  `json:"name"`
	Title       string  `json:"title"`
	SelfText    string  `json:"selftext"`
	URL         string  `json:"url"`
	Permalink   string  `json:"permalink"`
	Author      string  `json:"author"`
	CreatedUTC  float64 `json:"created_utc"`
	Score       int     `json:"score"`
	NumComments int     `json:"num_comments"`
	IsSelf      bool    `json:"is_self"`
	Stickied    bool    `json:"stickied"`
}

type redditTokenResponse struct {
	AccessToken string `json:"access_token"`
	TokenType   string `json:"token_type"`
	ExpiresIn   int    `json:"expires_in"`
	Scope       string `json:"scope"`
}

type redditWorker struct {
	store      *store.Store
	queue      *queue.Queue
	checker    *dedup.Checker
	httpClient *http.Client
	oauth      *redditOAuthClient
}

type redditOAuthClient struct {
	httpClient   *http.Client
	clientID     string
	clientSecret string
	username     string
	password     string

	mu        sync.Mutex
	token     string
	expiresAt time.Time
}

type redditRunStats struct {
	SourcesProcessed int
	PostsSeen        int
	NewArticles      int
	SkippedLowScore  int
	SkippedSeen      int
	SourceErrors     int
}

type sourceRunStats struct {
	PostsSeen       int
	NewArticles     int
	SkippedLowScore int
	SkippedSeen     int
}

func main() {
	cfg := config.Load()
	setupLogging(cfg.LogLevel)

	log.Info("Starting Flux Reddit worker")

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
	if _, ok := limits["oauth.reddit.com"]; !ok {
		limits["oauth.reddit.com"] = "60/min"
	}
	if _, ok := limits["reddit.com"]; !ok {
		limits["reddit.com"] = "60/min"
	}

	limiter, err := ratelimit.New(rdb, ratelimit.Config{
		Limits:    limits,
		UserAgent: cfg.UserAgent,
	})
	if err != nil {
		log.WithError(err).Fatal("Failed to initialize rate limiter")
	}

	httpClient := ratelimit.NewHTTPClient(limiter, requestTimeout)
	oauth, err := newRedditOAuthClient(httpClient)
	if err != nil {
		log.WithError(err).Fatal("Failed to initialize Reddit OAuth credentials")
	}

	worker := &redditWorker{
		store:      db,
		queue:      q,
		checker:    dedup.NewChecker(rdb),
		httpClient: httpClient,
		oauth:      oauth,
	}

	mode := parseWorkerMode()
	for {
		runStart := time.Now()
		stats, err := worker.runOnce(ctx)
		if err != nil {
			log.WithError(err).Error("Reddit worker run failed")
		}

		log.WithFields(log.Fields{
			"mode":              mode,
			"sources_processed": stats.SourcesProcessed,
			"posts_seen":        stats.PostsSeen,
			"new_articles":      stats.NewArticles,
			"skipped_low_score": stats.SkippedLowScore,
			"skipped_seen":      stats.SkippedSeen,
			"source_errors":     stats.SourceErrors,
			"elapsed_ms":        time.Since(runStart).Milliseconds(),
		}).Info("Reddit worker run completed")

		if mode != workerModeDaemon {
			break
		}

		log.WithField("sleep", runInterval.String()).Info("Reddit daemon sleeping")
		select {
		case <-ctx.Done():
			log.Info("Reddit worker shutting down")
			return
		case <-time.After(runInterval):
		}
	}

	log.Info("Reddit worker finished")
}

func newRedditOAuthClient(httpClient *http.Client) (*redditOAuthClient, error) {
	clientID := strings.TrimSpace(os.Getenv("REDDIT_CLIENT_ID"))
	clientSecret := strings.TrimSpace(os.Getenv("REDDIT_CLIENT_SECRET"))
	username := strings.TrimSpace(os.Getenv("REDDIT_USERNAME"))
	password := strings.TrimSpace(os.Getenv("REDDIT_PASSWORD"))

	missing := make([]string, 0, 4)
	if clientID == "" {
		missing = append(missing, "REDDIT_CLIENT_ID")
	}
	if clientSecret == "" {
		missing = append(missing, "REDDIT_CLIENT_SECRET")
	}
	if username == "" {
		missing = append(missing, "REDDIT_USERNAME")
	}
	if password == "" {
		missing = append(missing, "REDDIT_PASSWORD")
	}
	if len(missing) > 0 {
		return nil, fmt.Errorf("missing required env vars: %s", strings.Join(missing, ", "))
	}

	return &redditOAuthClient{
		httpClient:   httpClient,
		clientID:     clientID,
		clientSecret: clientSecret,
		username:     username,
		password:     password,
	}, nil
}

func (c *redditOAuthClient) AccessToken(ctx context.Context) (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.token != "" && time.Now().Before(c.expiresAt.Add(-30*time.Second)) {
		return c.token, nil
	}

	token, expiresAt, err := c.refreshToken(ctx)
	if err != nil {
		return "", err
	}
	c.token = token
	c.expiresAt = expiresAt
	return c.token, nil
}

func (c *redditOAuthClient) InvalidateToken() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.token = ""
	c.expiresAt = time.Time{}
}

func (c *redditOAuthClient) refreshToken(ctx context.Context) (string, time.Time, error) {
	form := nurl.Values{}
	form.Set("grant_type", "password")
	form.Set("username", c.username)
	form.Set("password", c.password)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, redditOAuthURL, strings.NewReader(form.Encode()))
	if err != nil {
		return "", time.Time{}, err
	}
	req.SetBasicAuth(c.clientID, c.clientSecret)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", time.Time{}, err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return "", time.Time{}, fmt.Errorf("reddit oauth status %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	var out redditTokenResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return "", time.Time{}, fmt.Errorf("decoding reddit oauth response: %w", err)
	}
	if strings.TrimSpace(out.AccessToken) == "" {
		return "", time.Time{}, errors.New("reddit oauth response missing access_token")
	}

	expiresIn := out.ExpiresIn
	if expiresIn <= 0 {
		expiresIn = 3600
	}

	expiresAt := time.Now().Add(time.Duration(expiresIn) * time.Second)
	return strings.TrimSpace(out.AccessToken), expiresAt, nil
}

func (w *redditWorker) runOnce(ctx context.Context) (redditRunStats, error) {
	stats := redditRunStats{}

	sources, err := w.store.ListSourcesByTypeWithSectionIDs(ctx, sourceTypeReddit, true)
	if err != nil {
		return stats, fmt.Errorf("listing enabled reddit sources: %w", err)
	}

	for _, src := range sources {
		sourceStats, err := w.processSubredditSource(ctx, src)
		stats.SourcesProcessed++
		stats.PostsSeen += sourceStats.PostsSeen
		stats.NewArticles += sourceStats.NewArticles
		stats.SkippedLowScore += sourceStats.SkippedLowScore
		stats.SkippedSeen += sourceStats.SkippedSeen

		if err != nil {
			stats.SourceErrors++
			log.WithFields(log.Fields{
				"source_id": src.Source.ID,
				"source":    src.Source.Name,
				"error":     err.Error(),
			}).Error("Failed to process subreddit source")
			continue
		}
	}

	return stats, nil
}

func (w *redditWorker) processSubredditSource(ctx context.Context, src *store.SourceWithSectionIDs) (sourceRunStats, error) {
	stats := sourceRunStats{}

	cfg, err := parseRedditSourceConfig(src.Source.Config)
	if err != nil {
		_ = w.store.UpdateSourceFetchStatus(ctx, src.Source.ID, err)
		return stats, err
	}

	posts, err := w.fetchSubredditPosts(ctx, cfg)
	if err != nil {
		_ = w.store.UpdateSourceFetchStatus(ctx, src.Source.ID, err)
		return stats, fmt.Errorf("fetching r/%s: %w", cfg.Subreddit, err)
	}

	var sectionID *string
	if len(src.SectionIDs) == 1 {
		sectionID = &src.SectionIDs[0]
	}

	for _, post := range posts {
		stats.PostsSeen++

		if post.Stickied {
			continue
		}
		if post.Score <= cfg.MinScore {
			stats.SkippedLowScore++
			continue
		}

		permalink := normalizePermalink(post.Permalink)
		articleURL := permalink
		if !post.IsSelf {
			rawURL := strings.TrimSpace(post.URL)
			if rawURL != "" {
				articleURL = dedup.NormalizeURL(rawURL)
			}
			if articleURL == "" {
				articleURL = permalink
			}
			isNew, dedupErr := w.checker.IsNew(ctx, articleURL)
			if dedupErr != nil {
				log.WithFields(log.Fields{
					"source_id":   src.Source.ID,
					"subreddit":   cfg.Subreddit,
					"reddit_post": post.ID,
					"url":         articleURL,
				}).WithError(dedupErr).Error("Dedup check failed for Reddit link post")
				continue
			}
			if !isNew {
				stats.SkippedSeen++
				continue
			}
		}

		content := ""
		if post.IsSelf {
			content = strings.TrimSpace(post.SelfText)
		} else {
			content, err = w.fetchReadableContent(ctx, articleURL)
			if err != nil {
				log.WithFields(log.Fields{
					"source_id":   src.Source.ID,
					"subreddit":   cfg.Subreddit,
					"reddit_post": post.ID,
					"url":         articleURL,
				}).WithError(err).Warn("Failed to fetch readable content, falling back to selftext")
				content = strings.TrimSpace(post.SelfText)
			}
		}

		var contentPtr *string
		if content != "" {
			contentPtr = &content
		}

		title := strings.TrimSpace(post.Title)
		if title == "" {
			title = articleURL
		}

		var author *string
		authorName := strings.TrimSpace(post.Author)
		if authorName != "" {
			author = &authorName
		}

		var publishedAt *time.Time
		if post.CreatedUTC > 0 {
			ts := time.Unix(int64(post.CreatedUTC), 0).UTC()
			publishedAt = &ts
		}

		metadata, err := json.Marshal(map[string]interface{}{
			"reddit_score":    post.Score,
			"reddit_comments": post.NumComments,
			"subreddit":       cfg.Subreddit,
			"reddit_id":       post.ID,
			"is_self":         post.IsSelf,
			"source_name":     fmt.Sprintf("r/%s", cfg.Subreddit),
			"source_ref":      src.Source.ID,
			"permalink":       permalink,
		})
		if err != nil {
			log.WithError(err).Warn("Failed to marshal Reddit metadata")
			metadata = []byte("{}")
		}

		article := &models.Article{
			SourceType:  sourceTypeReddit,
			SourceID:    post.ID,
			SectionID:   sectionID,
			URL:         articleURL,
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
				"source_id":   src.Source.ID,
				"subreddit":   cfg.Subreddit,
				"reddit_post": post.ID,
			}).WithError(err).Error("Failed to insert Reddit article")
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
		"subreddit":     cfg.Subreddit,
		"posts_seen":    stats.PostsSeen,
		"new_articles":  stats.NewArticles,
		"section_links": len(src.SectionIDs),
	}).Info("Reddit source processed")

	return stats, nil
}

func (w *redditWorker) fetchSubredditPosts(ctx context.Context, cfg *redditSourceConfig) ([]redditPost, error) {
	token, err := w.oauth.AccessToken(ctx)
	if err != nil {
		return nil, fmt.Errorf("obtaining oauth token: %w", err)
	}

	posts, statusCode, err := w.fetchSubredditPostsWithToken(ctx, cfg, token)
	if err == nil {
		return posts, nil
	}
	if statusCode != http.StatusUnauthorized {
		return nil, err
	}

	w.oauth.InvalidateToken()
	token, tokenErr := w.oauth.AccessToken(ctx)
	if tokenErr != nil {
		return nil, fmt.Errorf("refreshing oauth token after 401: %w", tokenErr)
	}
	posts, _, err = w.fetchSubredditPostsWithToken(ctx, cfg, token)
	return posts, err
}

func (w *redditWorker) fetchSubredditPostsWithToken(ctx context.Context, cfg *redditSourceConfig, token string) ([]redditPost, int, error) {
	url := fmt.Sprintf("%s/r/%s/%s.json?limit=%d", redditAPIBase, cfg.Subreddit, cfg.Sort, cfg.Limit)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, 0, err
	}
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := w.httpClient.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return nil, resp.StatusCode, fmt.Errorf("reddit api status %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	var listing redditListingResponse
	if err := json.NewDecoder(resp.Body).Decode(&listing); err != nil {
		return nil, resp.StatusCode, fmt.Errorf("decoding subreddit response: %w", err)
	}

	posts := make([]redditPost, 0, len(listing.Data.Children))
	for _, child := range listing.Data.Children {
		if strings.TrimSpace(child.Data.ID) == "" {
			continue
		}
		posts = append(posts, child.Data)
	}
	return posts, resp.StatusCode, nil
}

func (w *redditWorker) fetchReadableContent(ctx context.Context, url string) (string, error) {
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

func parseRedditSourceConfig(raw json.RawMessage) (*redditSourceConfig, error) {
	cfg := &redditSourceConfig{}
	if err := json.Unmarshal(raw, cfg); err != nil {
		return nil, fmt.Errorf("parsing source config: %w", err)
	}

	cfg.Subreddit = normalizeSubreddit(cfg.Subreddit)
	if cfg.Subreddit == "" {
		return nil, errors.New("reddit source config missing subreddit")
	}

	if cfg.MinScore < 0 {
		cfg.MinScore = defaultMinScore
	}
	if cfg.MinScore == 0 {
		cfg.MinScore = defaultMinScore
	}

	cfg.Sort = normalizeRedditSort(cfg.Sort)
	if cfg.Limit <= 0 || cfg.Limit > 100 {
		cfg.Limit = defaultLimit
	}

	return cfg, nil
}

func normalizeSubreddit(raw string) string {
	raw = strings.TrimSpace(raw)
	raw = strings.TrimPrefix(strings.ToLower(raw), "r/")
	raw = strings.Trim(raw, "/")
	return raw
}

func normalizeRedditSort(raw string) string {
	raw = strings.ToLower(strings.TrimSpace(raw))
	switch raw {
	case "hot", "new", "top", "rising":
		return raw
	default:
		return defaultSort
	}
}

func normalizePermalink(permalink string) string {
	permalink = strings.TrimSpace(permalink)
	if permalink == "" {
		return ""
	}
	if strings.HasPrefix(permalink, "http://") || strings.HasPrefix(permalink, "https://") {
		return dedup.NormalizeURL(permalink)
	}
	return dedup.NormalizeURL("https://www.reddit.com" + permalink)
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
