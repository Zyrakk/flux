package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	log "github.com/sirupsen/logrus"
	"github.com/zyrak/flux/internal/config"
)

func main() {
	cfg := config.Load()
	setupLogging(cfg.LogLevel)

	log.Info("Starting Flux RSS worker")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// TODO (Phase 1): Initialize store, queue, dedup, ratelimit
	// TODO (Phase 1): Load enabled RSS sources from DB
	// TODO (Phase 1): For each feed:
	//   - Parse with gofeed
	//   - Check dedup (Redis SETNX)
	//   - Download full content with go-readability (through rate limiter)
	//   - Publish to NATS "articles.new"
	//   - Insert to PostgreSQL with status "pending"
	_ = ctx
	_ = cfg

	log.Info("RSS worker initialized (Phase 1 placeholder)")

	// Wait for shutdown signal
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Info("RSS worker shutting down")
}

func setupLogging(level string) {
	log.SetFormatter(&log.JSONFormatter{})
	lvl, err := log.ParseLevel(level)
	if err != nil {
		lvl = log.InfoLevel
	}
	log.SetLevel(lvl)
}
