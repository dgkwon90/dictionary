package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"neulsang/desktopd/internal/domain/knowledge"
	"neulsang/desktopd/internal/domain/notification"
	"neulsang/desktopd/internal/id"
)

// insertNotificationSQL is shared by NotificationRepository.Enqueue and by the explain
// SaveSuccess transaction (result_ready), so the column list is single-sourced. The
// dedup_key UNIQUE constraint makes this coalesce (and blocks post-ack re-fire).
const insertNotificationSQL = `INSERT INTO notifications(id, kind, dedup_key, title, body, route, payload_id, created_at, expires_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(dedup_key) DO NOTHING`

// execContexter is satisfied by both *sql.DB and *sql.Tx.
type execContexter interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
}

func insertNotification(ctx context.Context, ex execContexter, n notification.Notification) error {
	// Store all timestamps in UTC. The driver persists time.Time as a tz-qualified
	// wall-clock string, so mixing zones would break the string comparisons in
	// ListUnacked; UTC everywhere keeps them monotonic (matches stats/review repos).
	if _, err := ex.ExecContext(ctx, insertNotificationSQL,
		id.New(), n.Kind, n.DedupKey, n.Title, n.Body,
		nilIfEmpty(n.Route), nilIfEmpty(n.PayloadID), n.CreatedAt.UTC(), nilIfZeroTime(n.ExpiresAt),
	); err != nil {
		return fmt.Errorf("insert notification: %w", err)
	}
	return nil
}

func nilIfEmpty(s string) any {
	if s == "" {
		return nil
	}
	return s
}

func nilIfZeroTime(t time.Time) any {
	if t.IsZero() {
		return nil
	}
	return t.UTC()
}

type NotificationRepository struct {
	db *sql.DB
}

func NewNotificationRepository(db *sql.DB) *NotificationRepository {
	return &NotificationRepository{db: db}
}

func (r *NotificationRepository) Enqueue(ctx context.Context, n notification.Notification) error {
	return insertNotification(ctx, r.db, n)
}

func (r *NotificationRepository) ListUnacked(ctx context.Context, at time.Time) (list []notification.Notification, resultErr error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT id, kind, dedup_key, title, body, route, payload_id, created_at, expires_at
FROM notifications
WHERE acked_at IS NULL AND (expires_at IS NULL OR expires_at > ?)
ORDER BY created_at`, at.UTC())
	if err != nil {
		return nil, fmt.Errorf("query notifications: %w", err)
	}
	defer func() {
		if err := rows.Close(); err != nil && resultErr == nil {
			resultErr = fmt.Errorf("close notifications rows: %w", err)
		}
	}()

	for rows.Next() {
		var n notification.Notification
		var route, payloadID sql.NullString
		var expiresAt sql.NullTime
		if err := rows.Scan(&n.ID, &n.Kind, &n.DedupKey, &n.Title, &n.Body, &route, &payloadID, &n.CreatedAt, &expiresAt); err != nil {
			return nil, fmt.Errorf("scan notification: %w", err)
		}
		n.Route = route.String
		n.PayloadID = payloadID.String
		if expiresAt.Valid {
			n.ExpiresAt = expiresAt.Time
		}
		list = append(list, n)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate notifications: %w", err)
	}
	return list, nil
}

// Ack is idempotent: acking an already-acked row keeps the original ack time and still
// reports found. Returns false only when the id does not exist.
func (r *NotificationRepository) Ack(ctx context.Context, notificationID string) (bool, error) {
	res, err := r.db.ExecContext(ctx,
		`UPDATE notifications SET acked_at = COALESCE(acked_at, ?) WHERE id = ?`,
		time.Now().UTC(), notificationID)
	if err != nil {
		return false, fmt.Errorf("ack notification: %w", err)
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("ack rows affected: %w", err)
	}
	return affected > 0, nil
}

// AckByCapture acks a capture's result_ready (dedup_key == captureID). Best-effort:
// no row (still running / failed / already acked) is not an error.
func (r *NotificationRepository) AckByCapture(ctx context.Context, captureID string) error {
	if _, err := r.db.ExecContext(ctx,
		`UPDATE notifications SET acked_at = COALESCE(acked_at, ?) WHERE dedup_key = ?`,
		time.Now().UTC(), captureID); err != nil {
		return fmt.Errorf("ack notification by capture: %w", err)
	}
	return nil
}

// CountDueCards reports review cards due at `at` whose learner item is not "known",
// mirroring the dashboard due predicate (PRD §13). Satisfies notification.DueCounter.
func (r *NotificationRepository) CountDueCards(ctx context.Context, at time.Time) (int, error) {
	var count int
	if err := r.db.QueryRowContext(ctx,
		`SELECT count(*)
FROM review_cards rc
LEFT JOIN learner_items li ON li.knowledge_item_id = rc.knowledge_item_id
WHERE rc.due_at IS NOT NULL AND rc.due_at <= ? AND COALESCE(li.status, 'active') <> ?`,
		at.UTC(), knowledge.StatusKnown).Scan(&count); err != nil {
		return 0, fmt.Errorf("count due cards: %w", err)
	}
	return count, nil
}
