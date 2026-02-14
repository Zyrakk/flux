package main

import (
	"context"
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
	"github.com/nats-io/nats.go"
	"github.com/redis/go-redis/v9"
	log "github.com/sirupsen/logrus"
	"github.com/zyrak/flux/internal/config"
	"github.com/zyrak/flux/internal/models"
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

	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	r.Use(middleware.Timeout(30 * time.Second))

	r.Get("/healthz", healthzHandler(db, nc, rdb))

	r.Route("/api", func(r chi.Router) {
		r.Get("/articles", listArticlesHandler(db))
		r.Get("/articles/{id}", getArticleHandler(db))

		r.Get("/sources", listSourcesHandler(db))
		r.Post("/sources", createSourceHandler(db))
		r.Patch("/sources/{id}", updateSourceHandler(db))

		r.Get("/sections", listSectionsHandler(db))

		r.Get("/briefings/latest", latestBriefingHandler(db))
		r.Get("/briefings", listBriefingsHandler(db))

		r.Post("/feedback", createFeedbackHandler(db))
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
		if sourceType := strings.TrimSpace(r.URL.Query().Get("source_type")); sourceType != "" {
			filter.SourceType = &sourceType
		}
		if status := strings.TrimSpace(r.URL.Query().Get("status")); status != "" {
			filter.Status = &status
		}

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
	}
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
		respondJSON(w, briefing)
	}
}

func listBriefingsHandler(db *store.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		briefings, err := db.ListBriefings(r.Context(), 20, 0)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		respondJSON(w, briefings)
	}
}

func createFeedbackHandler(db *store.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var fb models.Feedback
		if err := json.NewDecoder(r.Body).Decode(&fb); err != nil {
			http.Error(w, "invalid request body", http.StatusBadRequest)
			return
		}
		if err := db.CreateFeedback(r.Context(), &fb); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		respondJSONWithStatus(w, http.StatusCreated, fb)
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
