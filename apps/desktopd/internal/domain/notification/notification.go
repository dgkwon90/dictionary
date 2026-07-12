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
	// Ack marks a notification seen; returns false if the id does not exist.
	Ack(ctx context.Context, id string) (bool, error)
}

// Service is the read/ack use case exposed over HTTP.
type Service struct {
	repo Repository
	now  func() time.Time
}

func NewService(repo Repository) *Service {
	return &Service{repo: repo, now: time.Now}
}

// Pending is the unacked feed plus the badge count (== len(Notifications)).
type Pending struct {
	Notifications []Notification
	UnackedCount  int
}

func (s *Service) Pending(ctx context.Context) (Pending, error) {
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
