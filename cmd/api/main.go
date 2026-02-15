package main

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/mmcdole/gofeed"
	"github.com/nats-io/nats.go"
	"github.com/redis/go-redis/v9"
	log "github.com/sirupsen/logrus"
	"github.com/zyrak/flux/internal/config"
	"github.com/zyrak/flux/internal/embeddings"
	"github.com/zyrak/flux/internal/models"
	"github.com/zyrak/flux/internal/profile"
	"github.com/zyrak/flux/internal/store"
)

type articleSectionResponse struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	DisplayName string `json:"display_name"`
}

type articleSourceResponse struct {
	Type string  `json:"type"`
	ID   string  `json:"id"`
	Name string  `json:"name"`
	Ref  *string `json:"ref,omitempty"`
}

type articleFeedbackResponse struct {
	Likes     int     `json:"likes"`
	Dislikes  int     `json:"dislikes"`
	Saves     int     `json:"saves"`
	Liked     bool    `json:"liked"`
	Disliked  bool    `json:"disliked"`
	Saved     bool    `json:"saved"`
	LikeID    *string `json:"like_id,omitempty"`
	DislikeID *string `json:"dislike_id,omitempty"`
	SaveID    *string `json:"save_id,omitempty"`
}

type articleResponse struct {
	ID             string                  `json:"id"`
	SourceType     string                  `json:"source_type"`
	SourceID       string                  `json:"source_id"`
	URL            string                  `json:"url"`
	Title          string                  `json:"title"`
	Content        *string                 `json:"content,omitempty"`
	Summary        *string                 `json:"summary,omitempty"`
	Author         *string                 `json:"author,omitempty"`
	PublishedAt    *time.Time              `json:"published_at,omitempty"`
	IngestedAt     time.Time               `json:"ingested_at"`
	ProcessedAt    *time.Time              `json:"processed_at,omitempty"`
	RelevanceScore *float64                `json:"relevance_score,omitempty"`
	Categories     []string                `json:"categories,omitempty"`
	Status         string                  `json:"status"`
	Metadata       json.RawMessage         `json:"metadata,omitempty"`
	Section        *articleSectionResponse `json:"section,omitempty"`
	Source         articleSourceResponse   `json:"source"`
	Feedback       articleFeedbackResponse `json:"feedback"`
}

type sourceStatsResponse struct {
	TotalIngested int     `json:"total_ingested"`
	Last24h       int     `json:"last_24h"`
	PassRatePct   float64 `json:"pass_rate_pct"`
}

type sourceResponse struct {
	ID            string                   `json:"id"`
	SourceType    string                   `json:"source_type"`
	Name          string                   `json:"name"`
	Config        json.RawMessage          `json:"config"`
	Enabled       bool                     `json:"enabled"`
	LastFetchedAt *time.Time               `json:"last_fetched_at,omitempty"`
	ErrorCount    int                      `json:"error_count"`
	LastError     *string                  `json:"last_error,omitempty"`
	Sections      []store.SourceSectionRef `json:"sections"`
	Stats         sourceStatsResponse      `json:"stats"`
}

type briefingListItem struct {
	ID          string          `json:"id"`
	GeneratedAt time.Time       `json:"generated_at"`
	Metadata    json.RawMessage `json:"metadata,omitempty"`
}

type briefingResponse struct {
	ID          string            `json:"id"`
	GeneratedAt time.Time         `json:"generated_at"`
	Content     string            `json:"content"`
	ArticleIDs  []string          `json:"article_ids"`
	Metadata    json.RawMessage   `json:"metadata,omitempty"`
	Articles    []articleResponse `json:"articles"`
}

type rssSourceConfig struct {
	URL string `json:"url"`
}

func main() {
	cfg := config.Load()
	setupLogging(cfg.LogLevel)

	log.Info("Starting Flux API server")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	db, err := store.New(ctx, cfg.DatabaseURL)
	if err != nil {
		log.WithError(err).Fatal("Failed to connect to database")
	}
	defer db.Close()

	migrationsDir := os.Getenv("MIGRATIONS_DIR")
	if migrationsDir == "" {
		migrationsDir = "migrations"
	}
	if err := db.RunMigrations(ctx, migrationsDir); err != nil {
		log.WithError(err).Fatal("Failed to run migrations")
	}

	nc, err := nats.Connect(cfg.NatsURL, nats.Timeout(5*time.Second))
	if err != nil {
		log.WithError(err).Fatal("Failed to connect to NATS")
	}
	defer func() {
		if err := nc.Drain(); err != nil {
			log.WithError(err).Warn("Failed to drain NATS connection")
		}
	}()

	redisOpts, err := redis.ParseURL(cfg.RedisURL)
	if err != nil {
		log.WithError(err).Fatal("Failed to parse REDIS_URL")
	}
	rdb := redis.NewClient(redisOpts)
	defer func() { _ = rdb.Close() }()

	if err := rdb.Ping(ctx).Err(); err != nil {
		log.WithError(err).Fatal("Failed to connect to Redis")
	}

	embedClient := embeddings.NewClient(cfg.EmbeddingsURL)
	profileRecalc := profile.NewRecalculator(db, embedClient, 0.7)

	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	r.Use(middleware.Timeout(30 * time.Second))

	r.Get("/healthz", healthzHandler(db, nc, rdb))

	r.Route("/api", func(r chi.Router) {
		r.Use(bearerAuthMiddleware(cfg.AuthToken))

		r.Get("/articles", listArticlesHandler(db))
		r.Get("/articles/{id}", getArticleHandler(db))

		r.Get("/sources", listSourcesHandler(db))
		r.Post("/sources", createSourceHandler(db))
		r.Patch("/sources/{id}", updateSourceHandler(db))
		r.Post("/sources/validate-rss", validateRSSHandler())

		r.Get("/sections", listSectionsHandler(db))
		r.Post("/sections", createSectionHandler(db))
		r.Patch("/sections/{id}", updateSectionHandler(db))
		r.Post("/sections/reorder", reorderSectionsHandler(db))

		r.Get("/briefings/latest", latestBriefingHandler(db))
		r.Get("/briefings", listBriefingsHandler(db))
		r.Get("/briefings/{id}", getBriefingHandler(db))

		r.Post("/feedback", createFeedbackHandler(db, profileRecalc, cfg))
		r.Get("/feedback/stats", feedbackStatsHandler(db))
		r.Delete("/feedback/{id}", deleteFeedbackHandler(db, profileRecalc, cfg))
	})

	addr := fmt.Sprintf(":%d", cfg.APIPort)
	srv := &http.Server{
		Addr:    addr,
		Handler: r,
	}

	go func() {
		log.WithField("addr", addr).Info("API server listening")
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.WithError(err).Fatal("Server failed")
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Info("Shutting down API server...")
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.WithError(err).Error("Server shutdown error")
	}
}

func setupLogging(level string) {
	log.SetFormatter(&log.JSONFormatter{})
	lvl, err := log.ParseLevel(level)
	if err != nil {
		lvl = log.InfoLevel
	}
	log.SetLevel(lvl)
}

func bearerAuthMiddleware(authToken string) func(http.Handler) http.Handler {
	authToken = strings.TrimSpace(authToken)
	if authToken == "" {
		return func(next http.Handler) http.Handler { return next }
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			authHeader := strings.TrimSpace(r.Header.Get("Authorization"))
			if !strings.HasPrefix(authHeader, "Bearer ") {
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}

			provided := strings.TrimSpace(strings.TrimPrefix(authHeader, "Bearer "))
			if subtle.ConstantTimeCompare([]byte(provided), []byte(authToken)) != 1 {
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

func healthzHandler(db *store.Store, nc *nats.Conn, rdb *redis.Client) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
		defer cancel()

		services := map[string]string{}
		healthy := true

		if err := db.Pool().Ping(ctx); err != nil {
			healthy = false
			services["postgres"] = "error: " + err.Error()
		} else {
			services["postgres"] = "ok"
		}

		if err := rdb.Ping(ctx).Err(); err != nil {
			healthy = false
			services["redis"] = "error: " + err.Error()
		} else {
			services["redis"] = "ok"
		}

		if nc == nil || !nc.IsConnected() {
			healthy = false
			services["nats"] = "error: disconnected"
		} else if err := nc.FlushTimeout(2 * time.Second); err != nil {
			healthy = false
			services["nats"] = "error: " + err.Error()
		} else {
			services["nats"] = "ok"
		}

		statusCode := http.StatusOK
		status := "ok"
		if !healthy {
			statusCode = http.StatusServiceUnavailable
			status = "degraded"
		}

		respondJSONWithStatus(w, statusCode, map[string]interface{}{
			"status":   status,
			"services": services,
		})
	}
}

func listArticlesHandler(db *store.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		page := parsePositiveInt(r.URL.Query().Get("page"), 1)
		perPage := parsePositiveInt(r.URL.Query().Get("per_page"), 20)
		if perPage > 100 {
			perPage = 100
		}

		filter := store.ArticleListQuery{
			Limit:  perPage,
			Offset: (page - 1) * perPage,
		}

		if section := strings.TrimSpace(r.URL.Query().Get("section")); section != "" {
			filter.SectionName = &section
		}
		if sectionsRaw := strings.TrimSpace(r.URL.Query().Get("sections")); sectionsRaw != "" {
			parts := strings.Split(sectionsRaw, ",")
			filter.SectionNames = make([]string, 0, len(parts))
			for _, part := range parts {
				part = strings.TrimSpace(part)
				if part != "" {
					filter.SectionNames = append(filter.SectionNames, part)
				}
			}
			if len(filter.SectionNames) > 0 {
				filter.SectionName = nil
			}
		}
		if sourceType := strings.TrimSpace(r.URL.Query().Get("source_type")); sourceType != "" {
			filter.SourceType = &sourceType
		}
		if sourceRef := strings.TrimSpace(r.URL.Query().Get("source_ref")); sourceRef != "" {
			filter.SourceRef = &sourceRef
		}
		if status := strings.TrimSpace(r.URL.Query().Get("status")); status != "" {
			filter.Status = &status
		}
		filter.LikedOnly = parseBool(r.URL.Query().Get("liked_only"))

		if from := strings.TrimSpace(r.URL.Query().Get("from")); from != "" {
			t, err := parseISO8601(from)
			if err != nil {
				http.Error(w, "invalid 'from' datetime (use ISO 8601)", http.StatusBadRequest)
				return
			}
			filter.From = &t
		}
		if to := strings.TrimSpace(r.URL.Query().Get("to")); to != "" {
			t, err := parseISO8601(to)
			if err != nil {
				http.Error(w, "invalid 'to' datetime (use ISO 8601)", http.StatusBadRequest)
				return
			}
			filter.To = &t
		}

		articles, total, err := db.ListArticlesWithRelations(r.Context(), filter)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		out := make([]articleResponse, 0, len(articles))
		for _, a := range articles {
			out = append(out, mapArticleResponse(a))
		}

		totalPages := 0
		if perPage > 0 {
			totalPages = (total + perPage - 1) / perPage
		}

		respondJSON(w, map[string]interface{}{
			"data":        out,
			"articles":    out,
			"total":       total,
			"page":        page,
			"per_page":    perPage,
			"total_pages": totalPages,
		})
	}
}

func getArticleHandler(db *store.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		article, err := db.GetArticleWithRelationsByID(r.Context(), id)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if article == nil {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		respondJSON(w, mapArticleResponse(article))
	}
}

func mapArticleResponse(a *store.ArticleWithRelations) articleResponse {
	var section *articleSectionResponse
	if a.SectionID != nil {
		sectionID := *a.SectionID
		sectionName := ""
		sectionDisplayName := ""
		if a.SectionName != nil {
			sectionName = *a.SectionName
		}
		if a.SectionDisplayName != nil {
			sectionDisplayName = *a.SectionDisplayName
		}
		section = &articleSectionResponse{
			ID:          sectionID,
			Name:        sectionName,
			DisplayName: sectionDisplayName,
		}
	}

	return articleResponse{
		ID:             a.ID,
		SourceType:     a.SourceType,
		SourceID:       a.SourceID,
		URL:            a.URL,
		Title:          a.Title,
		Content:        a.Content,
		Summary:        a.Summary,
		Author:         a.Author,
		PublishedAt:    a.PublishedAt,
		IngestedAt:     a.IngestedAt,
		ProcessedAt:    a.ProcessedAt,
		RelevanceScore: a.RelevanceScore,
		Categories:     a.Categories,
		Status:         a.Status,
		Metadata:       a.Metadata,
		Section:        section,
		Source: articleSourceResponse{
			Type: a.SourceType,
			ID:   a.SourceID,
			Name: a.SourceName,
			Ref:  a.SourceRef,
		},
		Feedback: articleFeedbackResponse{
			Likes:     a.LikeCount,
			Dislikes:  a.DislikeCount,
			Saves:     a.SaveCount,
			Liked:     a.Liked,
			Disliked:  a.Disliked,
			Saved:     a.Saved,
			LikeID:    a.LatestLikeID,
			DislikeID: a.LatestDislikeID,
			SaveID:    a.LatestSaveID,
		},
	}
}

func listSourcesHandler(db *store.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		sources, err := db.ListSourcesWithSections(r.Context())
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		out := make([]sourceResponse, 0, len(sources))
		for _, src := range sources {
			out = append(out, mapSourceResponse(src))
		}
		respondJSON(w, out)
	}
}

func createSourceHandler(db *store.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			SourceType string          `json:"source_type"`
			Name       string          `json:"name"`
			Config     json.RawMessage `json:"config"`
			SectionIDs []string        `json:"section_ids"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid request body", http.StatusBadRequest)
			return
		}

		req.SourceType = strings.TrimSpace(req.SourceType)
		req.Name = strings.TrimSpace(req.Name)
		if req.SourceType == "" || req.Name == "" || len(req.Config) == 0 {
			http.Error(w, "source_type, name and config are required", http.StatusBadRequest)
			return
		}
		if req.SourceType == "rss" {
			if err := validateRSSConfig(req.Config); err != nil {
				http.Error(w, "invalid RSS feed URL: "+err.Error(), http.StatusBadRequest)
				return
			}
		}

		src := &models.Source{
			SourceType: req.SourceType,
			Name:       req.Name,
			Config:     req.Config,
			Enabled:    true,
		}

		if err := db.CreateSource(r.Context(), src, req.SectionIDs); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		created, err := db.GetSourceWithSectionsByID(r.Context(), src.ID)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if created == nil {
			http.Error(w, "created source not found", http.StatusInternalServerError)
			return
		}

		respondJSONWithStatus(w, http.StatusCreated, mapSourceResponse(created))
	}
}

func updateSourceHandler(db *store.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")

		var req struct {
			Name       *string          `json:"name,omitempty"`
			Config     *json.RawMessage `json:"config,omitempty"`
			Enabled    *bool            `json:"enabled,omitempty"`
			SectionIDs *[]string        `json:"section_ids,omitempty"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid request body", http.StatusBadRequest)
			return
		}

		if req.Name == nil && req.Config == nil && req.Enabled == nil && req.SectionIDs == nil {
			http.Error(w, "empty patch body", http.StatusBadRequest)
			return
		}

		src, err := db.GetSourceByID(r.Context(), id)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if src == nil {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}

		if req.Name != nil {
			src.Name = strings.TrimSpace(*req.Name)
		}
		if req.Config != nil {
			if src.SourceType == "rss" {
				if err := validateRSSConfig(*req.Config); err != nil {
					http.Error(w, "invalid RSS feed URL: "+err.Error(), http.StatusBadRequest)
					return
				}
			}
			src.Config = *req.Config
		}
		if req.Enabled != nil {
			src.Enabled = *req.Enabled
		}

		if err := db.UpdateSource(r.Context(), src); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if req.SectionIDs != nil {
			if err := db.ReplaceSourceSections(r.Context(), id, *req.SectionIDs); err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
		}

		updated, err := db.GetSourceWithSectionsByID(r.Context(), id)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if updated == nil {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}

		respondJSON(w, mapSourceResponse(updated))
	}
}

func mapSourceResponse(src *store.SourceWithSections) sourceResponse {
	return sourceResponse{
		ID:            src.Source.ID,
		SourceType:    src.Source.SourceType,
		Name:          src.Source.Name,
		Config:        src.Source.Config,
		Enabled:       src.Source.Enabled,
		LastFetchedAt: src.Source.LastFetchedAt,
		ErrorCount:    src.Source.ErrorCount,
		LastError:     src.Source.LastError,
		Sections:      src.Sections,
		Stats: sourceStatsResponse{
			TotalIngested: src.Stats.TotalIngested,
			Last24h:       src.Stats.Last24h,
			PassRatePct:   src.Stats.PassRatePct,
		},
	}
}

func validateRSSHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			URL string `json:"url"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid request body", http.StatusBadRequest)
			return
		}
		req.URL = strings.TrimSpace(req.URL)
		if req.URL == "" {
			http.Error(w, "url is required", http.StatusBadRequest)
			return
		}

		cfg, _ := json.Marshal(rssSourceConfig{URL: req.URL})
		if err := validateRSSConfig(cfg); err != nil {
			http.Error(w, "invalid RSS feed URL: "+err.Error(), http.StatusBadRequest)
			return
		}

		respondJSON(w, map[string]any{"valid": true})
	}
}

func validateRSSConfig(raw json.RawMessage) error {
	var cfg rssSourceConfig
	if err := json.Unmarshal(raw, &cfg); err != nil {
		return fmt.Errorf("invalid config JSON")
	}
	cfg.URL = strings.TrimSpace(cfg.URL)
	if cfg.URL == "" {
		return fmt.Errorf("missing config.url")
	}

	client := &http.Client{Timeout: 15 * time.Second}
	parser := gofeed.NewParser()
	parser.Client = client
	if _, err := parser.ParseURL(cfg.URL); err != nil {
		return err
	}
	return nil
}

func listSectionsHandler(db *store.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		sections, err := db.ListSectionsWithStats(r.Context())
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		respondJSON(w, sections)
	}
}

func createSectionHandler(db *store.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Name                string          `json:"name"`
			DisplayName         string          `json:"display_name"`
			Enabled             *bool           `json:"enabled,omitempty"`
			SortOrder           *int            `json:"sort_order,omitempty"`
			MaxBriefingArticles *int            `json:"max_briefing_articles,omitempty"`
			SeedKeywords        []string        `json:"seed_keywords,omitempty"`
			Config              json.RawMessage `json:"config,omitempty"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid request body", http.StatusBadRequest)
			return
		}

		name := strings.TrimSpace(strings.ToLower(req.Name))
		displayName := strings.TrimSpace(req.DisplayName)
		if name == "" || displayName == "" {
			http.Error(w, "name and display_name are required", http.StatusBadRequest)
			return
		}

		existing, err := db.GetSectionByName(r.Context(), name)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if existing != nil {
			http.Error(w, "section already exists", http.StatusConflict)
			return
		}

		enabled := true
		if req.Enabled != nil {
			enabled = *req.Enabled
		}

		sortOrder := 0
		if req.SortOrder != nil {
			sortOrder = *req.SortOrder
		} else {
			nextOrder, err := db.NextSectionSortOrder(r.Context())
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			sortOrder = nextOrder
		}

		maxBriefing := 5
		if req.MaxBriefingArticles != nil && *req.MaxBriefingArticles > 0 {
			maxBriefing = *req.MaxBriefingArticles
		}

		sec := &models.Section{
			Name:                name,
			DisplayName:         displayName,
			Enabled:             enabled,
			SortOrder:           sortOrder,
			MaxBriefingArticles: maxBriefing,
			SeedKeywords:        req.SeedKeywords,
			Config:              req.Config,
		}
		if len(sec.Config) == 0 {
			sec.Config = []byte("{}")
		}

		if err := db.CreateSection(r.Context(), sec); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		respondJSONWithStatus(w, http.StatusCreated, sec)
	}
}

func updateSectionHandler(db *store.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		sec, err := db.GetSectionByID(r.Context(), id)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if sec == nil {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}

		var req struct {
			DisplayName         *string          `json:"display_name,omitempty"`
			Enabled             *bool            `json:"enabled,omitempty"`
			SortOrder           *int             `json:"sort_order,omitempty"`
			MaxBriefingArticles *int             `json:"max_briefing_articles,omitempty"`
			SeedKeywords        *[]string        `json:"seed_keywords,omitempty"`
			Config              *json.RawMessage `json:"config,omitempty"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid request body", http.StatusBadRequest)
			return
		}

		if req.DisplayName == nil && req.Enabled == nil && req.SortOrder == nil && req.MaxBriefingArticles == nil && req.SeedKeywords == nil && req.Config == nil {
			http.Error(w, "empty patch body", http.StatusBadRequest)
			return
		}

		if req.DisplayName != nil {
			sec.DisplayName = strings.TrimSpace(*req.DisplayName)
		}
		if req.Enabled != nil {
			sec.Enabled = *req.Enabled
		}
		if req.SortOrder != nil {
			sec.SortOrder = *req.SortOrder
		}
		if req.MaxBriefingArticles != nil && *req.MaxBriefingArticles > 0 {
			sec.MaxBriefingArticles = *req.MaxBriefingArticles
		}
		if req.SeedKeywords != nil {
			sec.SeedKeywords = *req.SeedKeywords
		}
		if req.Config != nil {
			sec.Config = *req.Config
		}

		if err := db.UpdateSection(r.Context(), sec); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		respondJSON(w, sec)
	}
}

func reorderSectionsHandler(db *store.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			SectionIDs []string `json:"section_ids"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid request body", http.StatusBadRequest)
			return
		}
		if len(req.SectionIDs) == 0 {
			http.Error(w, "section_ids are required", http.StatusBadRequest)
			return
		}
		if err := db.ReorderSections(r.Context(), req.SectionIDs); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		respondJSON(w, map[string]any{"ok": true})
	}
}

func latestBriefingHandler(db *store.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		briefing, err := db.GetLatestBriefing(r.Context())
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if briefing == nil {
			http.Error(w, "no briefings generated yet", http.StatusNotFound)
			return
		}

		resp, err := buildBriefingResponse(r.Context(), db, briefing)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		respondJSON(w, resp)
	}
}

func listBriefingsHandler(db *store.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		briefings, err := db.ListBriefings(r.Context(), 20, 0)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		out := make([]briefingListItem, 0, len(briefings))
		for _, b := range briefings {
			out = append(out, briefingListItem{
				ID:          b.ID,
				GeneratedAt: b.GeneratedAt,
				Metadata:    b.Metadata,
			})
		}

		respondJSON(w, out)
	}
}

func getBriefingHandler(db *store.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		briefing, err := db.GetBriefingByID(r.Context(), id)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if briefing == nil {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}

		resp, err := buildBriefingResponse(r.Context(), db, briefing)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		respondJSON(w, resp)
	}
}

func buildBriefingResponse(ctx context.Context, db *store.Store, b *models.Briefing) (*briefingResponse, error) {
	articles, err := db.ListArticlesWithRelationsByIDs(ctx, b.ArticleIDs)
	if err != nil {
		return nil, err
	}

	out := make([]articleResponse, 0, len(articles))
	for _, article := range articles {
		out = append(out, mapArticleResponse(article))
	}

	return &briefingResponse{
		ID:          b.ID,
		GeneratedAt: b.GeneratedAt,
		Content:     b.Content,
		ArticleIDs:  b.ArticleIDs,
		Metadata:    b.Metadata,
		Articles:    out,
	}, nil
}

func createFeedbackHandler(db *store.Store, recalc *profile.Recalculator, cfg *config.Config) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			ArticleID string `json:"article_id"`
			Action    string `json:"action"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid request body", http.StatusBadRequest)
			return
		}

		req.ArticleID = strings.TrimSpace(req.ArticleID)
		req.Action = strings.TrimSpace(strings.ToLower(req.Action))
		if req.ArticleID == "" || !validFeedbackAction(req.Action) {
			http.Error(w, "article_id and action (like|dislike|save) are required", http.StatusBadRequest)
			return
		}

		article, err := db.GetArticleByID(r.Context(), req.ArticleID)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if article == nil {
			http.Error(w, "article not found", http.StatusNotFound)
			return
		}

		fb := &models.Feedback{
			ArticleID: req.ArticleID,
			Action:    req.Action,
		}
		if err := db.CreateFeedback(r.Context(), fb); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		recalculated := false
		if shouldRecalculateAfterFeedback(cfg, req.Action) && article.SectionID != nil {
			if err := recalc.RecalculateSection(r.Context(), *article.SectionID); err != nil {
				log.WithFields(log.Fields{
					"section_id": *article.SectionID,
					"action":     req.Action,
					"article_id": req.ArticleID,
				}).WithError(err).Warn("Section profile recalculation failed")
			} else {
				recalculated = true
			}
		}

		respondJSONWithStatus(w, http.StatusCreated, map[string]any{
			"feedback":     fb,
			"recalculated": recalculated,
		})
	}
}

func deleteFeedbackHandler(db *store.Store, recalc *profile.Recalculator, cfg *config.Config) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		deleted, err := db.DeleteFeedbackByID(r.Context(), id)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if deleted == nil {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}

		recalculated := false
		if shouldRecalculateAfterFeedback(cfg, deleted.Action) {
			article, err := db.GetArticleByID(r.Context(), deleted.ArticleID)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			if article != nil && article.SectionID != nil {
				if err := recalc.RecalculateSection(r.Context(), *article.SectionID); err != nil {
					log.WithFields(log.Fields{
						"section_id":  *article.SectionID,
						"action":      deleted.Action,
						"feedback_id": deleted.ID,
					}).WithError(err).Warn("Section profile recalculation failed after feedback delete")
				} else {
					recalculated = true
				}
			}
		}

		respondJSON(w, map[string]any{
			"feedback":     deleted,
			"recalculated": recalculated,
		})
	}
}

func feedbackStatsHandler(db *store.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		sections, err := db.ListSections(r.Context())
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		stats := make(map[string]map[string]int, len(sections))
		for _, sec := range sections {
			likes, dislikes, err := db.CountFeedbackBySection(r.Context(), sec.ID)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			stats[sec.Name] = map[string]int{
				"likes":    likes,
				"dislikes": dislikes,
			}
		}

		respondJSON(w, stats)
	}
}

func shouldRecalculateAfterFeedback(cfg *config.Config, action string) bool {
	if cfg.ProfileRecalcTrigger != "immediate" {
		return false
	}
	return action == models.ActionLike || action == models.ActionDislike
}

func validFeedbackAction(action string) bool {
	switch action {
	case models.ActionLike, models.ActionDislike, models.ActionSave:
		return true
	default:
		return false
	}
}

func parsePositiveInt(raw string, fallback int) int {
	if strings.TrimSpace(raw) == "" {
		return fallback
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n <= 0 {
		return fallback
	}
	return n
}

func parseBool(raw string) bool {
	b, err := strconv.ParseBool(strings.TrimSpace(raw))
	if err != nil {
		return false
	}
	return b
}

func parseISO8601(raw string) (time.Time, error) {
	layouts := []string{
		time.RFC3339,
		"2006-01-02",
	}
	for _, layout := range layouts {
		if t, err := time.Parse(layout, raw); err == nil {
			return t, nil
		}
	}
	return time.Time{}, fmt.Errorf("invalid datetime %q", raw)
}

func respondJSON(w http.ResponseWriter, data interface{}) {
	respondJSONWithStatus(w, http.StatusOK, data)
}

func respondJSONWithStatus(w http.ResponseWriter, statusCode int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	_ = json.NewEncoder(w).Encode(data)
}
