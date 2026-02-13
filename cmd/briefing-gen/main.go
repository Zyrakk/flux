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

	log.Info("Starting Flux briefing generator")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Initialize LLM analyzer
	analyzer, err := llm.NewAnalyzer(cfg.LLMProvider, cfg.LLMEndpoint, cfg.LLMModel, cfg.LLMAPIKey)
	if err != nil {
		log.WithError(err).Fatal("Failed to initialize LLM analyzer")
	}
	log.WithField("provider", analyzer.Provider()).Info("LLM analyzer ready")

	// TODO (Phase 2): Initialize store
	// TODO (Phase 2): Implement the briefing generation pipeline:
	//   1. Load all enabled sections
	//   2. For each section: get top N articles by relevance_score (status = "processed")
	//   3. Group into BriefingSections
	//   4. Call analyzer.GenerateBriefing(sections)
	//   5. Store briefing in DB
	//   6. Mark articles as "briefed"
	//
	// This runs as a k8s CronJob (daily at 03:00) or triggered via NATS "briefing.generate"
	_ = ctx
	_ = cfg
	_ = analyzer

	log.Info("Briefing generator completed (Phase 2 placeholder)")

	// For CronJob mode, exit after completion
	// For daemon mode, wait for signal
	if os.Getenv("BRIEFING_MODE") == "daemon" {
		quit := make(chan os.Signal, 1)
		signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
		<-quit
	}

	log.Info("Briefing generator shutting down")
}

func setupLogging(level string) {
	log.SetFormatter(&log.JSONFormatter{})
	lvl, err := log.ParseLevel(level)
	if err != nil {
		lvl = log.InfoLevel
	}
	log.SetLevel(lvl)
}
