package db

import (
	"context"
	"database/sql"
	"io"
	"log/slog"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestMigrateIsIdempotentAndCreatesAllTables(t *testing.T) {
	database := openTestDB(t)
	ctx := context.Background()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	if err := Migrate(ctx, database, logger); err != nil {
		t.Fatalf("first Migrate() error = %v", err)
	}
	if err := Migrate(ctx, database, logger); err != nil {
		t.Fatalf("second Migrate() error = %v", err)
	}

	wantTables := []string{
		"app_settings", "captures", "lookup_jobs", "explanations", "knowledge_items",
		"capture_items", "learner_items", "review_cards", "review_logs", "reminders", "sync_outbox",
	}
	for _, table := range wantTables {
		var count int
		err := database.QueryRowContext(
			ctx,
			"SELECT count(*) FROM sqlite_master WHERE type = 'table' AND name = ?", table,
		).Scan(&count)
		if err != nil {
			t.Fatalf("query table %q: %v", table, err)
		}
		if count != 1 {
			t.Errorf("table %q count = %d, want 1", table, count)
		}
	}

	var migrationCount int
	if err := database.QueryRowContext(ctx, "SELECT count(*) FROM schema_migrations").Scan(&migrationCount); err != nil {
		t.Fatalf("count migrations: %v", err)
	}
	if migrationCount != 1 {
		t.Errorf("migration count = %d, want 1", migrationCount)
	}
}

func TestMigrateDetectsChecksumTampering(t *testing.T) {
	database := openMigratedTestDB(t)
	if _, err := database.Exec("UPDATE schema_migrations SET checksum = 'bogus' WHERE version = 1"); err != nil {
		t.Fatalf("tamper checksum: %v", err)
	}

	err := Migrate(context.Background(), database, slog.Default())
	if err == nil || !strings.Contains(err.Error(), "applied migration 0001 modified") {
		t.Fatalf("Migrate() error = %v, want modified migration error", err)
	}
}

func TestMigrateRejectsNewerSchemaVersion(t *testing.T) {
	database := openMigratedTestDB(t)
	if _, err := database.Exec(
		"INSERT INTO schema_migrations(version, checksum, applied_at) VALUES (9999, 'future', ?)",
		time.Now().UTC(),
	); err != nil {
		t.Fatalf("insert future version: %v", err)
	}

	err := Migrate(context.Background(), database, slog.Default())
	if err == nil || !strings.Contains(err.Error(), "9999 is newer than this binary") {
		t.Fatalf("Migrate() error = %v, want newer-schema error", err)
	}
}

// 동시 기동 방어(ADR-0007): 같은 DB에 여러 커넥션이 동시에 Migrate해도
// BEGIN IMMEDIATE 직렬화로 정확히 한 번만 적용되어야 한다.
// Open은 순차로 한다 — 최초 WAL 전환은 배타 락이 필요해 동시 첫 Open은 별개 문제다.
func TestMigrateConcurrentStartup(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")
	const workers = 4

	databases := make([]*sql.DB, workers)
	for i := range workers {
		database, err := Open(dbPath)
		if err != nil {
			t.Fatalf("Open() error = %v", err)
		}
		t.Cleanup(func() {
			if err := database.Close(); err != nil {
				t.Errorf("Close() error = %v", err)
			}
		})
		databases[i] = database
	}

	errs := make(chan error, workers)
	for i := range workers {
		go func(database *sql.DB) {
			errs <- Migrate(context.Background(), database, slog.New(slog.NewTextHandler(io.Discard, nil)))
		}(databases[i])
	}
	for range workers {
		if err := <-errs; err != nil {
			t.Fatalf("concurrent Migrate() error = %v", err)
		}
	}

	var count int
	if err := databases[0].QueryRow("SELECT count(*) FROM schema_migrations").Scan(&count); err != nil {
		t.Fatalf("count migrations: %v", err)
	}
	if count != 1 {
		t.Fatalf("migration count = %d, want 1", count)
	}
}

func TestOpenEnforcesForeignKeys(t *testing.T) {
	database := openMigratedTestDB(t)
	_, err := database.Exec(`INSERT INTO lookup_jobs
(id, capture_id, status, created_at) VALUES (?, ?, ?, ?)`, "job-1", "missing", "queued", time.Now().UTC())
	if err == nil {
		t.Fatal("foreign key violation was accepted")
	}
}

func TestOpenUsesWAL(t *testing.T) {
	database := openTestDB(t)
	var mode string
	if err := database.QueryRow("PRAGMA journal_mode").Scan(&mode); err != nil {
		t.Fatalf("query journal mode: %v", err)
	}
	if mode != "wal" {
		t.Fatalf("journal mode = %q, want wal", mode)
	}
}

func TestCapturesInboxStatusCheck(t *testing.T) {
	database := openMigratedTestDB(t)
	now := time.Now().UTC()
	insert := `INSERT INTO captures
(id, selected_text, input_mode, text_hash, created_at, inbox_status) VALUES (?, ?, ?, ?, ?, ?)`
	if _, err := database.Exec(insert, "bad", "text", "manual", "hash-bad", now, "bogus"); err == nil {
		t.Fatal("invalid inbox_status was accepted")
	}
	if _, err := database.Exec(insert, "good", "text", "manual", "hash-good", now, "saved"); err != nil {
		t.Fatalf("valid inbox_status was rejected: %v", err)
	}
}

func openTestDB(t *testing.T) *sql.DB {
	t.Helper()
	database, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	t.Cleanup(func() {
		if err := database.Close(); err != nil {
			t.Errorf("Close() error = %v", err)
		}
	})
	return database
}

func openMigratedTestDB(t *testing.T) *sql.DB {
	t.Helper()
	database := openTestDB(t)
	if err := Migrate(context.Background(), database, slog.Default()); err != nil {
		t.Fatalf("Migrate() error = %v", err)
	}
	return database
}
