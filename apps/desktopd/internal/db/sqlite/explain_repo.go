package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"neulsang/desktopd/internal/domain/explain"
	"neulsang/desktopd/internal/id"
)

type ExplainRepository struct {
	db *sql.DB
}

func NewExplainRepository(db *sql.DB) *ExplainRepository {
	return &ExplainRepository{db: db}
}

func (r *ExplainRepository) MarkRunning(ctx context.Context, jobID string, startedAt time.Time) error {
	if _, err := r.db.ExecContext(ctx, `UPDATE lookup_jobs SET status = 'running', started_at = ? WHERE id = ?`, startedAt, jobID); err != nil {
		return fmt.Errorf("mark explain job running: %w", err)
	}
	return nil
}

func (r *ExplainRepository) SaveSuccess(ctx context.Context, jobID, captureID string, result explain.ExplainResult, rawResponseJSON string, finishedAt time.Time) (resultErr error) {
	examplesJSON, err := json.Marshal(result.Examples)
	if err != nil {
		return fmt.Errorf("marshal examples: %w", err)
	}
	termsJSON, err := json.Marshal(result.SubItems)
	if err != nil {
		return fmt.Errorf("marshal sub items: %w", err)
	}

	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin explain success transaction: %w", err)
	}
	defer func() {
		if resultErr != nil {
			resultErr = errors.Join(resultErr, tx.Rollback())
		}
	}()

	if _, err := tx.ExecContext(
		ctx,
		`INSERT INTO explanations(
id, capture_id, brief_ko, detailed_ko, pronunciation, examples_json, terms_json, difficulty_estimate, category, raw_response_json, created_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		id.New(), captureID, result.BriefKo, result.DetailedKo, result.PronunciationKo, string(examplesJSON), string(termsJSON), result.Difficulty, result.DomainCategory, rawResponseJSON, finishedAt,
	); err != nil {
		return fmt.Errorf("insert explanation: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `UPDATE lookup_jobs SET status = 'done', finished_at = ? WHERE id = ?`, finishedAt, jobID); err != nil {
		return fmt.Errorf("mark explain job done: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit explain success transaction: %w", err)
	}
	return nil
}

func (r *ExplainRepository) SaveFailure(ctx context.Context, jobID string, errMessage string, finishedAt time.Time) error {
	if _, err := r.db.ExecContext(ctx, `UPDATE lookup_jobs SET status = 'failed', error_message = ?, finished_at = ? WHERE id = ?`, errMessage, finishedAt, jobID); err != nil {
		return fmt.Errorf("mark explain job failed: %w", err)
	}
	return nil
}

func (r *ExplainRepository) GetSnapshot(ctx context.Context, captureID string) (explain.Snapshot, error) {
	var status string
	var errorMessage sql.NullString
	err := r.db.QueryRowContext(
		ctx,
		`SELECT status, error_message FROM lookup_jobs WHERE capture_id = ? ORDER BY created_at DESC LIMIT 1`,
		captureID,
	).Scan(&status, &errorMessage)
	switch {
	case err == nil:
	case errors.Is(err, sql.ErrNoRows):
		return explain.Snapshot{}, explain.ErrCaptureNotFound
	default:
		return explain.Snapshot{}, fmt.Errorf("select latest lookup job: %w", err)
	}

	snapshot := explain.Snapshot{Status: status}
	if errorMessage.Valid {
		snapshot.ErrorMessage = errorMessage.String
	}
	if status != "done" {
		return snapshot, nil
	}

	var result explain.ExplainResult
	var examplesJSON string
	var termsJSON string
	if err := r.db.QueryRowContext(
		ctx,
		`SELECT brief_ko, detailed_ko, pronunciation, examples_json, terms_json, difficulty_estimate, category
FROM explanations WHERE capture_id = ?`,
		captureID,
	).Scan(&result.BriefKo, &result.DetailedKo, &result.PronunciationKo, &examplesJSON, &termsJSON, &result.Difficulty, &result.DomainCategory); err != nil {
		return explain.Snapshot{}, fmt.Errorf("select explanation: %w", err)
	}
	if err := json.Unmarshal([]byte(examplesJSON), &result.Examples); err != nil {
		return explain.Snapshot{}, fmt.Errorf("unmarshal examples: %w", err)
	}
	if err := json.Unmarshal([]byte(termsJSON), &result.SubItems); err != nil {
		return explain.Snapshot{}, fmt.Errorf("unmarshal sub items: %w", err)
	}
	// InputType, DetectedLanguage, and ReviewCardCandidates are preserved only
	// in raw_response_json in backlog #4 and are not projected into this snapshot.
	snapshot.Result = &result
	return snapshot, nil
}
