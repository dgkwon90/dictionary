package sqlite

import (
	"context"
	"database/sql"
	"testing"
	"time"
)

func TestOutboxRepositoryListUnsent(t *testing.T) {
	database := openMigratedDB(t)
	repo := NewOutboxRepository(database)
	ctx := context.Background()
	base := time.Date(2026, 7, 17, 1, 0, 0, 0, time.UTC)
	sentAt := base.Add(30 * time.Second)

	seedOutboxEvent(t, database, "acked", base, nil, ptrTime(base.Add(time.Minute)))
	seedOutboxEvent(t, database, "oldest", base.Add(time.Minute), &sentAt, nil)
	seedOutboxEvent(t, database, "middle", base.Add(2*time.Minute), nil, nil)
	seedOutboxEvent(t, database, "newest", base.Add(3*time.Minute), nil, nil)

	events, err := repo.ListUnsent(ctx, 2)
	if err != nil {
		t.Fatalf("ListUnsent() error = %v", err)
	}
	if len(events) != 2 {
		t.Fatalf("ListUnsent() = %d events, want 2", len(events))
	}
	if events[0].EventID != "oldest" || events[1].EventID != "middle" {
		t.Fatalf("ListUnsent() order = [%s, %s], want [oldest, middle]", events[0].EventID, events[1].EventID)
	}
	if events[0].ID == 0 || events[0].AggregateType != "capture" || events[0].AggregateID != "capture-oldest" ||
		events[0].EventType != "capture_created" || events[0].PayloadJSON != `{"id":"capture-oldest"}` {
		t.Fatalf("first event = %+v", events[0])
	}
	if events[0].SentAt == nil || !events[0].SentAt.Equal(sentAt) {
		t.Fatalf("SentAt = %v, want %v", events[0].SentAt, sentAt)
	}
	if events[0].AckedAt != nil {
		t.Fatalf("AckedAt = %v, want nil", events[0].AckedAt)
	}
}

func TestOutboxRepositoryListUnsentReleasesConnection(t *testing.T) {
	database := openMigratedDB(t)
	repo := NewOutboxRepository(database)
	ctx := context.Background()
	base := time.Date(2026, 7, 17, 1, 0, 0, 0, time.UTC)

	seedOutboxEvent(t, database, "event-1", base, nil, nil)

	if _, err := repo.ListUnsent(ctx, 1); err != nil {
		t.Fatalf("ListUnsent() error = %v", err)
	}

	queryCtx, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()
	var count int
	if err := database.QueryRowContext(queryCtx, `SELECT count(*) FROM sync_outbox`).Scan(&count); err != nil {
		t.Fatalf("query after ListUnsent() error = %v", err)
	}
	if count != 1 {
		t.Fatalf("count = %d, want 1", count)
	}
}

func TestOutboxRepositoryMarkAcked(t *testing.T) {
	database := openMigratedDB(t)
	repo := NewOutboxRepository(database)
	ctx := context.Background()
	base := time.Date(2026, 7, 17, 1, 0, 0, 0, time.UTC)
	kst := time.FixedZone("KST", 9*60*60)
	ackedAt := time.Date(2026, 7, 17, 12, 30, 0, 0, kst)

	seedOutboxEvent(t, database, "event-1", base, nil, nil)
	seedOutboxEvent(t, database, "event-2", base.Add(time.Minute), nil, nil)
	seedOutboxEvent(t, database, "event-3", base.Add(2*time.Minute), nil, nil)

	if err := repo.MarkAcked(ctx, []string{"event-1", "event-3"}, ackedAt); err != nil {
		t.Fatalf("MarkAcked() error = %v", err)
	}
	if err := repo.MarkAcked(ctx, []string{"event-1"}, ackedAt.Add(time.Minute)); err != nil {
		t.Fatalf("MarkAcked() idempotent error = %v", err)
	}
	if err := repo.MarkAcked(ctx, nil, ackedAt); err != nil {
		t.Fatalf("MarkAcked(nil) error = %v", err)
	}

	events, err := repo.ListUnsent(ctx, 10)
	if err != nil {
		t.Fatalf("ListUnsent() error = %v", err)
	}
	if len(events) != 1 || events[0].EventID != "event-2" {
		t.Fatalf("remaining events = %+v, want only event-2", events)
	}

	var sentAt sql.NullTime
	var storedAckedAt sql.NullTime
	if err := database.QueryRowContext(ctx, `SELECT sent_at, acked_at FROM sync_outbox WHERE event_id = ?`, "event-1").
		Scan(&sentAt, &storedAckedAt); err != nil {
		t.Fatalf("query acked event: %v", err)
	}
	if !sentAt.Valid || !sentAt.Time.Equal(ackedAt.UTC()) {
		t.Fatalf("sent_at = %#v, want %v", sentAt, ackedAt.UTC())
	}
	if !storedAckedAt.Valid || !storedAckedAt.Time.Equal(ackedAt.Add(time.Minute).UTC()) {
		t.Fatalf("acked_at = %#v, want %v", storedAckedAt, ackedAt.Add(time.Minute).UTC())
	}
}

func TestOutboxRepositoryPendingCount(t *testing.T) {
	database := openMigratedDB(t)
	repo := NewOutboxRepository(database)
	ctx := context.Background()
	base := time.Date(2026, 7, 17, 1, 0, 0, 0, time.UTC)

	seedOutboxEvent(t, database, "pending-1", base, nil, nil)
	seedOutboxEvent(t, database, "pending-2", base.Add(time.Minute), nil, nil)
	seedOutboxEvent(t, database, "acked", base.Add(2*time.Minute), nil, ptrTime(base.Add(3*time.Minute)))

	count, err := repo.PendingCount(ctx)
	if err != nil {
		t.Fatalf("PendingCount() error = %v", err)
	}
	if count != 2 {
		t.Fatalf("PendingCount() = %d, want 2", count)
	}
}

func seedOutboxEvent(t *testing.T, database *sql.DB, eventID string, createdAt time.Time, sentAt, ackedAt *time.Time) {
	t.Helper()
	if _, err := database.ExecContext(context.Background(), `INSERT INTO sync_outbox(
event_id, aggregate_type, aggregate_id, event_type, payload_json, created_at, sent_at, acked_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		eventID, "capture", "capture-"+eventID, "capture_created", `{"id":"capture-`+eventID+`"}`,
		createdAt.UTC(), nullableTime(sentAt), nullableTime(ackedAt),
	); err != nil {
		t.Fatalf("insert sync_outbox %s: %v", eventID, err)
	}
}

func nullableTime(t *time.Time) any {
	if t == nil {
		return nil
	}
	return t.UTC()
}

func ptrTime(t time.Time) *time.Time {
	return &t
}
