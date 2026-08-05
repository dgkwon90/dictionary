// Package outbox owns the sync_outbox read/send use case without depending on
// storage or network infrastructure.
package outbox

import (
	"context"
	"errors"
	"time"
)

// ErrPermanent marks a rejection the server will never accept — a malformed or
// unknown event, not a network hiccup or an overloaded server.
//
// The distinction matters because the queue is oldest-first: retrying a permanently
// rejected event forever also blocks every good event behind it, so the sync silently
// stops working and the outbox only grows. A Publisher returns this (wrapped) when it
// can tell the difference; anything else is treated as transient and retried.
var ErrPermanent = errors.New("sync rejected permanently")

type Event struct {
	ID            int64      `json:"id"`
	EventID       string     `json:"event_id"`
	AggregateType string     `json:"aggregate_type"`
	AggregateID   string     `json:"aggregate_id"`
	EventType     string     `json:"event_type"`
	PayloadJSON   string     `json:"payload_json"`
	CreatedAt     time.Time  `json:"created_at"`
	SentAt        *time.Time `json:"sent_at"`
	AckedAt       *time.Time `json:"acked_at"`
	// FailedAt is set when the event was quarantined: kept in the ledger, skipped by
	// the sender. Nil for everything still queued or already acked.
	FailedAt *time.Time `json:"failed_at"`
}

type Repository interface {
	// ListUnsent returns queued events oldest-first, skipping quarantined ones.
	ListUnsent(ctx context.Context, limit int) ([]Event, error)
	MarkAcked(ctx context.Context, eventIDs []string, at time.Time) error
	// MarkFailed quarantines one event with the reason it was rejected.
	MarkFailed(ctx context.Context, eventID string, at time.Time, reason string) error
	// PendingCount counts what is still waiting to be sent (quarantined excluded).
	PendingCount(ctx context.Context) (int, error)
	// FailedCount counts quarantined events, so the status endpoint can show that
	// something needs a human rather than reporting a healthy empty queue.
	FailedCount(ctx context.Context) (int, error)
}

type Publisher interface {
	Publish(ctx context.Context, events []Event) error
}

type Status struct {
	Enabled bool `json:"enabled"`
	Pending int  `json:"pending"`
	Failed  int  `json:"failed"`
}
