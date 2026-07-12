package notification

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"neulsang/desktopd/internal/domain/settings"
)

const (
	// reviewDueWindow: a slot only fires within this window after its configured time,
	// so opening the app long after a missed slot does not surface a stale reminder.
	reviewDueWindow = 2 * time.Hour
	// reviewDueTTL bounds how long an unacked review reminder stays live.
	reviewDueTTL = 6 * time.Hour
	// defaultTick is how often the scheduler re-evaluates the slots.
	defaultTick = time.Minute
)

// PreferencesReader supplies the notification toggle and review-slot times.
type PreferencesReader interface {
	Load(ctx context.Context) (settings.Preferences, error)
}

// DueCounter reports how many review cards are due at a given time.
type DueCounter interface {
	CountDueCards(ctx context.Context, at time.Time) (int, error)
}

// Scheduler enqueues review_due reminders at the configured morning/evening slots when
// due cards exist. Enqueue is idempotent per (date, slot), so repeated ticks — and
// restarts within the same slot — never double-notify (ADR-0008).
type Scheduler struct {
	prefs PreferencesReader
	due   DueCounter
	repo  Repository
	log   *slog.Logger
	now   func() time.Time
	tick  time.Duration
}

func NewScheduler(prefs PreferencesReader, due DueCounter, repo Repository, log *slog.Logger) *Scheduler {
	return &Scheduler{prefs: prefs, due: due, repo: repo, log: log, now: time.Now, tick: defaultTick}
}

// Run evaluates the slots every tick until ctx is cancelled.
func (s *Scheduler) Run(ctx context.Context) {
	ticker := time.NewTicker(s.tick)
	defer ticker.Stop()
	s.checkLogged(ctx)
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.checkLogged(ctx)
		}
	}
}

func (s *Scheduler) checkLogged(ctx context.Context) {
	if err := s.Check(ctx); err != nil {
		s.log.Warn("review reminder check failed", "error", err)
	}
}

// Check runs a single scheduling pass. Exported for tests.
func (s *Scheduler) Check(ctx context.Context) error {
	prefs, err := s.prefs.Load(ctx)
	if err != nil {
		return fmt.Errorf("load preferences: %w", err)
	}
	if !prefs.NotificationsEnabled {
		return nil
	}

	now := s.now()
	slots := [...]struct{ name, hhmm string }{
		{"morning", prefs.MorningReviewTime},
		{"evening", prefs.EveningReviewTime},
	}

	due := -1 // count due cards at most once, and only when a slot is active
	for _, slot := range slots {
		slotTime, ok := slotToday(now, slot.hhmm)
		if !ok {
			continue
		}
		if now.Before(slotTime) || !now.Before(slotTime.Add(reviewDueWindow)) {
			continue
		}
		if due < 0 {
			due, err = s.due.CountDueCards(ctx, now)
			if err != nil {
				return fmt.Errorf("count due cards: %w", err)
			}
		}
		if due == 0 {
			continue
		}
		if err := s.repo.Enqueue(ctx, Notification{
			Kind:      KindReviewDue,
			DedupKey:  fmt.Sprintf("review_due:%s:%s", now.Format("2006-01-02"), slot.name),
			Title:     "복습 시간이에요",
			Body:      fmt.Sprintf("복습할 카드가 %d개 있어요.", due),
			Route:     "Today Review",
			CreatedAt: now,
			ExpiresAt: now.Add(reviewDueTTL),
		}); err != nil {
			return fmt.Errorf("enqueue review_due: %w", err)
		}
	}
	return nil
}

// slotToday builds today's local time for an "HH:MM" slot; ok=false if unparseable.
func slotToday(now time.Time, hhmm string) (time.Time, bool) {
	t, err := time.ParseInLocation("15:04", hhmm, now.Location())
	if err != nil {
		return time.Time{}, false
	}
	return time.Date(now.Year(), now.Month(), now.Day(), t.Hour(), t.Minute(), 0, 0, now.Location()), true
}
