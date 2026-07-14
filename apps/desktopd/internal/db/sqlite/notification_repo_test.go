package sqlite

import (
	"context"
	"testing"
	"time"

	"neulsang/desktopd/internal/domain/notification"
)

func TestNotificationRepositoryEnqueueAndList(t *testing.T) {
	repo := NewNotificationRepository(openMigratedDB(t))
	ctx := context.Background()
	base := time.Date(2026, 7, 13, 9, 0, 0, 0, time.UTC)

	if err := repo.Enqueue(ctx, notification.Notification{
		Kind: notification.KindResultReady, DedupKey: "cap-1", Title: "t1", Body: "b1", Route: "Inbox", PayloadID: "cap-1", CreatedAt: base,
	}); err != nil {
		t.Fatalf("Enqueue() error = %v", err)
	}
	if err := repo.Enqueue(ctx, notification.Notification{
		Kind: notification.KindReviewDue, DedupKey: "review_due:2026-07-13:morning", Title: "t2", Body: "b2", Route: "Today Review", CreatedAt: base.Add(time.Minute),
	}); err != nil {
		t.Fatalf("Enqueue() error = %v", err)
	}

	list, err := repo.ListUnacked(ctx, base.Add(time.Hour))
	if err != nil {
		t.Fatalf("ListUnacked() error = %v", err)
	}
	if len(list) != 2 {
		t.Fatalf("ListUnacked() = %d, want 2", len(list))
	}
	if list[0].DedupKey != "cap-1" || list[0].PayloadID != "cap-1" || list[0].Route != "Inbox" {
		t.Fatalf("first notification = %+v", list[0])
	}
}

func TestNotificationRepositoryCoalescesOnDedupKey(t *testing.T) {
	repo := NewNotificationRepository(openMigratedDB(t))
	ctx := context.Background()
	base := time.Date(2026, 7, 13, 9, 0, 0, 0, time.UTC)

	for i := 0; i < 3; i++ {
		if err := repo.Enqueue(ctx, notification.Notification{
			Kind: notification.KindResultReady, DedupKey: "cap-1", Title: "t", Body: "b", CreatedAt: base,
		}); err != nil {
			t.Fatalf("Enqueue() error = %v", err)
		}
	}
	list, err := repo.ListUnacked(ctx, base.Add(time.Hour))
	if err != nil {
		t.Fatalf("ListUnacked() error = %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("ListUnacked() = %d, want 1 (coalesced)", len(list))
	}
}

func TestNotificationRepositoryAck(t *testing.T) {
	repo := NewNotificationRepository(openMigratedDB(t))
	ctx := context.Background()
	base := time.Date(2026, 7, 13, 9, 0, 0, 0, time.UTC)

	if err := repo.Enqueue(ctx, notification.Notification{
		Kind: notification.KindResultReady, DedupKey: "cap-1", Title: "t", Body: "b", CreatedAt: base,
	}); err != nil {
		t.Fatalf("Enqueue() error = %v", err)
	}
	list, err := repo.ListUnacked(ctx, base.Add(time.Hour))
	if err != nil {
		t.Fatalf("ListUnacked() error = %v", err)
	}
	id := list[0].ID

	found, err := repo.Ack(ctx, id)
	if err != nil || !found {
		t.Fatalf("Ack() = (%v, %v), want (true, nil)", found, err)
	}
	after, err := repo.ListUnacked(ctx, base.Add(time.Hour))
	if err != nil {
		t.Fatalf("ListUnacked() error = %v", err)
	}
	if len(after) != 0 {
		t.Fatalf("ListUnacked() after ack = %d, want 0", len(after))
	}

	// Idempotent: re-acking still reports found (row exists).
	if found, err := repo.Ack(ctx, id); err != nil || !found {
		t.Fatalf("re-Ack() = (%v, %v), want (true, nil)", found, err)
	}
	// Unknown id reports not found.
	if found, err := repo.Ack(ctx, "does-not-exist"); err != nil || found {
		t.Fatalf("Ack(unknown) = (%v, %v), want (false, nil)", found, err)
	}
}

func TestNotificationRepositoryAckByCapture(t *testing.T) {
	repo := NewNotificationRepository(openMigratedDB(t))
	ctx := context.Background()
	base := time.Date(2026, 7, 13, 9, 0, 0, 0, time.UTC)

	if err := repo.Enqueue(ctx, notification.Notification{
		Kind: notification.KindResultReady, DedupKey: "cap-1", Title: "t", Body: "b", CreatedAt: base,
	}); err != nil {
		t.Fatalf("Enqueue() error = %v", err)
	}
	// Best-effort no-op for an unknown capture.
	if err := repo.AckByCapture(ctx, "cap-unknown"); err != nil {
		t.Fatalf("AckByCapture(unknown) error = %v", err)
	}
	if err := repo.AckByCapture(ctx, "cap-1"); err != nil {
		t.Fatalf("AckByCapture() error = %v", err)
	}
	list, err := repo.ListUnacked(ctx, base.Add(time.Hour))
	if err != nil {
		t.Fatalf("ListUnacked() error = %v", err)
	}
	if len(list) != 0 {
		t.Fatalf("ListUnacked() after AckByCapture = %d, want 0", len(list))
	}
}

func TestNotificationRepositoryExpiredNotListed(t *testing.T) {
	repo := NewNotificationRepository(openMigratedDB(t))
	ctx := context.Background()
	base := time.Date(2026, 7, 13, 9, 0, 0, 0, time.UTC)

	if err := repo.Enqueue(ctx, notification.Notification{
		Kind: notification.KindResultReady, DedupKey: "cap-1", Title: "t", Body: "b", CreatedAt: base, ExpiresAt: base.Add(time.Minute),
	}); err != nil {
		t.Fatalf("Enqueue() error = %v", err)
	}
	// Query after expiry.
	list, err := repo.ListUnacked(ctx, base.Add(2*time.Minute))
	if err != nil {
		t.Fatalf("ListUnacked() error = %v", err)
	}
	if len(list) != 0 {
		t.Fatalf("ListUnacked() = %d, want 0 (expired)", len(list))
	}
	// Still live before expiry.
	live, err := repo.ListUnacked(ctx, base.Add(30*time.Second))
	if err != nil {
		t.Fatalf("ListUnacked() error = %v", err)
	}
	if len(live) != 1 {
		t.Fatalf("ListUnacked() before expiry = %d, want 1", len(live))
	}
}

func TestNotificationRepositoryCountDueCardsEmpty(t *testing.T) {
	repo := NewNotificationRepository(openMigratedDB(t))
	count, err := repo.CountDueCards(context.Background(), time.Now())
	if err != nil {
		t.Fatalf("CountDueCards() error = %v", err)
	}
	if count != 0 {
		t.Fatalf("CountDueCards() = %d, want 0", count)
	}
}
