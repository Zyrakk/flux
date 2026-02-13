package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	log "github.com/sirupsen/logrus"
	"github.com/zyrak/flux/internal/config"
	"github.com/zyrak/flux/internal/models"
	"github.com/zyrak/flux/internal/store"
)

func main() {
	cfg := config.Load()
	setupLogging(cfg.LogLevel)

	log.Info("Starting Flux API server")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Initialize store
	db, err := store.New(ctx, cfg.DatabaseURL)
	if err != nil {
		log.WithError(err).Fatal("Failed to connect to database")
	}
	defer db.Close()

	// Run migrations
	migrationsDir := os.Getenv("MIGRATIONS_DIR")
	if migrationsDir == "" {
		migrationsDir = "migrations"
	}
	if err := db.RunMigrations(ctx, migrationsDir); err != nil {
		log.WithError(err).Fatal("Failed to run migrations")
	}

	// Setup router
	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	r.Use(middleware.Timeout(30 * time.Second))

	// Health check
	r.Get("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	})

	// API routes
	r.Route("/api", func(r chi.Router) {
		// Articles
		r.Get("/articles", listArticlesHandler(db))
		r.Get("/articles/{id}", getArticleHandler(db))

		// Sources
		r.Get("/sources", listSourcesHandler(db))
		r.Post("/sources", createSourceHandler(db))
		r.Patch("/sources/{id}", updateSourceHandler(db))

		// Sections
		r.Get("/sections", listSectionsHandler(db))

		// Briefings
		r.Get("/briefings/latest", latestBriefingHandler(db))
		r.Get("/briefings", listBriefingsHandler(db))

		// Feedback
		r.Post("/feedback", createFeedbackHandler(db))
	})

	// Start server
	addr := fmt.Sprintf(":%d", cfg.APIPort)
	srv := &http.Server{
		Addr:    addr,
		Handler: r,
	}

	go func() {
		log.WithField("addr", addr).Info("API server listening")
		if err := srv.ListenAndServe(); err != http.ErrServerClosed {
			log.WithError(err).Fatal("Server failed")
		}
	}()

	// Graceful shutdown
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

// --- Handlers ---

func listArticlesHandler(db *store.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		filter := models.ArticleFilter{Limit: 50}
		if s := r.URL.Query().Get("section_id"); s != "" {
			filter.SectionID = &s
		}
		if s := r.URL.Query().Get("source_type"); s != "" {
			filter.SourceType = &s
		}
		if s := r.URL.Query().Get("status"); s != "" {
			filter.Status = &s
		}

		articles, err := db.ListArticles(r.Context(), filter)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		respondJSON(w, articles)
	}
}

func getArticleHandler(db *store.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		article, err := db.GetArticleByID(r.Context(), id)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if article == nil {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		respondJSON(w, article)
	}
}

func listSourcesHandler(db *store.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		sources, err := db.ListSources(r.Context(), models.SourceFilter{})
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		respondJSON(w, sources)
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
		w.WriteHeader(http.StatusCreated)
		respondJSON(w, src)
	}
}

func updateSourceHandler(db *store.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		var req struct {
			Name    *string          `json:"name,omitempty"`
			Config  *json.RawMessage `json:"config,omitempty"`
			Enabled *bool            `json:"enabled,omitempty"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid request body", http.StatusBadRequest)
			return
		}

		src := &models.Source{ID: id}
		if req.Name != nil {
			src.Name = *req.Name
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
		w.WriteHeader(http.StatusOK)
		respondJSON(w, src)
	}
}

func listSectionsHandler(db *store.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		sections, err := db.ListSections(r.Context())
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
		w.WriteHeader(http.StatusCreated)
		respondJSON(w, fb)
	}
}

func respondJSON(w http.ResponseWriter, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(data)
}
