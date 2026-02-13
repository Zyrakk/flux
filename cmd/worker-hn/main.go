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

	log.Info("Starting Flux Hacker News worker")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// TODO (Phase 1): Initialize store, queue, dedup, ratelimit
	// TODO (Phase 1): Fetch from HN Firebase API:
	//   - GET /v0/topstories, /v0/beststories, /v0/newstories
	//   - Filter by score > threshold (default 10)
	//   - For each story: get title, URL, score, comments
	//   - If external URL: download content with go-readability
	//   - If Ask HN / Show HN: store post text
	//   - Dedup by URL, publish to NATS "articles.new"
	//   - Store metadata: {"hn_score": N, "hn_comments": N, "hn_id": N}
	_ = ctx
	_ = cfg

	log.Info("HN worker initialized (Phase 1 placeholder)")

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Info("HN worker shutting down")
}

func setupLogging(level string) {
	log.SetFormatter(&log.JSONFormatter{})
	lvl, err := log.ParseLevel(level)
	if err != nil {
		lvl = log.InfoLevel
	}
	log.SetLevel(lvl)
}
