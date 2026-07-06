package db

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestOpenCreatesParentAndFile(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "missing parent", "special # name.db")
	database, err := Open(dbPath)
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	if err := database.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	if _, err := os.Stat(dbPath); err != nil {
		t.Fatalf("database file was not created: %v", err)
	}
}

func TestOpenSupportsFTS5(t *testing.T) {
	database := openTestDB(t)
	ctx := context.Background()
	if _, err := database.ExecContext(ctx, "CREATE VIRTUAL TABLE t USING fts5(x)"); err != nil {
		t.Fatalf("create FTS5 table: %v", err)
	}
	if _, err := database.ExecContext(ctx, "INSERT INTO t(x) VALUES (?)", "hello dictionary"); err != nil {
		t.Fatalf("insert FTS5 row: %v", err)
	}
	var count int
	if err := database.QueryRowContext(ctx, "SELECT count(*) FROM t WHERE t MATCH ?", "dictionary").Scan(&count); err != nil {
		t.Fatalf("search FTS5 table: %v", err)
	}
	if count != 1 {
		t.Fatalf("FTS5 match count = %d, want 1", count)
	}
}
