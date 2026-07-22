package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"neulsang/desktopd/internal/domain/review"
)

func insertGradableCard(t *testing.T, database *sql.DB, cardID, knowledgeID string, reps int, stability float64, createdAt time.Time) {
	t.Helper()
	if _, err := database.ExecContext(context.Background(),
		`INSERT INTO review_cards(id, knowledge_item_id, card_type, question, answer, state, due_at, stability, reps, created_at, updated_at)
VALUES (?, ?, 'meaning', 'q', 'a', ?, ?, ?, ?, ?, ?)`,
		cardID, knowledgeID, review.CardStateNew, createdAt, stability, reps, createdAt, createdAt); err != nil {
		t.Fatalf("insert card %s: %v", cardID, err)
	}
}

func insertPracticeCard(t *testing.T, database *sql.DB, cardID, knowledgeID, question, answer string, dueAt time.Time) {
	t.Helper()
	if _, err := database.ExecContext(context.Background(),
		`INSERT INTO review_cards(id, knowledge_item_id, card_type, question, answer, state, due_at, created_at, updated_at)
VALUES (?, ?, 'meaning', ?, ?, ?, ?, ?, ?)`,
		cardID, knowledgeID, question, answer, review.CardStateReview, dueAt, dueAt, dueAt); err != nil {
		t.Fatalf("insert practice card %s: %v", cardID, err)
	}
}

func TestReviewRepositoryGradeFirstReview(t *testing.T) {
	database := openMigratedDB(t)
	insertKnowledgeItemFixture(t, database, "know-1")
	created := time.Date(2026, 7, 8, 12, 0, 0, 0, time.UTC)
	insertGradableCard(t, database, "card-1", "know-1", 0, 0, created)
	repo := NewReviewRepository(database)
	now := time.Date(2026, 7, 8, 12, 30, 0, 0, time.UTC)

	result, err := repo.Grade(context.Background(), "card-1", review.RatingGood, 3200, now)
	if err != nil {
		t.Fatalf("Grade() error = %v", err)
	}
	if result.Reps != 1 || result.State != review.CardStateReview {
		t.Fatalf("result = %#v", result)
	}
	// Good on a first review → 3 days out.
	if !result.DueAt.Equal(now.Add(3 * 24 * time.Hour)) {
		t.Fatalf("dueAt = %v, want +3d", result.DueAt)
	}

	var reps int
	var stability float64
	var state string
	var lastReview sql.NullTime
	if err := database.QueryRowContext(context.Background(),
		`SELECT reps, stability, state, last_review_at FROM review_cards WHERE id = ?`, "card-1").
		Scan(&reps, &stability, &state, &lastReview); err != nil {
		t.Fatalf("query card: %v", err)
	}
	if reps != 1 || stability != 3.0 || state != review.CardStateReview || !lastReview.Valid {
		t.Fatalf("card reps=%d stability=%v state=%q lastReview=%v", reps, stability, state, lastReview)
	}

	var logCount int
	var rating string
	var elapsed sql.NullInt64
	if err := database.QueryRowContext(context.Background(),
		`SELECT count(*), max(rating), max(elapsed_ms) FROM review_logs WHERE review_card_id = ?`, "card-1").
		Scan(&logCount, &rating, &elapsed); err != nil {
		t.Fatalf("query logs: %v", err)
	}
	if logCount != 1 || rating != review.RatingGood || !elapsed.Valid || elapsed.Int64 != 3200 {
		t.Fatalf("log count=%d rating=%q elapsed=%v", logCount, rating, elapsed)
	}

	var reviewCount int
	var mastery float64
	if err := database.QueryRowContext(context.Background(),
		`SELECT review_count, mastery_score FROM learner_items WHERE knowledge_item_id = ?`, "know-1").Scan(&reviewCount, &mastery); err != nil {
		t.Fatalf("query learner: %v", err)
	}
	// one "good" → mastery 0.2 (§13.2)
	if reviewCount != 1 || mastery != 0.2 || result.MasteryScore != 0.2 {
		t.Fatalf("review_count=%d mastery=%v result.mastery=%v", reviewCount, mastery, result.MasteryScore)
	}
}

func TestReviewRepositoryGradeRecomputesMastery(t *testing.T) {
	database := openMigratedDB(t)
	insertKnowledgeItemFixture(t, database, "know-1")
	created := time.Date(2026, 7, 8, 12, 0, 0, 0, time.UTC)
	insertGradableCard(t, database, "card-1", "know-1", 0, 0, created)
	repo := NewReviewRepository(database)
	now := created.Add(time.Hour)

	// good, good, easy → 0.2 + 0.2 + 0.3 = 0.7 (mastery aggregates all logs of the item)
	for _, rating := range []string{review.RatingGood, review.RatingGood, review.RatingEasy} {
		if _, err := repo.Grade(context.Background(), "card-1", rating, 0, now); err != nil {
			t.Fatalf("Grade(%s) error = %v", rating, err)
		}
	}

	var mastery float64
	if err := database.QueryRowContext(context.Background(),
		`SELECT mastery_score FROM learner_items WHERE knowledge_item_id = ?`, "know-1").Scan(&mastery); err != nil {
		t.Fatalf("query mastery: %v", err)
	}
	if !approxFloat(mastery, 0.7) {
		t.Fatalf("mastery = %v, want 0.7", mastery)
	}
}

func TestReviewRepositoryGradeMasteryAcrossMultipleCards(t *testing.T) {
	database := openMigratedDB(t)
	insertKnowledgeItemFixture(t, database, "know-1")
	created := time.Date(2026, 7, 8, 12, 0, 0, 0, time.UTC)
	// two cards for the same knowledge item (e.g. meaning + reverse)
	insertGradableCard(t, database, "card-1", "know-1", 0, 0, created)
	insertGradableCard(t, database, "card-2", "know-1", 0, 0, created)
	repo := NewReviewRepository(database)
	now := created.Add(time.Hour)

	// good on card-1, easy on card-2 → mastery aggregates both cards: 0.2 + 0.3 = 0.5
	if _, err := repo.Grade(context.Background(), "card-1", review.RatingGood, 0, now); err != nil {
		t.Fatalf("grade card-1: %v", err)
	}
	result, err := repo.Grade(context.Background(), "card-2", review.RatingEasy, 0, now)
	if err != nil {
		t.Fatalf("grade card-2: %v", err)
	}
	if !approxFloat(result.MasteryScore, 0.5) {
		t.Fatalf("mastery across cards = %v, want 0.5", result.MasteryScore)
	}
}

func approxFloat(a, b float64) bool {
	d := a - b
	if d < 0 {
		d = -d
	}
	return d < 1e-9
}

func TestReviewRepositoryGradeAgainLapses(t *testing.T) {
	database := openMigratedDB(t)
	insertKnowledgeItemFixture(t, database, "know-1")
	created := time.Date(2026, 7, 8, 12, 0, 0, 0, time.UTC)
	// mature card: reps 3, 30-day interval
	insertGradableCard(t, database, "card-1", "know-1", 3, 30, created)
	repo := NewReviewRepository(database)
	now := time.Date(2026, 7, 20, 12, 0, 0, 0, time.UTC)

	result, err := repo.Grade(context.Background(), "card-1", review.RatingAgain, 0, now)
	if err != nil {
		t.Fatalf("Grade() error = %v", err)
	}
	if result.Reps != 0 || result.State != review.CardStateLearning {
		t.Fatalf("result = %#v", result)
	}

	var reps, lapses int
	if err := database.QueryRowContext(context.Background(),
		`SELECT reps, lapses FROM review_cards WHERE id = ?`, "card-1").Scan(&reps, &lapses); err != nil {
		t.Fatalf("query card: %v", err)
	}
	if reps != 0 || lapses != 1 {
		t.Fatalf("reps=%d lapses=%d, want 0/1", reps, lapses)
	}
	// elapsed_ms 0 must be stored as NULL, not 0.
	var elapsed sql.NullInt64
	if err := database.QueryRowContext(context.Background(),
		`SELECT elapsed_ms FROM review_logs WHERE review_card_id = ?`, "card-1").Scan(&elapsed); err != nil {
		t.Fatalf("query log: %v", err)
	}
	if elapsed.Valid {
		t.Fatalf("elapsed_ms = %v, want NULL for 0", elapsed)
	}
}

func TestReviewRepositoryGradeNotFound(t *testing.T) {
	database := openMigratedDB(t)
	repo := NewReviewRepository(database)
	_, err := repo.Grade(context.Background(), "missing", review.RatingGood, 0, time.Now().UTC())
	if !errors.Is(err, review.ErrCardNotFound) {
		t.Fatalf("Grade() error = %v, want ErrCardNotFound", err)
	}
}

func TestReviewRepositoryGradeKnownCardNotFound(t *testing.T) {
	database := openMigratedDB(t)
	insertKnowledgeItemFixture(t, database, "know-1")
	created := time.Date(2026, 7, 8, 12, 0, 0, 0, time.UTC)
	insertGradableCard(t, database, "card-1", "know-1", 0, 0, created)
	if _, err := database.ExecContext(context.Background(),
		`INSERT INTO learner_items(id, knowledge_item_id, status) VALUES ('learn-1', 'know-1', 'known')`); err != nil {
		t.Fatalf("insert known learner item: %v", err)
	}
	repo := NewReviewRepository(database)

	_, err := repo.Grade(context.Background(), "card-1", review.RatingGood, 0, created.Add(time.Hour))
	if !errors.Is(err, review.ErrCardNotFound) {
		t.Fatalf("Grade() error = %v, want ErrCardNotFound for known item", err)
	}
}

func TestReviewRepositoryDueCards(t *testing.T) {
	database := openMigratedDB(t)
	insertKnowledgeItemFixture(t, database, "know-1")
	now := time.Date(2026, 7, 8, 12, 0, 0, 0, time.UTC)

	// due yesterday (should appear), due tomorrow (should not), NULL due (should not)
	cards := []struct {
		id    string
		state string
		due   any
	}{
		{"card-due", review.CardStateNew, now.Add(-24 * time.Hour)},
		{"card-future", review.CardStateNew, now.Add(24 * time.Hour)},
		{"card-null", review.CardStateNew, nil},
	}
	for _, c := range cards {
		if _, err := database.ExecContext(context.Background(),
			`INSERT INTO review_cards(id, knowledge_item_id, card_type, question, answer, state, due_at, created_at, updated_at)
VALUES (?, 'know-1', 'meaning', 'q', 'a', ?, ?, ?, ?)`,
			c.id, c.state, c.due, now, now); err != nil {
			t.Fatalf("insert card %s: %v", c.id, err)
		}
	}

	repo := NewReviewRepository(database)
	got, err := repo.DueCards(context.Background(), now, 50)
	if err != nil {
		t.Fatalf("DueCards() error = %v", err)
	}
	if len(got) != 1 || got[0].CardID != "card-due" {
		t.Fatalf("DueCards() = %#v, want only card-due", got)
	}
	if got[0].KnowledgeItemID != "know-1" || got[0].CardType != "meaning" || got[0].Question != "q" || got[0].Answer != "a" || got[0].State != review.CardStateNew {
		t.Fatalf("card fields = %#v", got[0])
	}
}

func TestReviewRepositoryDueCardsExcludesKnownItems(t *testing.T) {
	database := openMigratedDB(t)
	insertKnowledgeItemFixture(t, database, "know-1")
	now := time.Date(2026, 7, 8, 12, 0, 0, 0, time.UTC)
	if _, err := database.ExecContext(context.Background(),
		`INSERT INTO learner_items(id, knowledge_item_id, status) VALUES ('learn-1', 'know-1', 'known')`); err != nil {
		t.Fatalf("insert known learner item: %v", err)
	}
	if _, err := database.ExecContext(context.Background(),
		`INSERT INTO review_cards(id, knowledge_item_id, card_type, question, answer, state, due_at, created_at, updated_at)
VALUES ('card-known', 'know-1', 'meaning', 'q', 'a', 'new', ?, ?, ?)`,
		now.Add(-time.Hour), now, now); err != nil {
		t.Fatalf("insert known card: %v", err)
	}

	repo := NewReviewRepository(database)
	got, err := repo.DueCards(context.Background(), now, 50)
	if err != nil {
		t.Fatalf("DueCards() error = %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("DueCards() = %#v, want no cards for known item", got)
	}
}

func TestReviewRepositoryDueCardsOrdersBySoonest(t *testing.T) {
	database := openMigratedDB(t)
	insertKnowledgeItemFixture(t, database, "know-1")
	now := time.Date(2026, 7, 8, 12, 0, 0, 0, time.UTC)

	for _, c := range []struct {
		id  string
		due time.Time
	}{
		{"card-newer", now.Add(-1 * time.Hour)},
		{"card-older", now.Add(-10 * time.Hour)},
	} {
		if _, err := database.ExecContext(context.Background(),
			`INSERT INTO review_cards(id, knowledge_item_id, card_type, question, answer, state, due_at, created_at, updated_at)
VALUES (?, 'know-1', 'meaning', 'q', 'a', 'new', ?, ?, ?)`,
			c.id, c.due, now, now); err != nil {
			t.Fatalf("insert card %s: %v", c.id, err)
		}
	}

	repo := NewReviewRepository(database)
	got, err := repo.DueCards(context.Background(), now, 50)
	if err != nil {
		t.Fatalf("DueCards() error = %v", err)
	}
	if len(got) != 2 || got[0].CardID != "card-older" || got[1].CardID != "card-newer" {
		t.Fatalf("DueCards() order = %#v, want card-older then card-newer", got)
	}
}

func TestReviewRepositoryPracticeCardsIgnoresDueAndKnownStatus(t *testing.T) {
	database := openMigratedDB(t)
	insertKnowledgeItemFixture(t, database, "know-1")
	insertKnowledgeItemFixture(t, database, "know-2")
	now := time.Date(2026, 7, 8, 12, 0, 0, 0, time.UTC)
	if _, err := database.ExecContext(context.Background(),
		`INSERT INTO learner_items(id, knowledge_item_id, status) VALUES ('learn-2', 'know-2', 'known')`); err != nil {
		t.Fatalf("insert known learner item: %v", err)
	}
	insertPracticeCard(t, database, "card-future", "know-1", "alpha future", "answer", now.Add(24*time.Hour))
	insertPracticeCard(t, database, "card-known", "know-2", "beta known", "answer", now.Add(48*time.Hour))

	repo := NewReviewRepository(database)
	got, err := repo.PracticeCards(context.Background(), "", 50)
	if err != nil {
		t.Fatalf("PracticeCards() error = %v", err)
	}
	if len(got) != 2 || got[0].CardID != "card-future" || got[1].CardID != "card-known" {
		t.Fatalf("PracticeCards() = %#v, want future and known cards ordered by question", got)
	}
}

func TestReviewRepositoryPracticeCardsFiltersByQuery(t *testing.T) {
	database := openMigratedDB(t)
	insertKnowledgeItemFixture(t, database, "know-1")
	now := time.Date(2026, 7, 8, 12, 0, 0, 0, time.UTC)
	insertPracticeCard(t, database, "card-question", "know-1", "stale cache", "answer", now)
	insertPracticeCard(t, database, "card-answer", "know-1", "fresh cache", "contains stale value", now)
	insertPracticeCard(t, database, "card-other", "know-1", "fresh fruit", "answer", now)

	repo := NewReviewRepository(database)
	got, err := repo.PracticeCards(context.Background(), "stale", 50)
	if err != nil {
		t.Fatalf("PracticeCards() error = %v", err)
	}
	if len(got) != 2 || got[0].CardID != "card-answer" || got[1].CardID != "card-question" {
		t.Fatalf("PracticeCards(query) = %#v, want answer and question matches ordered by question", got)
	}
}

type practiceMutationSnapshot struct {
	reviewLogCount   int
	reviewCardCount  int
	learnerItemCount int
	cardState        string
	cardDueAtUnix    int64
	cardStability    float64
	cardReps         int
	cardLapses       int
	learnerStatus    string
	learnerReviews   int
	learnerMastery   float64
}

func snapshotPracticeMutationState(t *testing.T, database *sql.DB, cardID, knowledgeID string) practiceMutationSnapshot {
	t.Helper()
	var snapshot practiceMutationSnapshot
	if err := database.QueryRowContext(context.Background(), `SELECT count(*) FROM review_logs`).Scan(&snapshot.reviewLogCount); err != nil {
		t.Fatalf("count review_logs: %v", err)
	}
	if err := database.QueryRowContext(context.Background(), `SELECT count(*) FROM review_cards`).Scan(&snapshot.reviewCardCount); err != nil {
		t.Fatalf("count review_cards: %v", err)
	}
	if err := database.QueryRowContext(context.Background(), `SELECT count(*) FROM learner_items`).Scan(&snapshot.learnerItemCount); err != nil {
		t.Fatalf("count learner_items: %v", err)
	}

	var dueAt time.Time
	if err := database.QueryRowContext(context.Background(),
		`SELECT state, due_at, stability, reps, lapses FROM review_cards WHERE id = ?`, cardID).
		Scan(&snapshot.cardState, &dueAt, &snapshot.cardStability, &snapshot.cardReps, &snapshot.cardLapses); err != nil {
		t.Fatalf("query review card state: %v", err)
	}
	snapshot.cardDueAtUnix = dueAt.UnixNano()

	if err := database.QueryRowContext(context.Background(),
		`SELECT status, review_count, mastery_score FROM learner_items WHERE knowledge_item_id = ?`, knowledgeID).
		Scan(&snapshot.learnerStatus, &snapshot.learnerReviews, &snapshot.learnerMastery); err != nil {
		t.Fatalf("query learner item state: %v", err)
	}
	return snapshot
}

func TestReviewRepositoryPracticeCardsDoesNotMutateReviewState(t *testing.T) {
	database := openMigratedDB(t)
	insertKnowledgeItemFixture(t, database, "know-1")
	now := time.Date(2026, 7, 8, 12, 0, 0, 0, time.UTC)
	if _, err := database.ExecContext(context.Background(),
		`INSERT INTO learner_items(id, knowledge_item_id, status, review_count, mastery_score)
VALUES ('learn-1', 'know-1', 'active', 3, 0.4)`); err != nil {
		t.Fatalf("insert learner item: %v", err)
	}
	if _, err := database.ExecContext(context.Background(),
		`INSERT INTO review_cards(id, knowledge_item_id, card_type, question, answer, state, due_at, stability, reps, lapses, created_at, updated_at)
VALUES ('card-1', 'know-1', 'meaning', 'practice query', 'answer', ?, ?, 2.5, 3, 1, ?, ?)`,
		review.CardStateReview, now.Add(7*24*time.Hour), now, now); err != nil {
		t.Fatalf("insert review card: %v", err)
	}
	if _, err := database.ExecContext(context.Background(),
		`INSERT INTO review_logs(id, review_card_id, source, rating, elapsed_ms, reviewed_at)
VALUES ('log-1', 'card-1', 'review', 'good', 1000, ?)`, now); err != nil {
		t.Fatalf("insert review log: %v", err)
	}

	before := snapshotPracticeMutationState(t, database, "card-1", "know-1")
	repo := NewReviewRepository(database)
	got, err := repo.PracticeCards(context.Background(), "practice", 50)
	if err != nil {
		t.Fatalf("PracticeCards() error = %v", err)
	}
	if len(got) != 1 || got[0].CardID != "card-1" {
		t.Fatalf("PracticeCards() = %#v, want card-1", got)
	}
	after := snapshotPracticeMutationState(t, database, "card-1", "know-1")
	if after != before {
		t.Fatalf("PracticeCards() mutated review state: before=%#v after=%#v", before, after)
	}
}
