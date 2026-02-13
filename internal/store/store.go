package store

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
	log "github.com/sirupsen/logrus"
)

// Store provides access to the PostgreSQL database.
type Store struct {
	pool *pgxpool.Pool
}

// New creates a new Store with a connection pool.
func New(ctx context.Context, connString string) (*Store, error) {
	config, err := pgxpool.ParseConfig(connString)
	if err != nil {
		return nil, fmt.Errorf("parsing connection string: %w", err)
	}

	config.MaxConns = 20
	config.MinConns = 2

	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		return nil, fmt.Errorf("creating connection pool: %w", err)
	}

	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("pinging database: %w", err)
	}

	log.Info("Connected to PostgreSQL")
	return &Store{pool: pool}, nil
}

// RunMigrations executes all *.up.sql files from the given directory in order.
func (s *Store) RunMigrations(ctx context.Context, migrationsDir string) error {
	// Create migrations tracking table
	_, err := s.pool.Exec(ctx, `
		CREATE TABLE IF NOT EXISTS schema_migrations (
			version TEXT PRIMARY KEY,
			applied_at TIMESTAMPTZ DEFAULT NOW()
		)`)
	if err != nil {
		return fmt.Errorf("creating schema_migrations table: %w", err)
	}

	entries, err := os.ReadDir(migrationsDir)
	if err != nil {
		return fmt.Errorf("reading migrations dir %s: %w", migrationsDir, err)
	}

	var upFiles []string
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".up.sql") {
			upFiles = append(upFiles, e.Name())
		}
	}
	sort.Strings(upFiles)

	for _, fname := range upFiles {
		version := strings.TrimSuffix(fname, ".up.sql")

		// Check if already applied
		var exists bool
		err := s.pool.QueryRow(ctx,
			"SELECT EXISTS(SELECT 1 FROM schema_migrations WHERE version = $1)", version).Scan(&exists)
		if err != nil {
			return fmt.Errorf("checking migration %s: %w", version, err)
		}
		if exists {
			log.WithField("version", version).Debug("Migration already applied, skipping")
			continue
		}

		sql, err := os.ReadFile(filepath.Join(migrationsDir, fname))
		if err != nil {
			return fmt.Errorf("reading migration %s: %w", fname, err)
		}

		if _, err := s.pool.Exec(ctx, string(sql)); err != nil {
			return fmt.Errorf("executing migration %s: %w", fname, err)
		}

		if _, err := s.pool.Exec(ctx,
			"INSERT INTO schema_migrations (version) VALUES ($1)", version); err != nil {
			return fmt.Errorf("recording migration %s: %w", fname, err)
		}

		log.WithField("version", version).Info("Applied migration")
	}

	return nil
}

// Close shuts down the connection pool.
func (s *Store) Close() {
	s.pool.Close()
}

// Pool returns the underlying pgxpool for advanced usage.
func (s *Store) Pool() *pgxpool.Pool {
	return s.pool
}
