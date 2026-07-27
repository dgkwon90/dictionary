package sqlite

import (
	"context"
	"database/sql"
	"errors"
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

// learnerIsMastered is review.IsMastered written as SQL so the due list can push
// well-known items back before LIMIT cuts the list off. The attempt threshold is bound
// from review.MinAttemptsForMastery rather than typed in, and a repository test holds
// this expression to the Go function.
//
// It appears only in ORDER BY — never in WHERE. Filtering mastered cards out would
// make the due list, the due badge and the dashboard disagree, and would strand any
// item that ever reached a perfect record (D6).
const learnerIsMastered = `CASE WHEN COALESCE(li.attempt_count, 0) >= ?
       AND COALESCE(li.correct_count, 0) >= COALESCE(li.attempt_count, 0)
     THEN 1 ELSE 0 END`

func (r *ReviewRepository) DueCards(ctx context.Context, now time.Time, limit int) (cards []review.Card, resultErr error) {
	rows, err := r.db.QueryContext(
		ctx,
		`SELECT rc.id, rc.knowledge_item_id, rc.card_type, rc.question, rc.answer, rc.explanation, rc.state, rc.due_at
FROM review_cards rc
LEFT JOIN learner_items li ON li.knowledge_item_id = rc.knowledge_item_id
WHERE rc.due_at IS NOT NULL
  AND rc.due_at <= ?
  AND `+learnerIsActive+`
ORDER BY `+learnerIsMastered+` ASC, rc.due_at ASC
LIMIT ?`,
		utc(now), review.MinAttemptsForMastery, limit,
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

func (r *ReviewRepository) PracticeCards(ctx context.Context, query string, limit int) (cards []review.Card, resultErr error) {
	sqlQuery := `SELECT rc.id, rc.knowledge_item_id, rc.card_type, rc.question, rc.answer, rc.explanation, rc.state, rc.due_at
FROM review_cards rc`
	args := []any{}
	if query != "" {
		sqlQuery += `
WHERE (rc.question LIKE ? OR rc.answer LIKE ?)`
		likeQuery := "%" + query + "%"
		args = append(args, likeQuery, likeQuery)
	}
	sqlQuery += `
ORDER BY rc.question ASC
LIMIT ?`
	args = append(args, limit)

	rows, err := r.db.QueryContext(ctx, sqlQuery, args...)
	if err != nil {
		return nil, fmt.Errorf("select practice review cards: %w", err)
	}
	defer func() {
		if err := rows.Close(); err != nil && resultErr == nil {
			resultErr = fmt.Errorf("close practice review card rows: %w", err)
		}
	}()

	for rows.Next() {
		var card review.Card
		var explanation sql.NullString
		var dueAt sql.NullTime
		if err := rows.Scan(&card.CardID, &card.KnowledgeItemID, &card.CardType, &card.Question, &card.Answer, &explanation, &card.State, &dueAt); err != nil {
			return nil, fmt.Errorf("scan practice review card: %w", err)
		}
		card.Explanation = explanation.String
		if dueAt.Valid {
			card.DueAt = dueAt.Time
		}
		cards = append(cards, card)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate practice review cards: %w", err)
	}
	return cards, nil
}

// review_logs.source values. The column has a CHECK constraint listing exactly these
// two, so a typo here fails loudly at insert rather than quietly splitting the ledger.
const (
	reviewLogSource   = "review"
	practiceLogSource = "practice"
)

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
		`SELECT rc.reps, rc.lapses, rc.interval_days, rc.knowledge_item_id
FROM review_cards rc
LEFT JOIN learner_items li ON li.knowledge_item_id = rc.knowledge_item_id
WHERE rc.id = ? AND `+learnerIsActive,
		cardID,
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

	if _, err := tx.ExecContext(
		ctx,
		`UPDATE review_cards SET
state = ?, due_at = ?, interval_days = ?, reps = ?, lapses = ?, last_review_at = ?, updated_at = ?
WHERE id = ?`,
		schedule.State, utc(schedule.DueAt), schedule.IntervalDays, schedule.Reps, lapses, utc(now), utc(now), cardID,
	); err != nil {
		return review.GradeResult{}, fmt.Errorf("update review card: %w", err)
	}

	if err := recordAttempt(ctx, tx, cardID, knowledgeItemID, reviewLogSource, rating, elapsedMs, now); err != nil {
		return review.GradeResult{}, err
	}

	accuracy, attempts, correct, err := readAccuracy(ctx, tx, knowledgeItemID)
	if err != nil {
		return review.GradeResult{}, err
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
		Accuracy:     accuracy,
		AttemptCount: attempts,
		CorrectCount: correct,
	}, nil
}

// GradePractice records a practice answer without rescheduling anything.
//
// The card is read only to find out which item the attempt belongs to; review_cards
// is never written. That is the whole point of practice — the user can drill a word
// as often as they like and tomorrow's review list looks exactly the same as if they
// had not.
//
// Unlike Grade there is no learnerIsActive filter, matching PracticeCards: practice
// deliberately reaches items the review rotation has let go, and answering one of them
// should still be counted rather than silently dropped.
func (r *ReviewRepository) GradePractice(ctx context.Context, cardID, rating string, elapsedMs int, now time.Time) (result review.PracticeResult, resultErr error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return review.PracticeResult{}, fmt.Errorf("begin practice grade transaction: %w", err)
	}
	defer func() {
		if resultErr != nil {
			resultErr = errors.Join(resultErr, tx.Rollback())
		}
	}()

	var knowledgeItemID string
	switch err := tx.QueryRowContext(
		ctx,
		`SELECT knowledge_item_id FROM review_cards WHERE id = ?`,
		cardID,
	).Scan(&knowledgeItemID); {
	case errors.Is(err, sql.ErrNoRows):
		return review.PracticeResult{}, review.ErrCardNotFound
	case err != nil:
		return review.PracticeResult{}, fmt.Errorf("select practice card: %w", err)
	}

	if err := recordAttempt(ctx, tx, cardID, knowledgeItemID, practiceLogSource, rating, elapsedMs, now); err != nil {
		return review.PracticeResult{}, err
	}

	accuracy, attempts, correct, err := readAccuracy(ctx, tx, knowledgeItemID)
	if err != nil {
		return review.PracticeResult{}, err
	}

	if err := tx.Commit(); err != nil {
		return review.PracticeResult{}, fmt.Errorf("commit practice grade transaction: %w", err)
	}
	return review.PracticeResult{
		CardID:       cardID,
		Rating:       rating,
		Accuracy:     accuracy,
		AttemptCount: attempts,
		CorrectCount: correct,
	}, nil
}

// recordAttempt appends the append-only log row for one grading and moves the
// learner counters that accuracy is computed from. Practice and review share it:
// both are attempts at recalling the same item, and the user asked for practice
// results to count. What practice does *not* do is touch the card's schedule, so
// that stays with the caller.
func recordAttempt(ctx context.Context, tx *sql.Tx, cardID, knowledgeItemID, source, rating string, elapsedMs int, now time.Time) error {
	correct := 0
	if review.IsCorrect(rating) {
		correct = 1
	}
	if _, err := tx.ExecContext(
		ctx,
		`INSERT INTO review_logs(id, review_card_id, source, rating, is_correct, elapsed_ms, reviewed_at)
VALUES (?, ?, ?, ?, ?, ?, ?)`,
		id.New(), cardID, source, rating, correct, nullableInt(elapsedMs), utc(now),
	); err != nil {
		return fmt.Errorf("insert review log: %w", err)
	}
	// Update-only: grading can only happen on a card, and a card only exists for an
	// item the user already committed to learning, so the learner row must be there.
	if _, err := tx.ExecContext(
		ctx,
		`UPDATE learner_items SET
attempt_count = attempt_count + 1,
correct_count = correct_count + ?,
last_graded_at = ?,
updated_at = ?
WHERE knowledge_item_id = ?`,
		correct, utc(now), utc(now), knowledgeItemID,
	); err != nil {
		return fmt.Errorf("update learner attempt counters: %w", err)
	}
	return nil
}

// readAccuracy returns the item's correct ratio along with the counts it came from.
// Accuracy is derived rather than stored so the ratio can never disagree with the
// counters it is made of.
func readAccuracy(ctx context.Context, tx *sql.Tx, knowledgeItemID string) (float64, int, int, error) {
	var attempts, correct int
	if err := tx.QueryRowContext(
		ctx,
		`SELECT attempt_count, correct_count FROM learner_items WHERE knowledge_item_id = ?`,
		knowledgeItemID,
	).Scan(&attempts, &correct); err != nil {
		return 0, 0, 0, fmt.Errorf("read learner accuracy: %w", err)
	}
	return review.Accuracy(attempts, correct), attempts, correct, nil
}

func nullableInt(value int) any {
	if value <= 0 {
		return nil
	}
	return value
}

// candidateForCard is a review_card_candidate not yet turned into a card.
