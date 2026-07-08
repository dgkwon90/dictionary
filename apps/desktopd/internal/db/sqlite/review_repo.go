package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"neulsang/desktopd/internal/domain/review"
	"neulsang/desktopd/internal/id"
)

type ReviewRepository struct {
	db *sql.DB
}

var _ review.Repository = (*ReviewRepository)(nil)

func NewReviewRepository(db *sql.DB) *ReviewRepository {
	return &ReviewRepository{db: db}
}

func (r *ReviewRepository) DueCards(ctx context.Context, now time.Time, limit int) (cards []review.Card, resultErr error) {
	rows, err := r.db.QueryContext(
		ctx,
		`SELECT id, knowledge_item_id, card_type, question, state, due_at
FROM review_cards
WHERE due_at IS NOT NULL AND due_at <= ?
ORDER BY due_at ASC
LIMIT ?`,
		now, limit,
	)
	if err != nil {
		return nil, fmt.Errorf("select due review cards: %w", err)
	}
	defer func() {
		if err := rows.Close(); err != nil && resultErr == nil {
			resultErr = fmt.Errorf("close due review card rows: %w", err)
		}
	}()

	for rows.Next() {
		var card review.Card
		if err := rows.Scan(&card.CardID, &card.KnowledgeItemID, &card.CardType, &card.Question, &card.State, &card.DueAt); err != nil {
			return nil, fmt.Errorf("scan due review card: %w", err)
		}
		cards = append(cards, card)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate due review cards: %w", err)
	}
	return cards, nil
}

// candidateForCard is a review_card_candidate not yet turned into a card.
type candidateForCard struct {
	cardType    string
	question    string
	answer      string
	explanation sql.NullString
}

// generateReviewCardsFromCandidates turns every not-yet-consumed candidate of a
// knowledge item into a review_cards row (PRD Task06). New cards are due immediately
// (due_at = now, state = new) so they surface in the next review. Consuming the
// candidates makes a repeated mark-unknown idempotent. It runs inside the caller's
// transaction so card creation commits atomically with the learner-state change.
func generateReviewCardsFromCandidates(ctx context.Context, tx *sql.Tx, knowledgeItemID string, now time.Time) (int, error) {
	rows, err := tx.QueryContext(
		ctx,
		`SELECT card_type, question, answer, explanation
FROM review_card_candidates
WHERE knowledge_item_id = ? AND consumed_at IS NULL`,
		knowledgeItemID,
	)
	if err != nil {
		return 0, fmt.Errorf("select unconsumed candidates: %w", err)
	}
	var candidates []candidateForCard
	for rows.Next() {
		var candidate candidateForCard
		if err := rows.Scan(&candidate.cardType, &candidate.question, &candidate.answer, &candidate.explanation); err != nil {
			return 0, fmt.Errorf("scan candidate: %w", err)
		}
		candidates = append(candidates, candidate)
	}
	if err := rows.Err(); err != nil {
		return 0, fmt.Errorf("iterate candidates: %w", err)
	}
	if err := rows.Close(); err != nil {
		return 0, fmt.Errorf("close candidate rows: %w", err)
	}
	if len(candidates) == 0 {
		return 0, nil
	}

	for _, candidate := range candidates {
		cardType := candidate.cardType
		if cardType == "" {
			cardType = review.DefaultCardType
		}
		if _, err := tx.ExecContext(
			ctx,
			`INSERT INTO review_cards(
id, knowledge_item_id, card_type, question, answer, explanation, state, due_at, created_at, updated_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			id.New(), knowledgeItemID, cardType, candidate.question, candidate.answer, candidate.explanation, review.CardStateNew, now, now, now,
		); err != nil {
			return 0, fmt.Errorf("insert review card: %w", err)
		}
	}

	if _, err := tx.ExecContext(
		ctx,
		`UPDATE review_card_candidates SET consumed_at = ? WHERE knowledge_item_id = ? AND consumed_at IS NULL`,
		now, knowledgeItemID,
	); err != nil {
		return 0, fmt.Errorf("mark candidates consumed: %w", err)
	}
	return len(candidates), nil
}
