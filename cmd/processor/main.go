package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	log "github.com/sirupsen/logrus"
	"github.com/zyrak/flux/internal/config"
	"github.com/zyrak/flux/internal/llm"
)

func main() {
	cfg := config.Load()
	setupLogging(cfg.LogLevel)

	log.Info("Starting Flux processor")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Initialize LLM analyzer
	analyzer, err := llm.NewAnalyzer(cfg.LLMProvider, cfg.LLMEndpoint, cfg.LLMModel, cfg.LLMAPIKey)
	if err != nil {
		log.WithError(err).Fatal("Failed to initialize LLM analyzer")
	}
	log.WithField("provider", analyzer.Provider()).Info("LLM analyzer ready")

	// TODO (Phase 2): Initialize store, embeddings client, queue
	// TODO (Phase 2): Subscribe to NATS "articles.new":
	//   1. Compute embedding via local embeddings service
	//   2. Calculate relevance score per section (cosine similarity vs section profile)
	//   3. Assign to highest-scoring section
	//   4. If score < threshold → mark as "archived" (skip LLM)
	//   5. If score >= threshold → keep as "pending" for LLM processing
	//
	// TODO (Phase 2): CronJob pipeline (03:00 daily):
	//   Phase 1 — Classify batch of pending articles via analyzer.Classify()
	//   Phase 2 — Summarize relevant articles via analyzer.Summarize()
	//   Phase 3 — Generate briefing via analyzer.GenerateBriefing()
	_ = ctx
	_ = cfg
	_ = analyzer

	log.Info("Processor initialized (Phase 2 placeholder)")

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Info("Processor shutting down")
}

func setupLogging(level string) {
	log.SetFormatter(&log.JSONFormatter{})
	lvl, err := log.ParseLevel(level)
	if err != nil {
		lvl = log.InfoLevel
	}
	log.SetLevel(lvl)
}
