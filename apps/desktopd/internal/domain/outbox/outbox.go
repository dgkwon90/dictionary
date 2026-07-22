// Package outbox owns the sync_outbox read/send use case without depending on
// storage or network infrastructure.
package outbox

import (
	"context"
	"time"
)

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
}

type Repository interface {
	ListUnsent(ctx context.Context, limit int) ([]Event, error)
	MarkAcked(ctx context.Context, eventIDs []string, at time.Time) error
	PendingCount(ctx context.Context) (int, error)
}

type Publisher interface {
	Publish(ctx context.Context, events []Event) error
}

type Status struct {
	Enabled bool `json:"enabled"`
	Pending int  `json:"pending"`
}
