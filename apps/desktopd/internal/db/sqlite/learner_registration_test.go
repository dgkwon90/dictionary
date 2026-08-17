package sqlite

import (
	"context"
	"database/sql"
	"testing"
	"time"
)

// These cover registerLearnerItem + promoteCandidatesToCards, the pair that grants
// learning membership and turns stored candidates into review cards.
//
// They used to run through mark-unknown, which is gone: a word now enters the
// learning list through "학습할래요" on its own search. The helpers underneath are the
// same ones, and the duplicate-card guard they protect is a bug that actually shipped
// — every re-search of a word quietly added another identical card to the queue.

// seedWordCaptureForLearning wires up the minimum a word capture needs to be
// registered: the capture, its knowledge item, and the capture_items link that tells
// RegisterWordForLearning which item the search was about.
func seedWordCaptureForLearning(t *testing.T, database *sql.DB, captureID, knowledgeID string) {
	t.Helper()
	ctx := context.Background()
	at := time.Date(2026, 7, 7, 1, 0, 0, 0, time.UTC)
	if _, err := database.ExecContext(ctx,
		`INSERT INTO captures(id, selected_text, input_mode, text_hash, learn_kind, created_at, updated_at)
VALUES (?, ?, 'manual', ?, 'word', ?, ?)`,
		captureID, knowledgeID, captureID+"-hash", at, at,
	); err != nil {
		t.Fatalf("seed capture: %v", err)
	}
	insertKnowledgeItemFixture(t, database, knowledgeID)
	if _, err := database.ExecContext(ctx,
		`INSERT INTO capture_items(id, capture_id, knowledge_item_id, role, confidence, created_at, updated_at)
VALUES (?, ?, ?, 'sub_item', 0.9, ?, ?)`,
		"ci-"+knowledgeID, captureID, knowledgeID, at, at,
	); err != nil {
		t.Fatalf("seed capture item: %v", err)
	}
}

func learnerStatus(t *testing.T, database *sql.DB, knowledgeID string) (status string, registeredAt time.Time) {
	t.Helper()
	if err := database.QueryRowContext(context.Background(),
		`SELECT status, registered_at FROM learner_items WHERE knowledge_item_id = ?`, knowledgeID,
	).Scan(&status, &registeredAt); err != nil {
		t.Fatalf("query learner item: %v", err)
	}
	return status, registeredAt
}

func TestRegisterWordForLearningCreatesLearnerItemAndCards(t *testing.T) {
	database := openMigratedDB(t)
	seedWordCaptureForLearning(t, database, "cap-1", "know-1")
	// Distinct card types: two candidates that agree on (owner, type, context) describe
	// the same card and are deliberately collapsed into one.
	seedCandidateOfType(t, database, "cand-1", "know-1", "meaning")
	seedCandidateOfType(t, database, "cand-2", "know-1", "cloze")
	at := time.Date(2026, 7, 8, 9, 0, 0, 0, time.UTC)

	result, err := NewSearchRepository(database).RegisterWordForLearning(context.Background(), "cap-1", at)
	if err != nil {
		t.Fatalf("RegisterWordForLearning() error = %v", err)
	}
	if result.CardsCreated != 2 {
		t.Fatalf("CardsCreated = %d, want 2", result.CardsCreated)
	}

	status, registeredAt := learnerStatus(t, database, "know-1")
	if status != "active" || !registeredAt.Equal(at) {
		t.Fatalf("learner item status=%q registered_at=%v, want active at %v", status, registeredAt, at)
	}

	var cardCount int
	var state string
	var dueAt time.Time
	if err := database.QueryRowContext(context.Background(),
		`SELECT count(*) FROM review_cards WHERE knowledge_item_id = ?`, "know-1").Scan(&cardCount); err != nil {
		t.Fatalf("count review_cards: %v", err)
	}
	if err := database.QueryRowContext(context.Background(),
		`SELECT state, due_at FROM review_cards WHERE knowledge_item_id = ? LIMIT 1`, "know-1").
		Scan(&state, &dueAt); err != nil {
		t.Fatalf("query review_cards: %v", err)
	}
	// due_at = now: a card the user just committed to is due immediately, not tomorrow.
	if cardCount != 2 || state != "new" || !dueAt.Equal(at) {
		t.Fatalf("cards count=%d state=%q due=%v", cardCount, state, dueAt)
	}

	var unconsumed int
	if err := database.QueryRowContext(context.Background(),
		`SELECT count(*) FROM review_card_candidates WHERE knowledge_item_id = ? AND consumed_at IS NULL`, "know-1").
		Scan(&unconsumed); err != nil {
		t.Fatalf("query unconsumed: %v", err)
	}
	if unconsumed != 0 {
		t.Fatalf("unconsumed candidates = %d, want 0", unconsumed)
	}
}

func TestRegisterWordForLearningIsIdempotentForCards(t *testing.T) {
	database := openMigratedDB(t)
	seedWordCaptureForLearning(t, database, "cap-1", "know-1")
	seedCandidate(t, database, "cand-1", "know-1")
	repo := NewSearchRepository(database)
	ctx := context.Background()

	first, err := repo.RegisterWordForLearning(ctx, "cap-1", time.Now().UTC())
	if err != nil {
		t.Fatalf("first RegisterWordForLearning() error = %v", err)
	}
	second, err := repo.RegisterWordForLearning(ctx, "cap-1", time.Now().UTC())
	if err != nil {
		t.Fatalf("second RegisterWordForLearning() error = %v", err)
	}
	if first.CardsCreated != 1 || second.CardsCreated != 0 {
		t.Fatalf("cards created first=%d second=%d, want 1 then 0", first.CardsCreated, second.CardsCreated)
	}

	var cardCount int
	if err := database.QueryRowContext(ctx,
		`SELECT count(*) FROM review_cards WHERE knowledge_item_id = ?`, "know-1").Scan(&cardCount); err != nil {
		t.Fatalf("query review_cards: %v", err)
	}
	if cardCount != 1 {
		t.Fatalf("review_cards count = %d, want 1 (no duplicate)", cardCount)
	}
}

func TestRegisterWordForLearningConsumesLaterCandidatesWithoutDuplicating(t *testing.T) {
	database := openMigratedDB(t)
	seedWordCaptureForLearning(t, database, "cap-1", "know-1")
	seedCandidate(t, database, "cand-1", "know-1")
	repo := NewSearchRepository(database)
	ctx := context.Background()

	first, err := repo.RegisterWordForLearning(ctx, "cap-1", time.Now().UTC())
	if err != nil {
		t.Fatalf("first RegisterWordForLearning() error = %v", err)
	}

	// Looking the same word up again produces a fresh candidate. It must NOT turn into a
	// second copy of a card the user already has. A candidate that genuinely describes a
	// different card (another type) still becomes one.
	seedCandidateOfType(t, database, "cand-2", "know-1", "meaning")
	seedCandidateOfType(t, database, "cand-3", "know-1", "cloze")

	second, err := repo.RegisterWordForLearning(ctx, "cap-1", time.Now().UTC())
	if err != nil {
		t.Fatalf("second RegisterWordForLearning() error = %v", err)
	}
	if first.CardsCreated != 1 || second.CardsCreated != 1 {
		t.Fatalf("cards created first=%d second=%d, want 1 then 1 (duplicate collapsed, new type added)", first.CardsCreated, second.CardsCreated)
	}

	var cardCount int
	if err := database.QueryRowContext(ctx,
		`SELECT count(*) FROM review_cards WHERE knowledge_item_id = ?`, "know-1").Scan(&cardCount); err != nil {
		t.Fatalf("count review_cards: %v", err)
	}
	if cardCount != 2 {
		t.Fatalf("review_cards count = %d, want 2", cardCount)
	}
}
