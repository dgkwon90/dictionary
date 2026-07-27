package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"neulsang/desktopd/internal/domain/learning"
	"neulsang/desktopd/internal/domain/review"
)

type LearningRepository struct {
	db *sql.DB
}

var _ learning.Repository = (*LearningRepository)(nil)

func NewLearningRepository(db *sql.DB) *LearningRepository {
	return &LearningRepository{db: db}
}

// learningSelect reads one learning item.
//
// The card count and the next due date are scalar subqueries rather than a join to
// review_cards on purpose (R5). A word that appears in a sentence owns both a meaning
// card and a cloze card, so joining would return it twice and any ranking computed
// over the result would count that word twice — the ranking columns all live on
// learner_items, which has exactly one row per item, and the card figures are read
// per item without ever widening it.
const learningSelect = `SELECT ki.id, ki.surface_text, ki.learn_kind, ki.pronunciation,
       ki.meaning_ko, ki.description_ko,
       li.status, li.ask_count, li.unknown_count, li.attempt_count, li.correct_count,
       li.registered_at, li.last_graded_at,
       (SELECT count(*) FROM review_cards rc WHERE rc.knowledge_item_id = ki.id),
       -- ORDER BY ... LIMIT 1 rather than min(due_at): an aggregate loses the column's
       -- DATETIME affinity and the driver hands back a string instead of a time.
       (SELECT rc.due_at FROM review_cards rc WHERE rc.knowledge_item_id = ki.id
        ORDER BY rc.due_at ASC LIMIT 1)
FROM learner_items li
JOIN knowledge_items ki ON ki.id = li.knowledge_item_id`

// learningWeakness is review.WeaknessScore written as SQL so the "자주 틀림" tab can
// order by it before applying LIMIT. The coefficients are bound from
// review.DefaultWeaknessWeights() rather than typed in, and
// TestLearningRepositoryWeakScopeOrderingMatchesDomainScore holds the expression
// itself to the Go function.
const learningWeakness = `MAX(0, li.ask_count * ? + li.unknown_count * ? -
  CASE WHEN li.attempt_count > 0
       THEN (CAST(li.correct_count AS REAL) / li.attempt_count) * ?
       ELSE 0 END)`

func (r *LearningRepository) List(ctx context.Context, input learning.ListInput) (items []learning.Item, resultErr error) {
	query := learningSelect + "\nWHERE " + learnerIsActive
	args := []any{}
	if input.LearnKind != "" {
		query += "\n  AND ki.learn_kind = ?"
		args = append(args, input.LearnKind)
	}
	if !input.Since.IsZero() {
		query += "\n  AND li.registered_at >= ?"
		args = append(args, utc(input.Since))
	}
	if input.Query != "" {
		query += "\n  AND (ki.surface_text LIKE ? OR COALESCE(ki.meaning_ko, '') LIKE ?)"
		like := "%" + input.Query + "%"
		args = append(args, like, like)
	}

	if input.Scope == learning.ScopeWeak {
		// "자주 틀림" shows only items with evidence of difficulty *beyond registering
		// them*. Registration itself writes unknown_count = 1 — committing to learn a
		// word is already saying you did not know it — so "unknown_count > 0" would
		// match every row and make this tab a reordered copy of [전체]. What counts is
		// meeting the word again and still not knowing it (a second unknown mark), or
		// missing it in a review.
		query += "\n  AND (li.unknown_count > 1 OR li.attempt_count > li.correct_count)"
		weights := review.DefaultWeaknessWeights()
		query += "\nORDER BY " + learningWeakness + " DESC, li.unknown_count DESC, ki.surface_text"
		args = append(args, weights.Ask, weights.Unknown, weights.Accuracy)
	} else {
		// Newest registration first: the list answers "what did I take on recently",
		// and the dated scopes are windows on the same axis.
		query += "\nORDER BY li.registered_at DESC, ki.surface_text"
	}
	query += "\nLIMIT ?"
	args = append(args, input.Limit)

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("select learning items: %w", err)
	}
	defer func() {
		if err := rows.Close(); err != nil && resultErr == nil {
			resultErr = fmt.Errorf("close learning rows: %w", err)
		}
	}()

	for rows.Next() {
		item, err := scanLearningItem(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate learning items: %w", err)
	}
	return items, nil
}

// SetStatus updates one item and reads it back in the same transaction, so the caller
// is told what the row actually became rather than what it was asked to become.
func (r *LearningRepository) SetStatus(ctx context.Context, knowledgeItemID, status string, at time.Time) (item learning.Item, resultErr error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return learning.Item{}, fmt.Errorf("begin learning status transaction: %w", err)
	}
	defer func() {
		if resultErr != nil {
			resultErr = errors.Join(resultErr, tx.Rollback())
		}
	}()

	result, err := tx.ExecContext(
		ctx,
		`UPDATE learner_items SET status = ?, updated_at = ? WHERE knowledge_item_id = ?`,
		status, utc(at), knowledgeItemID,
	)
	if err != nil {
		return learning.Item{}, fmt.Errorf("update learner item status: %w", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return learning.Item{}, fmt.Errorf("learner item rows affected: %w", err)
	}
	if affected == 0 {
		// The knowledge item may well exist — it just was never registered for
		// learning, so there is nothing here to retire or remove.
		return learning.Item{}, learning.ErrItemNotFound
	}

	item, err = scanLearningItem(tx.QueryRowContext(ctx, learningSelect+"\nWHERE li.knowledge_item_id = ?", knowledgeItemID))
	if err != nil {
		return learning.Item{}, err
	}
	if err := tx.Commit(); err != nil {
		return learning.Item{}, fmt.Errorf("commit learning status transaction: %w", err)
	}
	return item, nil
}

// scanner is satisfied by both *sql.Row and *sql.Rows so the row shape is written once.
type scanner interface {
	Scan(dest ...any) error
}

func scanLearningItem(row scanner) (learning.Item, error) {
	var item learning.Item
	var pronunciation, meaning, description sql.NullString
	var lastGradedAt, nextDueAt sql.NullTime
	if err := row.Scan(
		&item.KnowledgeItemID, &item.SurfaceText, &item.LearnKind, &pronunciation,
		&meaning, &description,
		&item.Status, &item.AskCount, &item.UnknownCount, &item.AttemptCount, &item.CorrectCount,
		&item.RegisteredAt, &lastGradedAt,
		&item.CardCount, &nextDueAt,
	); err != nil {
		return learning.Item{}, fmt.Errorf("scan learning item: %w", err)
	}
	item.PronunciationKo = nullString(pronunciation)
	item.MeaningKo = nullString(meaning)
	item.DescriptionKo = nullString(description)
	if lastGradedAt.Valid {
		item.LastGradedAt = lastGradedAt.Time
	}
	if nextDueAt.Valid {
		item.NextDueAt = nextDueAt.Time
	}
	return item, nil
}
