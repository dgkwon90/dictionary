// Package db owns database connections and schema migrations.
// It isolates SQLite infrastructure from the domain layer.
package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"time"

	_ "modernc.org/sqlite"
)

const pingTimeout = 5 * time.Second

func Open(dbPath string) (*sql.DB, error) {
	dir := filepath.Dir(dbPath)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("create database directory %q: %w", dir, err)
	}

	query := url.Values{}
	query.Add("_pragma", "busy_timeout(5000)")
	query.Add("_pragma", "foreign_keys(1)")
	query.Add("_pragma", "journal_mode(WAL)")
	dsn := (&url.URL{
		Scheme:   "file",
		Path:     dbPath,
		RawQuery: query.Encode(),
	}).String()

	database, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open SQLite database: %w", err)
	}
	// SQLite has a single writer; keep one pooled connection to serialize writes
	// once concurrent writes appear in backlog #3. See ADR-0007.
	database.SetMaxOpenConns(1)
	database.SetMaxIdleConns(1)

	ctx, cancel := context.WithTimeout(context.Background(), pingTimeout)
	defer cancel()
	if err := database.PingContext(ctx); err != nil {
		closeErr := database.Close()
		return nil, errors.Join(fmt.Errorf("ping SQLite database: %w", err), closeErr)
	}

	return database, nil
}
