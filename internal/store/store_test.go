package store

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"
)

func setupStoreTestDB(t *testing.T) (context.Context, *Store) {
	t.Helper()

	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		dsn = os.Getenv("DATABASE_URL")
	}
	if dsn == "" {
		t.Skip("set TEST_DATABASE_URL or DATABASE_URL to run store DB tests")
	}

	ctx := context.Background()

	cfg, err := pgxpool.ParseConfig(dsn)
	require.NoError(t, err)

	schema := fmt.Sprintf("store_test_%d", time.Now().UnixNano())
	if cfg.ConnConfig.RuntimeParams == nil {
		cfg.ConnConfig.RuntimeParams = map[string]string{}
	}
	cfg.ConnConfig.RuntimeParams["search_path"] = schema

	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	require.NoError(t, err)

	_, err = pool.Exec(ctx, fmt.Sprintf(`CREATE SCHEMA "%s"`, schema))
	require.NoError(t, err)

	_, err = pool.Exec(ctx, `
		CREATE TABLE articles (
			id UUID PRIMARY KEY,
			source_type TEXT NOT NULL,
			source_id TEXT NOT NULL,
			section_id UUID,
			url TEXT NOT NULL,
			title TEXT NOT NULL,
			content TEXT,
			summary TEXT,
			author TEXT,
			published_at TIMESTAMPTZ,
			ingested_at TIMESTAMPTZ NOT NULL,
			processed_at TIMESTAMPTZ,
			relevance_score FLOAT,
			categories TEXT[],
			status TEXT NOT NULL,
			metadata JSONB,
			UNIQUE(source_type, source_id)
		)
	`)
	require.NoError(t, err)

	s := &Store{pool: pool}

	t.Cleanup(func() {
		_, _ = pool.Exec(ctx, fmt.Sprintf(`DROP SCHEMA IF EXISTS "%s" CASCADE`, schema))
		pool.Close()
	})

	return ctx, s
}
