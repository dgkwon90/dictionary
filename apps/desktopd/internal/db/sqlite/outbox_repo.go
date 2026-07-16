package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"neulsang/desktopd/internal/domain/outbox"
)

type OutboxRepository struct {
	db *sql.DB
}

var _ outbox.Repository = (*OutboxRepository)(nil)

func NewOutboxRepository(db *sql.DB) *OutboxRepository {
	return &OutboxRepository{db: db}
}

func (r *OutboxRepository) ListUnsent(ctx context.Context, limit int) (events []outbox.Event, resultErr error) {
	rows, err := r.db.QueryContext(ctx, `SELECT id, event_id, aggregate_type, aggregate_id, event_type, payload_json, created_at, sent_at, acked_at
FROM sync_outbox
WHERE acked_at IS NULL
ORDER BY created_at, id
LIMIT ?`, limit)
	if err != nil {
		return nil, fmt.Errorf("query sync outbox: %w", err)
	}
	defer func() {
		if err := rows.Close(); err != nil && resultErr == nil {
			resultErr = fmt.Errorf("close sync outbox rows: %w", err)
		}
	}()

	for rows.Next() {
		var event outbox.Event
		var sentAt sql.NullTime
		var ackedAt sql.NullTime
		if err := rows.Scan(
			&event.ID, &event.EventID, &event.AggregateType, &event.AggregateID, &event.EventType,
			&event.PayloadJSON, &event.CreatedAt, &sentAt, &ackedAt,
		); err != nil {
			return nil, fmt.Errorf("scan sync outbox: %w", err)
		}
		event.CreatedAt = event.CreatedAt.UTC()
		event.SentAt = nullTimeToPtr(sentAt)
		event.AckedAt = nullTimeToPtr(ackedAt)
		events = append(events, event)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate sync outbox: %w", err)
	}
	return events, nil
}

func (r *OutboxRepository) MarkAcked(ctx context.Context, eventIDs []string, at time.Time) (resultErr error) {
	if len(eventIDs) == 0 {
		return nil
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin sync outbox ack transaction: %w", err)
	}
	defer func() {
		if resultErr != nil {
			resultErr = errors.Join(resultErr, tx.Rollback())
		}
	}()

	ackedAt := at.UTC()
	for _, eventID := range eventIDs {
		if _, err := tx.ExecContext(ctx,
			`UPDATE sync_outbox SET sent_at = COALESCE(sent_at, ?), acked_at = ? WHERE event_id = ?`,
			ackedAt, ackedAt, eventID,
		); err != nil {
			return fmt.Errorf("mark sync outbox acked: %w", err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit sync outbox ack transaction: %w", err)
	}
	return nil
}

func (r *OutboxRepository) PendingCount(ctx context.Context) (int, error) {
	var count int
	if err := r.db.QueryRowContext(ctx, `SELECT count(*) FROM sync_outbox WHERE acked_at IS NULL`).Scan(&count); err != nil {
		return 0, fmt.Errorf("count pending sync outbox: %w", err)
	}
	return count, nil
}

func nullTimeToPtr(value sql.NullTime) *time.Time {
	if !value.Valid {
		return nil
	}
	t := value.Time.UTC()
	return &t
}
