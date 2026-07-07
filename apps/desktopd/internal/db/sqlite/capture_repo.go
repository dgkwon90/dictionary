package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"neulsang/desktopd/internal/domain/capture"
)

type CaptureRepository struct {
	db *sql.DB
}

func NewCaptureRepository(db *sql.DB) *CaptureRepository {
	return &CaptureRepository{db: db}
}

func (r *CaptureRepository) SaveNew(ctx context.Context, c capture.Capture, j capture.LookupJob, e capture.OutboxEvent) (resultErr error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin capture transaction: %w", err)
	}
	defer func() {
		if resultErr != nil {
			resultErr = errors.Join(resultErr, tx.Rollback())
		}
	}()

	if _, err := tx.ExecContext(
		ctx, `INSERT INTO captures(
id, source_app, source_type, source_title, source_url, selected_text, detected_lang, input_mode, text_hash, created_at, inbox_status
) VALUES (?, NULLIF(?, ''), NULLIF(?, ''), NULLIF(?, ''), NULLIF(?, ''), ?, NULLIF(?, ''), ?, ?, ?, ?)`,
		c.ID, c.SourceApp, c.SourceType, c.SourceTitle, c.SourceURL, c.SelectedText, c.DetectedLang, c.InputMode, c.TextHash, c.CreatedAt, c.InboxStatus,
	); err != nil {
		return fmt.Errorf("insert capture: %w", err)
	}
	if _, err := tx.ExecContext(
		ctx, `INSERT INTO lookup_jobs(
id, capture_id, status, created_at
) VALUES (?, ?, ?, ?)`,
		j.ID, j.CaptureID, j.Status, j.CreatedAt,
	); err != nil {
		return fmt.Errorf("insert lookup job: %w", err)
	}
	if _, err := tx.ExecContext(
		ctx, `INSERT INTO sync_outbox(
event_id, aggregate_type, aggregate_id, event_type, payload_json, created_at
) VALUES (?, ?, ?, ?, ?, ?)`,
		e.EventID, e.AggregateType, e.AggregateID, e.EventType, e.PayloadJSON, e.CreatedAt,
	); err != nil {
		return fmt.Errorf("insert sync outbox: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit capture transaction: %w", err)
	}
	return nil
}
