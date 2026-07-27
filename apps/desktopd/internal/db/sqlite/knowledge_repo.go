package sqlite

import (
	"context"
	"database/sql"
	"fmt"

	"neulsang/desktopd/internal/domain/knowledge"
)

type KnowledgeRepository struct {
	db *sql.DB
}

var _ knowledge.Repository = (*KnowledgeRepository)(nil)

func NewKnowledgeRepository(db *sql.DB) *KnowledgeRepository {
	return &KnowledgeRepository{db: db}
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
		`SELECT ki.id, ki.surface_text, ki.learn_kind, ki.pronunciation, ki.meaning_ko,
       ci.role, ci.confidence,
       COALESCE(li.status, 'active'), COALESCE(li.ask_count, 0), COALESCE(li.unknown_count, 0)
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
			&item.Role, &item.Confidence, &item.Status, &item.AskCount, &item.UnknownCount,
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
