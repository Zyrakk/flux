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

	log.Info("Starting Flux Reddit worker")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// TODO (Phase 4): Initialize store, queue, dedup, ratelimit
	// TODO (Phase 4): Reddit OAuth authentication
	// TODO (Phase 4): For each configured subreddit:
	//   - Fetch hot posts via .json endpoint
	//   - Filter by score threshold (configurable per subreddit)
	//   - Link posts: download external article with go-readability
	//   - Self posts: store post body as content
	//   - Store metadata: {"reddit_score": N, "reddit_comments": N, "subreddit": "name"}
	//   - Rate limit: 60 req/min (Reddit OAuth limit)
	_ = ctx
	_ = cfg

	log.Info("Reddit worker initialized (Phase 4 placeholder)")

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Info("Reddit worker shutting down")
}

func setupLogging(level string) {
	log.SetFormatter(&log.JSONFormatter{})
	lvl, err := log.ParseLevel(level)
	if err != nil {
		lvl = log.InfoLevel
	}
	log.SetLevel(lvl)
}
