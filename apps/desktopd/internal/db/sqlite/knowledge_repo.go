package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"neulsang/desktopd/internal/domain/knowledge"
	"neulsang/desktopd/internal/id"
)

type KnowledgeRepository struct {
	db *sql.DB
}

var _ knowledge.Repository = (*KnowledgeRepository)(nil)

func NewKnowledgeRepository(db *sql.DB) *KnowledgeRepository {
	return &KnowledgeRepository{db: db}
}

func (r *KnowledgeRepository) MarkUnknown(ctx context.Context, knowledgeItemID string, at time.Time) (knowledge.MarkResult, error) {
	cardsCreated := 0
	result, err := r.mark(ctx, knowledgeItemID, func(ctx context.Context, tx *sql.Tx) error {
		if _, err := tx.ExecContext(
			ctx,
			`INSERT INTO learner_items(id, knowledge_item_id, wrong_count, last_wrong_at, status)
VALUES (?, ?, 1, ?, ?)
ON CONFLICT(knowledge_item_id) DO UPDATE SET
  wrong_count = wrong_count + 1,
  last_wrong_at = excluded.last_wrong_at,
  status = ?`,
			id.New(), knowledgeItemID, at, knowledge.StatusActive, knowledge.StatusActive,
		); err != nil {
			return err
		}
		// Marking a word unknown turns its candidates into review cards (PRD Task06),
		// atomically with the learner-state change.
		n, err := generateReviewCardsFromCandidates(ctx, tx, knowledgeItemID, at)
		if err != nil {
			return err
		}
		cardsCreated = n
		return nil
	})
	if err != nil {
		return knowledge.MarkResult{}, err
	}
	result.CardsCreated = cardsCreated
	return result, nil
}

func (r *KnowledgeRepository) MarkKnown(ctx context.Context, knowledgeItemID string, at time.Time) (knowledge.MarkResult, error) {
	return r.mark(ctx, knowledgeItemID, func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(
			ctx,
			`INSERT INTO learner_items(id, knowledge_item_id, status)
VALUES (?, ?, ?)
ON CONFLICT(knowledge_item_id) DO UPDATE SET status = ?`,
			id.New(), knowledgeItemID, knowledge.StatusKnown, knowledge.StatusKnown,
		)
		return err
	})
}

// mark runs a learner_items mutation inside a transaction after confirming the
// knowledge item exists (FK enforcement would reject a bad id, but an explicit
// check lets the caller return a clean 404), then reads back the resulting state.
func (r *KnowledgeRepository) mark(ctx context.Context, knowledgeItemID string, mutate func(context.Context, *sql.Tx) error) (result knowledge.MarkResult, resultErr error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return knowledge.MarkResult{}, fmt.Errorf("begin knowledge mark transaction: %w", err)
	}
	defer func() {
		if resultErr != nil {
			resultErr = errors.Join(resultErr, tx.Rollback())
		}
	}()

	var exists int
	switch err := tx.QueryRowContext(ctx, `SELECT 1 FROM knowledge_items WHERE id = ?`, knowledgeItemID).Scan(&exists); {
	case errors.Is(err, sql.ErrNoRows):
		return knowledge.MarkResult{}, knowledge.ErrKnowledgeItemNotFound
	case err != nil:
		return knowledge.MarkResult{}, fmt.Errorf("check knowledge item: %w", err)
	}

	if err := mutate(ctx, tx); err != nil {
		return knowledge.MarkResult{}, fmt.Errorf("mutate learner item: %w", err)
	}

	out := knowledge.MarkResult{KnowledgeItemID: knowledgeItemID}
	if err := tx.QueryRowContext(
		ctx,
		`SELECT status, ask_count, wrong_count FROM learner_items WHERE knowledge_item_id = ?`,
		knowledgeItemID,
	).Scan(&out.Status, &out.AskCount, &out.WrongCount); err != nil {
		return knowledge.MarkResult{}, fmt.Errorf("read learner item: %w", err)
	}
	if err := tx.QueryRowContext(
		ctx,
		`SELECT count(*) FROM review_card_candidates WHERE knowledge_item_id = ?`,
		knowledgeItemID,
	).Scan(&out.CandidateCount); err != nil {
		return knowledge.MarkResult{}, fmt.Errorf("count review card candidates: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return knowledge.MarkResult{}, fmt.Errorf("commit knowledge mark transaction: %w", err)
	}
	return out, nil
}

// ListByCapture returns the capture's linked knowledge items joined with learner
// state. It first confirms the capture exists so an unknown id yields
// ErrCaptureNotFound rather than an ambiguous empty list.
func (r *KnowledgeRepository) ListByCapture(ctx context.Context, captureID string) (items []knowledge.CaptureItem, resultErr error) {
	var exists bool
	if err := r.db.QueryRowContext(
		ctx, `SELECT EXISTS(SELECT 1 FROM captures WHERE id = ?)`, captureID,
	).Scan(&exists); err != nil {
		return nil, fmt.Errorf("check capture exists: %w", err)
	}
	if !exists {
		return nil, knowledge.ErrCaptureNotFound
	}

	rows, err := r.db.QueryContext(
		ctx,
		`SELECT ki.id, ki.surface_text, ki.item_type, ki.pronunciation, ki.meaning_ko,
       ci.role, ci.confidence,
       COALESCE(li.status, 'active'), COALESCE(li.ask_count, 0), COALESCE(li.wrong_count, 0)
FROM capture_items ci
JOIN knowledge_items ki ON ki.id = ci.knowledge_item_id
LEFT JOIN learner_items li ON li.knowledge_item_id = ki.id
WHERE ci.capture_id = ?
ORDER BY ci.confidence DESC, ki.surface_text`,
		captureID,
	)
	if err != nil {
		return nil, fmt.Errorf("select capture knowledge items: %w", err)
	}
	defer func() {
		if err := rows.Close(); err != nil && resultErr == nil {
			resultErr = fmt.Errorf("close capture knowledge rows: %w", err)
		}
	}()

	for rows.Next() {
		var item knowledge.CaptureItem
		var pronunciation, meaning sql.NullString
		if err := rows.Scan(
			&item.KnowledgeItemID, &item.SurfaceText, &item.ItemType, &pronunciation, &meaning,
			&item.Role, &item.Confidence, &item.Status, &item.AskCount, &item.WrongCount,
		); err != nil {
			return nil, fmt.Errorf("scan capture knowledge item: %w", err)
		}
		item.PronunciationKo = nullString(pronunciation)
		item.MeaningKo = nullString(meaning)
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate capture knowledge items: %w", err)
	}
	return items, nil
}
