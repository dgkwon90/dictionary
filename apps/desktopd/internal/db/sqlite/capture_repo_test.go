package sqlite

import (
	"context"
	"database/sql"
	"log/slog"
	"path/filepath"
	"testing"
	"time"

	"neulsang/desktopd/internal/db"
	"neulsang/desktopd/internal/domain/capture"
)

func TestCaptureRepositorySaveNew(t *testing.T) {
	database := openMigratedDB(t)
	repo := NewCaptureRepository(database)
	createdAt := time.Date(2026, 7, 7, 1, 2, 3, 0, time.UTC)
	c := capture.Capture{
		ID:           "capture-1",
		SelectedText: "hello",
		InputMode:    "manual",
		TextHash:     "same-hash",
		InboxStatus:  "new",
		CreatedAt:    createdAt,
	}
	j := capture.LookupJob{ID: "job-1", CaptureID: c.ID, Status: "queued", CreatedAt: createdAt}
	e := capture.OutboxEvent{EventID: "event-1", AggregateType: "capture", AggregateID: c.ID, EventType: "capture_created", PayloadJSON: `{"id":"capture-1"}`, CreatedAt: createdAt}

	if err := repo.SaveNew(context.Background(), c, j, e); err != nil {
		t.Fatalf("SaveNew() error = %v", err)
	}

	var captureCount int
	var inboxStatus string
	var sourceApp sql.NullString
	if err := database.QueryRowContext(context.Background(), "SELECT count(*), inbox_status, source_app FROM captures").Scan(&captureCount, &inboxStatus, &sourceApp); err != nil {
		t.Fatalf("query captures: %v", err)
	}
	if captureCount != 1 || inboxStatus != "new" || sourceApp.Valid {
		t.Fatalf("capture row count=%d inbox=%q source_app=%#v", captureCount, inboxStatus, sourceApp)
	}
	var lookupCount int
	var lookupCaptureID string
	var lookupStatus string
	if err := database.QueryRowContext(context.Background(), "SELECT count(*), capture_id, status FROM lookup_jobs").Scan(&lookupCount, &lookupCaptureID, &lookupStatus); err != nil {
		t.Fatalf("query lookup_jobs: %v", err)
	}
	if lookupCount != 1 || lookupCaptureID != c.ID || lookupStatus != "queued" {
		t.Fatalf("lookup row count=%d capture_id=%q status=%q", lookupCount, lookupCaptureID, lookupStatus)
	}
	var outboxCount int
	var outboxAggregateID string
	var outboxEventType string
	if err := database.QueryRowContext(context.Background(), "SELECT count(*), aggregate_id, event_type FROM sync_outbox").Scan(&outboxCount, &outboxAggregateID, &outboxEventType); err != nil {
		t.Fatalf("query sync_outbox: %v", err)
	}
	if outboxCount != 1 || outboxAggregateID != c.ID || outboxEventType != "capture_created" {
		t.Fatalf("outbox row count=%d aggregate_id=%q event_type=%q", outboxCount, outboxAggregateID, outboxEventType)
	}
}

func TestCaptureRepositoryAllowsDuplicateTextHash(t *testing.T) {
	database := openMigratedDB(t)
	repo := NewCaptureRepository(database)
	createdAt := time.Date(2026, 7, 7, 1, 2, 3, 0, time.UTC)

	for _, suffix := range []string{"1", "2"} {
		c := capture.Capture{ID: "capture-" + suffix, SelectedText: "hello", InputMode: "manual", TextHash: "same-hash", InboxStatus: "new", CreatedAt: createdAt}
		j := capture.LookupJob{ID: "job-" + suffix, CaptureID: c.ID, Status: "queued", CreatedAt: createdAt}
		e := capture.OutboxEvent{EventID: "event-" + suffix, AggregateType: "capture", AggregateID: c.ID, EventType: "capture_created", PayloadJSON: `{}`, CreatedAt: createdAt}
		if err := repo.SaveNew(context.Background(), c, j, e); err != nil {
			t.Fatalf("SaveNew(%s) error = %v", suffix, err)
		}
	}

	var count int
	if err := database.QueryRowContext(context.Background(), "SELECT count(*) FROM captures WHERE text_hash = ?", "same-hash").Scan(&count); err != nil {
		t.Fatalf("query captures: %v", err)
	}
	if count != 2 {
		t.Fatalf("duplicate text_hash count = %d, want 2", count)
	}
}

func openMigratedDB(t *testing.T) *sql.DB {
	t.Helper()
	database, err := db.Open(filepath.Join(t.TempDir(), "neulsang.db"))
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	t.Cleanup(func() {
		if err := database.Close(); err != nil {
			t.Fatalf("Close() error = %v", err)
		}
	})
	if err := db.Migrate(context.Background(), database, slog.Default()); err != nil {
		t.Fatalf("Migrate() error = %v", err)
	}
	return database
}
