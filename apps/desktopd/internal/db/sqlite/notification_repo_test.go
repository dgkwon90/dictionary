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
		Kind: notification.KindResultReady, DedupKey: "cap-1", Title: "t1", Body: "b1", Route: notification.RouteSearchHistory, PayloadID: "cap-1", CreatedAt: base,
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
	if list[0].DedupKey != "cap-1" || list[0].PayloadID != "cap-1" || list[0].Route != notification.RouteSearchHistory {
		t.Fatalf("first notification = %+v", list[0])
	}
}

func TestNotificationRepositoryListRecentIncludesAcked(t *testing.T) {
	repo := NewNotificationRepository(openMigratedDB(t))
	ctx := context.Background()
	base := time.Date(2026, 7, 13, 9, 0, 0, 0, time.UTC)

	if err := repo.Enqueue(ctx, notification.Notification{
		Kind: notification.KindResultReady, DedupKey: "cap-1", Title: "old", Body: "b", CreatedAt: base,
	}); err != nil {
		t.Fatalf("Enqueue() error = %v", err)
	}
	if err := repo.Enqueue(ctx, notification.Notification{
		Kind: notification.KindReviewDue, DedupKey: "review_due:2026-07-13:morning", Title: "new", Body: "b", CreatedAt: base.Add(time.Minute),
	}); err != nil {
		t.Fatalf("Enqueue() error = %v", err)
	}

	// Ack the older one; ListRecent must still surface it (history includes acked).
	unacked, err := repo.ListUnacked(ctx, base.Add(time.Hour))
	if err != nil {
		t.Fatalf("ListUnacked() error = %v", err)
	}
	var oldID string
	for _, n := range unacked {
		if n.DedupKey == "cap-1" {
			oldID = n.ID
		}
	}
	if _, err := repo.Ack(ctx, oldID); err != nil {
		t.Fatalf("Ack() error = %v", err)
	}

	recent, err := repo.ListRecent(ctx, 50)
	if err != nil {
		t.Fatalf("ListRecent() error = %v", err)
	}
	if len(recent) != 2 {
		t.Fatalf("ListRecent() = %d, want 2 (acked included)", len(recent))
	}
	// Newest first.
	if recent[0].Title != "new" || recent[1].Title != "old" {
		t.Fatalf("order = [%q, %q], want [new, old]", recent[0].Title, recent[1].Title)
	}
	// The acked older row carries AckedAt; the unacked newer one does not.
	if !recent[0].AckedAt.IsZero() {
		t.Fatalf("newest should be unacked, got AckedAt=%v", recent[0].AckedAt)
	}
	if recent[1].AckedAt.IsZero() {
		t.Fatalf("older row should carry AckedAt")
	}
}

// A deleted notification has to disappear from both reads: the log the user is looking
// at, and the unacked feed the Rust shell polls to fire OS banners. Missing the second
// one would mean the user deletes a notification and the banner still pops.
func TestNotificationRepositoryDeleteHidesFromBothLists(t *testing.T) {
	repo := NewNotificationRepository(openMigratedDB(t))
	ctx := context.Background()
	base := time.Date(2026, 8, 5, 9, 0, 0, 0, time.UTC)
	if err := repo.Enqueue(ctx, notification.Notification{
		Kind: notification.KindResultReady, DedupKey: "cap-1", Title: "t", Body: "b",
		Route: notification.RouteSearchHistory, PayloadID: "cap-1", CreatedAt: base,
	}); err != nil {
		t.Fatalf("Enqueue() error = %v", err)
	}
	var id string
	if err := repo.db.QueryRowContext(ctx, `SELECT id FROM notifications`).Scan(&id); err != nil {
		t.Fatalf("read id: %v", err)
	}

	deleted, err := repo.Delete(ctx, id, base.Add(time.Minute))
	if err != nil || !deleted {
		t.Fatalf("Delete() = %v, %v", deleted, err)
	}

	unacked, err := repo.ListUnacked(ctx, base.Add(time.Minute))
	if err != nil {
		t.Fatalf("ListUnacked() error = %v", err)
	}
	if len(unacked) != 0 {
		t.Errorf("ListUnacked() = %d, want 0 — a deleted notification must not fire a banner", len(unacked))
	}
	recent, err := repo.ListRecent(ctx, 10)
	if err != nil {
		t.Fatalf("ListRecent() error = %v", err)
	}
	if len(recent) != 0 {
		t.Errorf("ListRecent() = %d, want 0", len(recent))
	}

	// Deleting again is not an error: the id still exists, and the user's intent
	// ("this should be gone") is already true.
	again, err := repo.Delete(ctx, id, base.Add(2*time.Minute))
	if err != nil || !again {
		t.Errorf("second Delete() = %v, %v, want true, nil", again, err)
	}
	if missing, err := repo.Delete(ctx, "no-such-id", base); err != nil || missing {
		t.Errorf("Delete(unknown) = %v, %v, want false, nil", missing, err)
	}
}

// The row is kept on delete precisely so its dedup_key keeps working. Without this,
// the review reminder the user just deleted would be re-enqueued on the scheduler's
// next tick inside the same slot window and pop straight back up.
func TestNotificationRepositoryDeletedRowStillBlocksItsDedupKey(t *testing.T) {
	repo := NewNotificationRepository(openMigratedDB(t))
	ctx := context.Background()
	base := time.Date(2026, 8, 5, 21, 0, 0, 0, time.UTC)
	reminder := notification.Notification{
		Kind: notification.KindReviewDue, DedupKey: "review_due:2026-08-05:evening",
		Title: "복습 시간이에요", Body: "복습할 카드가 3개 있어요.",
		Route: notification.RouteTodayReview, CreatedAt: base,
	}
	if err := repo.Enqueue(ctx, reminder); err != nil {
		t.Fatalf("Enqueue() error = %v", err)
	}
	var id string
	if err := repo.db.QueryRowContext(ctx, `SELECT id FROM notifications`).Scan(&id); err != nil {
		t.Fatalf("read id: %v", err)
	}
	if _, err := repo.Delete(ctx, id, base.Add(time.Minute)); err != nil {
		t.Fatalf("Delete() error = %v", err)
	}

	// The scheduler tries again a minute later, as it does every tick in the window.
	reminder.CreatedAt = base.Add(2 * time.Minute)
	if err := repo.Enqueue(ctx, reminder); err != nil {
		t.Fatalf("re-Enqueue() error = %v", err)
	}

	list, err := repo.ListUnacked(ctx, base.Add(3*time.Minute))
	if err != nil {
		t.Fatalf("ListUnacked() error = %v", err)
	}
	if len(list) != 0 {
		t.Fatalf("ListUnacked() = %d, want 0 — the deleted reminder came back", len(list))
	}
}

func TestNotificationRepositoryDeleteAllCountsWhatItHid(t *testing.T) {
	repo := NewNotificationRepository(openMigratedDB(t))
	ctx := context.Background()
	base := time.Date(2026, 8, 5, 9, 0, 0, 0, time.UTC)
	for i, key := range []string{"cap-1", "cap-2", "cap-3"} {
		if err := repo.Enqueue(ctx, notification.Notification{
			Kind: notification.KindResultReady, DedupKey: key, Title: "t", Body: "b",
			Route: notification.RouteSearchHistory, PayloadID: key,
			CreatedAt: base.Add(time.Duration(i) * time.Minute),
		}); err != nil {
			t.Fatalf("Enqueue(%s) error = %v", key, err)
		}
	}

	deleted, err := repo.DeleteAll(ctx, base.Add(time.Hour))
	if err != nil {
		t.Fatalf("DeleteAll() error = %v", err)
	}
	if deleted != 3 {
		t.Fatalf("DeleteAll() = %d, want 3", deleted)
	}
	// Nothing left to hide: a second clear reports zero rather than counting again.
	if again, err := repo.DeleteAll(ctx, base.Add(2*time.Hour)); err != nil || again != 0 {
		t.Fatalf("second DeleteAll() = %d, %v, want 0, nil", again, err)
	}
	recent, err := repo.ListRecent(ctx, 10)
	if err != nil {
		t.Fatalf("ListRecent() error = %v", err)
	}
	if len(recent) != 0 {
		t.Fatalf("ListRecent() = %d, want 0", len(recent))
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
