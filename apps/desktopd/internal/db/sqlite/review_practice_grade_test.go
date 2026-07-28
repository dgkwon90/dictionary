package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"neulsang/desktopd/internal/domain/review"
)

// cardSchedule is everything practice is forbidden to touch.
type cardSchedule struct {
	state        string
	dueAt        sql.NullTime
	intervalDays float64
	reps         int
	lapses       int
	lastReviewAt sql.NullTime
}

func readCardSchedule(t *testing.T, database *sql.DB, cardID string) cardSchedule {
	t.Helper()
	var got cardSchedule
	if err := database.QueryRowContext(context.Background(),
		`SELECT state, due_at, interval_days, reps, lapses, last_review_at FROM review_cards WHERE id = ?`,
		cardID,
	).Scan(&got.state, &got.dueAt, &got.intervalDays, &got.reps, &got.lapses, &got.lastReviewAt); err != nil {
		t.Fatalf("read card schedule %s: %v", cardID, err)
	}
	return got
}

// The whole reason practice exists as a separate mode (#28) is that the user wanted to
// drill a word without having tomorrow's review pushed a week out. Counting the answer
// while leaving the schedule alone is the deal; this test is the receipt.
func TestGradePracticeCountsWithoutRescheduling(t *testing.T) {
	database := openMigratedDB(t)
	insertKnowledgeItemFixture(t, database, "know-1")
	created := time.Date(2026, 7, 27, 9, 0, 0, 0, time.UTC)
	insertGradableCard(t, database, "card-1", "know-1", 4, 12, created)
	before := readCardSchedule(t, database, "card-1")

	now := time.Date(2026, 7, 27, 12, 0, 0, 0, time.UTC)
	result, err := NewReviewRepository(database).GradePractice(context.Background(), "card-1", review.RatingGood, 2500, now)
	if err != nil {
		t.Fatalf("GradePractice() error = %v", err)
	}

	if after := readCardSchedule(t, database, "card-1"); after != before {
		t.Fatalf("practice moved the schedule: before %+v, after %+v", before, after)
	}
	if result.AttemptCount != 1 || result.CorrectCount != 1 || result.Accuracy != 1 {
		t.Fatalf("result = %+v, want one correct attempt at 100%%", result)
	}

	var source string
	var isCorrect int
	var elapsed sql.NullInt64
	if err := database.QueryRowContext(context.Background(),
		`SELECT source, is_correct, elapsed_ms FROM review_logs WHERE review_card_id = 'card-1'`,
	).Scan(&source, &isCorrect, &elapsed); err != nil {
		t.Fatalf("read review log: %v", err)
	}
	if source != "practice" || isCorrect != 1 || elapsed.Int64 != 2500 {
		t.Fatalf("log source=%q is_correct=%d elapsed=%v, want a correct practice entry", source, isCorrect, elapsed)
	}

	var attempts, correct int
	if err := database.QueryRowContext(context.Background(),
		`SELECT attempt_count, correct_count FROM learner_items WHERE knowledge_item_id = 'know-1'`,
	).Scan(&attempts, &correct); err != nil {
		t.Fatalf("read learner counters: %v", err)
	}
	if attempts != 1 || correct != 1 {
		t.Fatalf("learner counters = %d/%d, want 1/1 — practice answers count toward accuracy", correct, attempts)
	}
}

// again is the only wrong answer (D5), in practice exactly as in review.
func TestGradePracticeAgainCountsWrong(t *testing.T) {
	database := openMigratedDB(t)
	insertKnowledgeItemFixture(t, database, "know-1")
	created := time.Date(2026, 7, 27, 9, 0, 0, 0, time.UTC)
	insertGradableCard(t, database, "card-1", "know-1", 0, 0, created)
	repo := NewReviewRepository(database)
	now := time.Date(2026, 7, 27, 12, 0, 0, 0, time.UTC)

	if _, err := repo.GradePractice(context.Background(), "card-1", review.RatingHard, 0, now); err != nil {
		t.Fatalf("GradePractice(hard) error = %v", err)
	}
	result, err := repo.GradePractice(context.Background(), "card-1", review.RatingAgain, 0, now.Add(time.Minute))
	if err != nil {
		t.Fatalf("GradePractice(again) error = %v", err)
	}

	if result.AttemptCount != 2 || result.CorrectCount != 1 || result.Accuracy != 0.5 {
		t.Fatalf("result = %+v, want hard counted right and again counted wrong", result)
	}
}

// Practice and review write to the same counters, so an item's accuracy is its whole
// history rather than one mode's slice of it.
func TestGradePracticeAndReviewShareAccuracy(t *testing.T) {
	database := openMigratedDB(t)
	insertKnowledgeItemFixture(t, database, "know-1")
	created := time.Date(2026, 7, 27, 9, 0, 0, 0, time.UTC)
	insertGradableCard(t, database, "card-1", "know-1", 0, 0, created)
	repo := NewReviewRepository(database)
	now := time.Date(2026, 7, 27, 12, 0, 0, 0, time.UTC)

	if _, err := repo.Grade(context.Background(), "card-1", review.RatingAgain, 0, now, review.DefaultIntervals()); err != nil {
		t.Fatalf("Grade() error = %v", err)
	}
	result, err := repo.GradePractice(context.Background(), "card-1", review.RatingGood, 0, now.Add(time.Hour))
	if err != nil {
		t.Fatalf("GradePractice() error = %v", err)
	}
	if result.AttemptCount != 2 || result.CorrectCount != 1 {
		t.Fatalf("result = %+v, want the review miss and the practice hit in one tally", result)
	}
}

// PracticeCards deliberately reaches items the review rotation has let go, so grading
// one of them must be recorded rather than rejected. It stays retired either way —
// answering a card is not a request to put the word back on the list.
func TestGradePracticeAcceptsRetiredItem(t *testing.T) {
	database := openMigratedDB(t)
	insertKnowledgeItemFixture(t, database, "know-1")
	created := time.Date(2026, 7, 27, 9, 0, 0, 0, time.UTC)
	insertGradableCard(t, database, "card-1", "know-1", 0, 0, created)
	if _, err := database.ExecContext(context.Background(),
		`UPDATE learner_items SET status = 'known' WHERE knowledge_item_id = 'know-1'`,
	); err != nil {
		t.Fatalf("retire learner item: %v", err)
	}
	now := time.Date(2026, 7, 27, 12, 0, 0, 0, time.UTC)

	result, err := NewReviewRepository(database).GradePractice(context.Background(), "card-1", review.RatingGood, 0, now)
	if err != nil {
		t.Fatalf("GradePractice() on a retired item error = %v", err)
	}
	if result.AttemptCount != 1 {
		t.Fatalf("result = %+v, want the attempt recorded", result)
	}

	var status string
	if err := database.QueryRowContext(context.Background(),
		`SELECT status FROM learner_items WHERE knowledge_item_id = 'know-1'`,
	).Scan(&status); err != nil {
		t.Fatalf("read learner status: %v", err)
	}
	if status != "known" {
		t.Fatalf("status = %q, want practice to leave the retirement alone", status)
	}
}

func TestGradePracticeMissingCard(t *testing.T) {
	database := openMigratedDB(t)
	now := time.Date(2026, 7, 27, 12, 0, 0, 0, time.UTC)

	_, err := NewReviewRepository(database).GradePractice(context.Background(), "nope", review.RatingGood, 0, now)
	if !errors.Is(err, review.ErrCardNotFound) {
		t.Fatalf("GradePractice() error = %v, want ErrCardNotFound", err)
	}
}
