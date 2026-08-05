package db

import (
	"context"
	"database/sql"
	"io"
	"log/slog"
	"path/filepath"
	"strconv"
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
		"capture_items", "learner_items", "review_cards", "review_logs", "sync_outbox",
		"review_card_candidates", "suggest_cache", "notifications",
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

	migrations, err := loadMigrations()
	if err != nil {
		t.Fatalf("loadMigrations() error = %v", err)
	}
	var migrationCount int
	if err := database.QueryRowContext(ctx, "SELECT count(*) FROM schema_migrations").Scan(&migrationCount); err != nil {
		t.Fatalf("count migrations: %v", err)
	}
	if migrationCount != len(migrations) {
		t.Errorf("migration count = %d, want %d", migrationCount, len(migrations))
	}
}

// 0002 exists so the frontend can stop understanding the old route name. If a stored
// row kept saying "Inbox", clicking that notification in the in-app history would go
// nowhere once the compatibility map is deleted.
func TestMigrateRewritesStoredInboxRoute(t *testing.T) {
	database := openMigratedTestDB(t)
	ctx := context.Background()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	// Recreate the pre-0002 world: a notification written under the old name, and the
	// migration not yet recorded as applied.
	now := time.Now().UTC()
	if _, err := database.ExecContext(ctx,
		`INSERT INTO notifications(id, kind, dedup_key, title, body, route, payload_id, created_at)
VALUES ('n-old', 'result_ready', 'cap-old', 't', 'b', 'Inbox', 'cap-old', ?)`,
		now,
	); err != nil {
		t.Fatalf("seed old notification: %v", err)
	}
	// A review reminder, which this migration must not touch. Without it here, an
	// UPDATE that forgot its WHERE clause would still pass: every review reminder in
	// the user's history would silently start opening the search history instead of
	// the review session, and nothing would say so.
	if _, err := database.ExecContext(ctx,
		`INSERT INTO notifications(id, kind, dedup_key, title, body, route, created_at)
VALUES ('n-review', 'review_due', 'review_due:2026-08-04:evening', 't', 'b', 'Today Review', ?)`,
		now,
	); err != nil {
		t.Fatalf("seed review notification: %v", err)
	}
	if _, err := database.ExecContext(ctx, `DELETE FROM schema_migrations WHERE version = 2`); err != nil {
		t.Fatalf("unapply 0002: %v", err)
	}

	if err := Migrate(ctx, database, logger); err != nil {
		t.Fatalf("Migrate() error = %v", err)
	}

	routes := map[string]string{}
	rows, err := database.QueryContext(ctx, `SELECT id, route FROM notifications`)
	if err != nil {
		t.Fatalf("read routes: %v", err)
	}
	defer func() {
		if err := rows.Close(); err != nil {
			t.Errorf("close rows: %v", err)
		}
	}()
	for rows.Next() {
		var id, route string
		if err := rows.Scan(&id, &route); err != nil {
			t.Fatalf("scan route: %v", err)
		}
		routes[id] = route
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate routes: %v", err)
	}
	if routes["n-old"] != "Search History" {
		t.Errorf("renamed route = %q, want %q", routes["n-old"], "Search History")
	}
	if routes["n-review"] != "Today Review" {
		t.Errorf("review route = %q, want it untouched", routes["n-review"])
	}
}

func TestMigrateDetectsChecksumTampering(t *testing.T) {
	database := openMigratedTestDB(t)
	if _, err := database.Exec("UPDATE schema_migrations SET checksum = 'bogus' WHERE version = 1"); err != nil {
		t.Fatalf("tamper checksum: %v", err)
	}

	err := Migrate(context.Background(), database, slog.Default())
	if err == nil || !strings.Contains(err.Error(), "applied migration 0001 was modified") {
		t.Fatalf("Migrate() error = %v, want modified migration error", err)
	}
	// The message has to name the database file: a failed migration blocks startup,
	// and the Tauri shell only shows a generic error while this text goes to the
	// sidecar's stderr — so the path is the user's only lead.
	if !strings.Contains(err.Error(), "test.db") {
		t.Errorf("Migrate() error = %v, want it to name the database file", err)
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

	migrations, err := loadMigrations()
	if err != nil {
		t.Fatalf("loadMigrations() error = %v", err)
	}
	var count int
	if err := databases[0].QueryRow("SELECT count(*) FROM schema_migrations").Scan(&count); err != nil {
		t.Fatalf("count migrations: %v", err)
	}
	if count != len(migrations) {
		t.Fatalf("migration count = %d, want %d", count, len(migrations))
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

func TestCapturesTriageStateCheck(t *testing.T) {
	database := openMigratedTestDB(t)
	now := time.Now().UTC()
	insert := `INSERT INTO captures
(id, selected_text, input_mode, text_hash, created_at, updated_at, learn_kind, triage_state)
VALUES (?, ?, ?, ?, ?, ?, ?, ?)`

	tests := []struct {
		name       string
		learnKind  any
		state      string
		wantAccept bool
	}{
		{"unseen before AI resolves learn_kind", nil, "unseen", true},
		{"word can go straight to learning", "word", "learning", true},
		{"sentence awaiting word selection", "sentence", "needs_selection", true},
		{"discarded", "word", "discarded", true},
		{"unknown state", "word", "bogus", false},
		{"unknown learn_kind", "paragraph", "unseen", false},
		// 단어는 고를 단어가 자기 자신뿐이라 선택 단계를 거치지 않는다. 이걸 스키마가
		// 막지 않으면 UI가 빠져나올 수 없는 상태에 갇힌다.
		{"word cannot need selection", "word", "needs_selection", false},
		{"unresolved kind cannot need selection", nil, "needs_selection", false},
	}
	for index, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			id := "capture-" + strconv.Itoa(index)
			_, err := database.Exec(insert, id, "text", "manual", id, now, now, test.learnKind, test.state)
			if test.wantAccept && err != nil {
				t.Fatalf("valid row was rejected: %v", err)
			}
			if !test.wantAccept && err == nil {
				t.Fatal("invalid row was accepted")
			}
		})
	}
}

// TestReviewCardIdentityIsUnique pins the invariant that replaces the old duplicate-card
// bug: searching the same word twice used to pile up candidates, and each mark-unknown
// turned them into another copy of the same card. Identity is (owner, type, context),
// with cloze cards distinguished by the sentence they came from.
func TestReviewCardIdentityIsUnique(t *testing.T) {
	database := openMigratedTestDB(t)
	now := time.Now().UTC()
	mustExec(t, database, `INSERT INTO knowledge_items
(id, normalized_key, surface_text, learn_kind, language, first_seen_at, last_seen_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?)`, "word-1", "stale", "stale", "word", "en", now, now, now)
	mustExec(t, database, `INSERT INTO knowledge_items
(id, normalized_key, surface_text, learn_kind, language, first_seen_at, last_seen_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?)`, "sent-1", "the cache went stale", "The cache went stale.", "sentence", "en", now, now, now)

	insertCard := `INSERT INTO review_cards
(id, knowledge_item_id, context_knowledge_item_id, card_type, question, answer, state, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, 'new', ?, ?)`

	mustExec(t, database, insertCard, "card-1", "word-1", nil, "meaning", "q", "a", now, now)
	if _, err := database.Exec(insertCard, "card-2", "word-1", nil, "meaning", "q", "a", now, now); err == nil {
		t.Fatal("duplicate meaning card for the same word was accepted")
	}
	// 같은 단어라도 문맥(문장)이 다르면 별개의 cloze 카드다.
	mustExec(t, database, insertCard, "card-3", "word-1", "sent-1", "cloze", "q", "a", now, now)
	if _, err := database.Exec(insertCard, "card-4", "word-1", "sent-1", "cloze", "q", "a", now, now); err == nil {
		t.Fatal("duplicate cloze card for the same word+sentence was accepted")
	}
}

func mustExec(t *testing.T, database *sql.DB, query string, args ...any) {
	t.Helper()
	if _, err := database.Exec(query, args...); err != nil {
		t.Fatalf("exec %q: %v", query, err)
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
