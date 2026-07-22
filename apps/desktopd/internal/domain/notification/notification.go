// Package notification is the sidecar→UI event ledger (ADR-0008). desktopd records
// coalesced notifications (result_ready when an explanation finishes, review_due at the
// configured morning/evening slots); the Tauri shell polls the unacked list, renders OS
// notifications + a tray "New" indicator, then acks. This domain has no infra
// dependency (PRD §18.1).
package notification

import (
	"context"
	"errors"
	"time"
)

// Kinds of notification. dedup_key format differs per kind so they never collide:
// result_ready uses the capture id, review_due uses "review_due:<date>:<slot>".
const (
	KindResultReady = "result_ready"
	KindReviewDue   = "review_due"
)

// ResultReadyTTL bounds how long an unacked "result ready" stays live, so a stale
// explanation from a previous session does not notify after a restart (ADR-0008).
const ResultReadyTTL = 30 * time.Minute

// DefaultRecentLimit / MaxRecentLimit bound the in-app notification log (#24).
const (
	DefaultRecentLimit = 50
	MaxRecentLimit     = 200
)

// ErrNotFound is returned when acking a notification id that does not exist.
var ErrNotFound = errors.New("notification not found")

// Notification is one coalesced event. ExpiresAt/AckedAt zero means unset.
type Notification struct {
	ID        string
	Kind      string
	DedupKey  string
	Title     string
	Body      string
	Route     string // UI navigation target on click (matches App.tsx route labels)
	PayloadID string // capture id for result_ready, else empty
	CreatedAt time.Time
	ExpiresAt time.Time
	AckedAt   time.Time
}

// Repository persists and queries notifications. Enqueue coalesces on dedup_key
// (no-op if a row already exists), so callers can enqueue idempotently.
type Repository interface {
	Enqueue(ctx context.Context, n Notification) error
	// ListUnacked returns unacked, non-expired notifications oldest-first.
	ListUnacked(ctx context.Context, at time.Time) ([]Notification, error)
	// ListRecent returns the most recent notifications (acked included) newest-first,
	// for the in-app notification log (#24). Ignores the enabled toggle and expiry.
	ListRecent(ctx context.Context, limit int) ([]Notification, error)
	// Ack marks a notification seen; returns false if the id does not exist.
	Ack(ctx context.Context, id string) (bool, error)
	// AckByCapture marks a capture's result_ready seen (best-effort, no-op if none).
	// The capture's result_ready row uses the capture id as its dedup_key.
	AckByCapture(ctx context.Context, captureID string) error
}

// Service is the read/ack use case exposed over HTTP.
type Service struct {
	repo  Repository
	prefs PreferencesReader
	now   func() time.Time
}

func NewService(repo Repository, prefs PreferencesReader) *Service {
	return &Service{repo: repo, prefs: prefs, now: time.Now}
}

// Pending is the unacked feed plus the badge count (== len(Notifications)).
type Pending struct {
	Notifications []Notification
	UnackedCount  int
}

// Pending returns the unacked feed. When the user has turned notifications off it
// surfaces nothing — result_ready rows are still enqueued (atomic with the
// explanation) but must not reach the UI while the toggle is off (codex #18).
func (s *Service) Pending(ctx context.Context) (Pending, error) {
	prefs, err := s.prefs.Load(ctx)
	if err != nil {
		return Pending{}, err
	}
	if !prefs.NotificationsEnabled {
		return Pending{}, nil
	}
	list, err := s.repo.ListUnacked(ctx, s.now())
	if err != nil {
		return Pending{}, err
	}
	return Pending{Notifications: list, UnackedCount: len(list)}, nil
}

func (s *Service) Ack(ctx context.Context, id string) error {
	ok, err := s.repo.Ack(ctx, id)
	if err != nil {
		return err
	}
	if !ok {
		return ErrNotFound
	}
	return nil
}

// AckCapture acks a capture's result_ready when the Quick Search popup already showed
// the result, so the poll loop does not fire a redundant OS notification for something
// the user just read (codex #18). Best-effort: no error if there is no such row.
func (s *Service) AckCapture(ctx context.Context, captureID string) error {
	return s.repo.AckByCapture(ctx, captureID)
}

// Recent returns the notification history for the in-app log (#24). Unlike Pending it
// is NOT gated by the enabled toggle — it is a log the user opens deliberately, and it
// includes already-acked rows so past notifications remain visible.
func (s *Service) Recent(ctx context.Context, limit int) ([]Notification, error) {
	if limit <= 0 {
		limit = DefaultRecentLimit
	}
	if limit > MaxRecentLimit {
		limit = MaxRecentLimit
	}
	return s.repo.ListRecent(ctx, limit)
}
