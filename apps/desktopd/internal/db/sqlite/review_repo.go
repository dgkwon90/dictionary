package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"neulsang/desktopd/internal/domain/knowledge"
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
		`SELECT rc.id, rc.knowledge_item_id, rc.card_type, rc.question, rc.answer, rc.explanation, rc.state, rc.due_at
FROM review_cards rc
LEFT JOIN learner_items li ON li.knowledge_item_id = rc.knowledge_item_id
WHERE rc.due_at IS NOT NULL
  AND rc.due_at <= ?
  AND COALESCE(li.status, 'active') <> ?
ORDER BY rc.due_at ASC
LIMIT ?`,
		now, knowledge.StatusKnown, limit,
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
		var explanation sql.NullString
		if err := rows.Scan(&card.CardID, &card.KnowledgeItemID, &card.CardType, &card.Question, &card.Answer, &explanation, &card.State, &card.DueAt); err != nil {
			return nil, fmt.Errorf("scan due review card: %w", err)
		}
		card.Explanation = explanation.String
		cards = append(cards, card)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate due review cards: %w", err)
	}
	return cards, nil
}

// reviewLogSource marks a review_logs entry that came from a grading session.
const reviewLogSource = "review"

// Grade applies a rating to a card (PRD §15.6): it reschedules the card via
// review.NextSchedule, appends an append-only review_logs row, and bumps the card's
// reps/lapses and the learner_items review_count — all in one transaction.
func (r *ReviewRepository) Grade(ctx context.Context, cardID, rating string, elapsedMs int, now time.Time) (result review.GradeResult, resultErr error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return review.GradeResult{}, fmt.Errorf("begin grade transaction: %w", err)
	}
	defer func() {
		if resultErr != nil {
			resultErr = errors.Join(resultErr, tx.Rollback())
		}
	}()

	var reps, lapses int
	var prevIntervalDays float64
	var knowledgeItemID string
	switch err := tx.QueryRowContext(
		ctx,
		`SELECT rc.reps, rc.lapses, rc.stability, rc.knowledge_item_id
FROM review_cards rc
LEFT JOIN learner_items li ON li.knowledge_item_id = rc.knowledge_item_id
WHERE rc.id = ? AND COALESCE(li.status, 'active') <> ?`,
		cardID, knowledge.StatusKnown,
	).Scan(&reps, &lapses, &prevIntervalDays, &knowledgeItemID); {
	case errors.Is(err, sql.ErrNoRows):
		return review.GradeResult{}, review.ErrCardNotFound
	case err != nil:
		return review.GradeResult{}, fmt.Errorf("select review card: %w", err)
	}

	schedule, err := review.NextSchedule(reps, prevIntervalDays, rating, now)
	if err != nil {
		return review.GradeResult{}, err
	}
	if schedule.Lapsed {
		lapses++
	}

	// stability holds the current interval in days for FSRS-lite scheduling.
	if _, err := tx.ExecContext(
		ctx,
		`UPDATE review_cards SET
state = ?, due_at = ?, stability = ?, reps = ?, lapses = ?, last_review_at = ?, updated_at = ?
WHERE id = ?`,
		schedule.State, schedule.DueAt, schedule.IntervalDays, schedule.Reps, lapses, now, now, cardID,
	); err != nil {
		return review.GradeResult{}, fmt.Errorf("update review card: %w", err)
	}

	if _, err := tx.ExecContext(
		ctx,
		`INSERT INTO review_logs(id, review_card_id, source, rating, elapsed_ms, reviewed_at)
VALUES (?, ?, ?, ?, ?, ?)`,
		id.New(), cardID, reviewLogSource, rating, nullableInt(elapsedMs), now,
	); err != nil {
		return review.GradeResult{}, fmt.Errorf("insert review log: %w", err)
	}

	// Recompute mastery from the item's full grade history (now including this log)
	// and persist it (PRD §13.2 — "review 완료마다 재계산").
	counts, err := gradeCountsForKnowledgeItem(ctx, tx, knowledgeItemID)
	if err != nil {
		return review.GradeResult{}, err
	}
	mastery := review.MasteryScore(counts)

	if _, err := tx.ExecContext(
		ctx,
		`INSERT INTO learner_items(id, knowledge_item_id, review_count, last_reviewed_at, mastery_score)
VALUES (?, ?, 1, ?, ?)
ON CONFLICT(knowledge_item_id) DO UPDATE SET
  review_count = review_count + 1,
  last_reviewed_at = excluded.last_reviewed_at,
  mastery_score = excluded.mastery_score`,
		id.New(), knowledgeItemID, now, mastery,
	); err != nil {
		return review.GradeResult{}, fmt.Errorf("update learner review stats: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return review.GradeResult{}, fmt.Errorf("commit grade transaction: %w", err)
	}
	return review.GradeResult{
		CardID:       cardID,
		Rating:       rating,
		State:        schedule.State,
		Reps:         schedule.Reps,
		IntervalDays: schedule.IntervalDays,
		DueAt:        schedule.DueAt,
		MasteryScore: mastery,
	}, nil
}

// gradeCountsForKnowledgeItem tallies review_logs by rating across every review card
// of a knowledge item, for mastery recomputation.
func gradeCountsForKnowledgeItem(ctx context.Context, tx *sql.Tx, knowledgeItemID string) (counts review.GradeCounts, resultErr error) {
	rows, err := tx.QueryContext(
		ctx,
		`SELECT rl.rating, count(*)
FROM review_logs rl
JOIN review_cards rc ON rc.id = rl.review_card_id
WHERE rc.knowledge_item_id = ? AND rl.source = ?
GROUP BY rl.rating`,
		knowledgeItemID, reviewLogSource,
	)
	if err != nil {
		return review.GradeCounts{}, fmt.Errorf("aggregate grade counts: %w", err)
	}
	defer func() {
		if err := rows.Close(); err != nil && resultErr == nil {
			resultErr = fmt.Errorf("close grade count rows: %w", err)
		}
	}()

	for rows.Next() {
		var rating string
		var count int
		if err := rows.Scan(&rating, &count); err != nil {
			return review.GradeCounts{}, fmt.Errorf("scan grade count: %w", err)
		}
		switch rating {
		case review.RatingAgain:
			counts.Again = count
		case review.RatingHard:
			counts.Hard = count
		case review.RatingGood:
			counts.Good = count
		case review.RatingEasy:
			counts.Easy = count
		}
	}
	if err := rows.Err(); err != nil {
		return review.GradeCounts{}, fmt.Errorf("iterate grade counts: %w", err)
	}
	return counts, nil
}

func nullableInt(value int) any {
	if value <= 0 {
		return nil
	}
	return value
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
func generateReviewCardsFromCandidates(ctx context.Context, tx *sql.Tx, knowledgeItemID string, now time.Time) (created int, resultErr error) {
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
	defer func() {
		if rows != nil {
			if err := rows.Close(); err != nil && resultErr == nil {
				resultErr = fmt.Errorf("close candidate rows: %w", err)
			}
		}
	}()
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
	rows = nil
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
