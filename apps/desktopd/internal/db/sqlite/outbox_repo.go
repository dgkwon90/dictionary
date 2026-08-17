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
	// 격리된 이벤트(failed_at)는 건너뛴다 — 그러지 않으면 서버가 영원히 거절하는 이벤트
	// 하나가 오래된 것부터 보내는 큐의 맨 앞을 계속 차지한다(0004).
	rows, err := r.db.QueryContext(ctx, `SELECT id, event_id, aggregate_type, aggregate_id, event_type, payload_json, created_at, sent_at, acked_at, failed_at
FROM sync_outbox
WHERE acked_at IS NULL AND failed_at IS NULL
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
		var failedAt sql.NullTime
		if err := rows.Scan(
			&event.ID, &event.EventID, &event.AggregateType, &event.AggregateID, &event.EventType,
			&event.PayloadJSON, &event.CreatedAt, &sentAt, &ackedAt, &failedAt,
		); err != nil {
			return nil, fmt.Errorf("scan sync outbox: %w", err)
		}
		event.CreatedAt = event.CreatedAt.UTC()
		event.SentAt = nullTimeToPtr(sentAt)
		event.AckedAt = nullTimeToPtr(ackedAt)
		event.FailedAt = nullTimeToPtr(failedAt)
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

// MarkFailed quarantines one event: it stays in the ledger with the reason it was
// rejected, and ListUnsent stops handing it to the publisher.
func (r *OutboxRepository) MarkFailed(ctx context.Context, eventID string, at time.Time, reason string) error {
	if _, err := r.db.ExecContext(ctx,
		`UPDATE sync_outbox SET failed_at = ?, failure_reason = ? WHERE event_id = ? AND acked_at IS NULL`,
		at.UTC(), reason, eventID,
	); err != nil {
		return fmt.Errorf("mark sync outbox failed: %w", err)
	}
	return nil
}

func (r *OutboxRepository) FailedCount(ctx context.Context) (int, error) {
	var count int
	if err := r.db.QueryRowContext(ctx,
		`SELECT count(*) FROM sync_outbox WHERE failed_at IS NOT NULL AND acked_at IS NULL`).Scan(&count); err != nil {
		return 0, fmt.Errorf("count quarantined sync outbox events: %w", err)
	}
	return count, nil
}

func (r *OutboxRepository) PendingCount(ctx context.Context) (int, error) {
	var count int
	if err := r.db.QueryRowContext(ctx, `SELECT count(*) FROM sync_outbox WHERE acked_at IS NULL AND failed_at IS NULL`).Scan(&count); err != nil {
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
