package sqlite

import (
	"context"
	"database/sql"
	"fmt"

	"neulsang/desktopd/internal/domain/inbox"
)

type InboxRepository struct {
	db *sql.DB
}

var _ inbox.Repository = (*InboxRepository)(nil)

func NewInboxRepository(db *sql.DB) *InboxRepository {
	return &InboxRepository{db: db}
}

const inboxListQuery = `WITH job_latest AS (
  SELECT capture_id, status,
         ROW_NUMBER() OVER (PARTITION BY capture_id ORDER BY created_at DESC) AS rn
  FROM lookup_jobs
),
review_flag AS (
  SELECT DISTINCT ci.capture_id
  FROM capture_items ci
  JOIN review_cards rc ON rc.knowledge_item_id = ci.knowledge_item_id
),
computed AS (
  SELECT
    c.id AS capture_id,
    c.selected_text,
    c.source_app,
    c.source_type,
    c.input_mode,
    c.created_at,
    jl.status AS job_status,
    e.brief_ko,
    CASE
      WHEN c.inbox_status IN ('saved','archived') THEN c.inbox_status
      WHEN jl.status = 'failed' THEN 'failed'
      WHEN rf.capture_id IS NOT NULL THEN 'review_added'
      ELSE 'new'
    END AS effective_status
  FROM captures c
  LEFT JOIN job_latest jl ON jl.capture_id = c.id AND jl.rn = 1
  LEFT JOIN explanations e ON e.capture_id = c.id
  LEFT JOIN review_flag rf ON rf.capture_id = c.id
)
SELECT capture_id, selected_text, source_app, source_type, input_mode, created_at, job_status, brief_ko, effective_status
FROM computed
ORDER BY created_at DESC
LIMIT ?`

const inboxListByStatusQuery = `WITH job_latest AS (
  SELECT capture_id, status,
         ROW_NUMBER() OVER (PARTITION BY capture_id ORDER BY created_at DESC) AS rn
  FROM lookup_jobs
),
review_flag AS (
  SELECT DISTINCT ci.capture_id
  FROM capture_items ci
  JOIN review_cards rc ON rc.knowledge_item_id = ci.knowledge_item_id
),
computed AS (
  SELECT
    c.id AS capture_id,
    c.selected_text,
    c.source_app,
    c.source_type,
    c.input_mode,
    c.created_at,
    jl.status AS job_status,
    e.brief_ko,
    CASE
      WHEN c.inbox_status IN ('saved','archived') THEN c.inbox_status
      WHEN jl.status = 'failed' THEN 'failed'
      WHEN rf.capture_id IS NOT NULL THEN 'review_added'
      ELSE 'new'
    END AS effective_status
  FROM captures c
  LEFT JOIN job_latest jl ON jl.capture_id = c.id AND jl.rn = 1
  LEFT JOIN explanations e ON e.capture_id = c.id
  LEFT JOIN review_flag rf ON rf.capture_id = c.id
)
SELECT capture_id, selected_text, source_app, source_type, input_mode, created_at, job_status, brief_ko, effective_status
FROM computed
WHERE effective_status = ?
ORDER BY created_at DESC
LIMIT ?`

func (r *InboxRepository) List(ctx context.Context, status string, limit int) (items []inbox.Item, resultErr error) {
	query := inboxListQuery
	args := []any{limit}
	if status != "" {
		query = inboxListByStatusQuery
		args = []any{status, limit}
	}

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("select inbox items: %w", err)
	}
	defer func() {
		if err := rows.Close(); err != nil && resultErr == nil {
			resultErr = fmt.Errorf("close inbox rows: %w", err)
		}
	}()

	for rows.Next() {
		var item inbox.Item
		var sourceApp sql.NullString
		var sourceType sql.NullString
		var jobStatus sql.NullString
		var briefKo sql.NullString
		if err := rows.Scan(
			&item.CaptureID,
			&item.SelectedText,
			&sourceApp,
			&sourceType,
			&item.InputMode,
			&item.CreatedAt,
			&jobStatus,
			&briefKo,
			&item.Status,
		); err != nil {
			return nil, fmt.Errorf("scan inbox item: %w", err)
		}
		item.SourceApp = nullString(sourceApp)
		item.SourceType = nullString(sourceType)
		item.JobStatus = nullString(jobStatus)
		item.BriefKo = nullString(briefKo)
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate inbox items: %w", err)
	}
	return items, nil
}

func (r *InboxRepository) SetStatus(ctx context.Context, captureID, status string) error {
	result, err := r.db.ExecContext(ctx, `UPDATE captures SET inbox_status = ? WHERE id = ?`, status, captureID)
	if err != nil {
		return fmt.Errorf("update inbox status: %w", err)
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("read inbox status rows affected: %w", err)
	}
	if rowsAffected == 0 {
		return inbox.ErrCaptureNotFound
	}
	return nil
}

func nullString(value sql.NullString) string {
	if value.Valid {
		return value.String
	}
	return ""
}
