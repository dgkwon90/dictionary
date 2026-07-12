package notification

import (
	"context"
	"io"
	"log/slog"
	"testing"
	"time"

	"neulsang/desktopd/internal/domain/settings"
)

type fakePrefs struct {
	prefs settings.Preferences
	err   error
}

func (f fakePrefs) Load(context.Context) (settings.Preferences, error) {
	return f.prefs, f.err
}

type fakeDue struct {
	count int
	calls int
}

func (f *fakeDue) CountDueCards(context.Context, time.Time) (int, error) {
	f.calls++
	return f.count, nil
}

func quietScheduler(prefs PreferencesReader, due DueCounter, repo Repository, now time.Time) *Scheduler {
	s := NewScheduler(prefs, due, repo, slog.New(slog.NewTextHandler(io.Discard, nil)))
	s.now = func() time.Time { return now }
	return s
}

func enabledPrefs(morning, evening string) settings.Preferences {
	return settings.Preferences{NotificationsEnabled: true, MorningReviewTime: morning, EveningReviewTime: evening}
}

func TestSchedulerEnqueuesWithinSlotWindow(t *testing.T) {
	now := time.Date(2026, 7, 13, 9, 30, 0, 0, time.Local)
	repo := &fakeRepo{}
	due := &fakeDue{count: 3}
	s := quietScheduler(fakePrefs{prefs: enabledPrefs("09:00", "21:00")}, due, repo, now)

	if err := s.Check(context.Background()); err != nil {
		t.Fatalf("Check() error = %v", err)
	}
	if len(repo.enqueued) != 1 {
		t.Fatalf("enqueued = %d, want 1", len(repo.enqueued))
	}
	n := repo.enqueued[0]
	if n.Kind != KindReviewDue || n.DedupKey != "review_due:2026-07-13:morning" {
		t.Fatalf("notification = %+v", n)
	}
	if n.Route != "Today Review" || n.Body != "복습할 카드가 3개 있어요." {
		t.Fatalf("notification content = %+v", n)
	}
	if n.ExpiresAt.IsZero() {
		t.Fatalf("expected TTL on review_due")
	}
}

func TestSchedulerSkipsWhenNoDueCards(t *testing.T) {
	now := time.Date(2026, 7, 13, 9, 30, 0, 0, time.Local)
	repo := &fakeRepo{}
	s := quietScheduler(fakePrefs{prefs: enabledPrefs("09:00", "21:00")}, &fakeDue{count: 0}, repo, now)

	if err := s.Check(context.Background()); err != nil {
		t.Fatalf("Check() error = %v", err)
	}
	if len(repo.enqueued) != 0 {
		t.Fatalf("enqueued = %d, want 0", len(repo.enqueued))
	}
}

func TestSchedulerSkipsBeforeSlotAndAfterWindow(t *testing.T) {
	repo := &fakeRepo{}
	due := &fakeDue{count: 5}
	// 08:30 is before the 09:00 morning slot; 11:30 is past the 2h window (09:00–11:00).
	for _, hm := range []struct{ h, m int }{{8, 30}, {11, 30}} {
		now := time.Date(2026, 7, 13, hm.h, hm.m, 0, 0, time.Local)
		s := quietScheduler(fakePrefs{prefs: enabledPrefs("09:00", "21:00")}, due, repo, now)
		if err := s.Check(context.Background()); err != nil {
			t.Fatalf("Check() error = %v", err)
		}
	}
	if len(repo.enqueued) != 0 {
		t.Fatalf("enqueued = %d, want 0 (outside window)", len(repo.enqueued))
	}
	if due.calls != 0 {
		t.Fatalf("CountDueCards called %d times, want 0 (no active slot)", due.calls)
	}
}

func TestSchedulerDisabledDoesNothing(t *testing.T) {
	now := time.Date(2026, 7, 13, 9, 30, 0, 0, time.Local)
	repo := &fakeRepo{}
	prefs := settings.Preferences{NotificationsEnabled: false, MorningReviewTime: "09:00", EveningReviewTime: "21:00"}
	s := quietScheduler(fakePrefs{prefs: prefs}, &fakeDue{count: 9}, repo, now)

	if err := s.Check(context.Background()); err != nil {
		t.Fatalf("Check() error = %v", err)
	}
	if len(repo.enqueued) != 0 {
		t.Fatalf("enqueued = %d, want 0 (disabled)", len(repo.enqueued))
	}
}

func TestSchedulerEveningSlot(t *testing.T) {
	now := time.Date(2026, 7, 13, 21, 15, 0, 0, time.Local)
	repo := &fakeRepo{}
	s := quietScheduler(fakePrefs{prefs: enabledPrefs("09:00", "21:00")}, &fakeDue{count: 2}, repo, now)

	if err := s.Check(context.Background()); err != nil {
		t.Fatalf("Check() error = %v", err)
	}
	if len(repo.enqueued) != 1 || repo.enqueued[0].DedupKey != "review_due:2026-07-13:evening" {
		t.Fatalf("enqueued = %+v, want one evening notification", repo.enqueued)
	}
}
