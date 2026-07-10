package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"neulsang/desktopd/internal/domain/knowledge"
)

func insertKnowledgeItemFixture(t *testing.T, database *sql.DB, knowledgeID string) {
	t.Helper()
	at := time.Date(2026, 7, 7, 1, 0, 0, 0, time.UTC)
	if _, err := database.ExecContext(
		context.Background(),
		`INSERT INTO knowledge_items(id, normalized_key, surface_text, item_type, language, first_seen_at, last_seen_at)
VALUES (?, ?, ?, 'word', 'en', ?, ?)`,
		knowledgeID, knowledgeID+"-key", knowledgeID, at, at,
	); err != nil {
		t.Fatalf("insert knowledge item fixture: %v", err)
	}
}

func TestKnowledgeRepositoryMarkUnknown(t *testing.T) {
	database := openMigratedDB(t)
	insertKnowledgeItemFixture(t, database, "know-1")
	// seed an existing learner_item (ask_count from a prior lookup) and a candidate
	if _, err := database.ExecContext(context.Background(),
		`INSERT INTO learner_items(id, knowledge_item_id, ask_count) VALUES ('learn-1', 'know-1', 2)`); err != nil {
		t.Fatalf("seed learner item: %v", err)
	}
	if _, err := database.ExecContext(context.Background(),
		`INSERT INTO captures(id, selected_text, input_mode, text_hash, created_at) VALUES ('cap-1','hi','manual','h',?)`,
		time.Now().UTC()); err != nil {
		t.Fatalf("seed capture: %v", err)
	}
	if _, err := database.ExecContext(context.Background(),
		`INSERT INTO review_card_candidates(id, capture_id, knowledge_item_id, card_type, question, answer, created_at)
VALUES ('cand-1','cap-1','know-1','meaning','q','a',?)`, time.Now().UTC()); err != nil {
		t.Fatalf("seed candidate: %v", err)
	}
	repo := NewKnowledgeRepository(database)
	at := time.Date(2026, 7, 8, 9, 0, 0, 0, time.UTC)

	result, err := repo.MarkUnknown(context.Background(), "know-1", at)
	if err != nil {
		t.Fatalf("MarkUnknown() error = %v", err)
	}
	if result.KnowledgeItemID != "know-1" || result.Status != knowledge.StatusActive || result.WrongCount != 1 || result.AskCount != 2 || result.CandidateCount != 1 {
		t.Fatalf("result = %#v", result)
	}

	var wrongCount int
	var lastWrong sql.NullTime
	var status string
	if err := database.QueryRowContext(context.Background(),
		`SELECT wrong_count, last_wrong_at, status FROM learner_items WHERE knowledge_item_id = ?`, "know-1").
		Scan(&wrongCount, &lastWrong, &status); err != nil {
		t.Fatalf("query learner item: %v", err)
	}
	if wrongCount != 1 || !lastWrong.Valid || !lastWrong.Time.Equal(at) || status != knowledge.StatusActive {
		t.Fatalf("learner row wrong=%d lastWrong=%#v status=%q", wrongCount, lastWrong, status)
	}
}

func seedCandidate(t *testing.T, database *sql.DB, candidateID, knowledgeID string) {
	t.Helper()
	if _, err := database.ExecContext(context.Background(),
		`INSERT INTO review_card_candidates(id, capture_id, knowledge_item_id, card_type, question, answer, created_at)
VALUES (?, 'cap-1', ?, 'meaning', 'q-'||?, 'a-'||?, ?)`,
		candidateID, knowledgeID, candidateID, candidateID, time.Now().UTC()); err != nil {
		t.Fatalf("seed candidate %s: %v", candidateID, err)
	}
}

func TestKnowledgeRepositoryMarkUnknownGeneratesCards(t *testing.T) {
	database := openMigratedDB(t)
	insertKnowledgeItemFixture(t, database, "know-1")
	if _, err := database.ExecContext(context.Background(),
		`INSERT INTO captures(id, selected_text, input_mode, text_hash, created_at) VALUES ('cap-1','hi','manual','h',?)`,
		time.Now().UTC()); err != nil {
		t.Fatalf("seed capture: %v", err)
	}
	seedCandidate(t, database, "cand-1", "know-1")
	seedCandidate(t, database, "cand-2", "know-1")
	repo := NewKnowledgeRepository(database)
	at := time.Date(2026, 7, 8, 9, 0, 0, 0, time.UTC)

	result, err := repo.MarkUnknown(context.Background(), "know-1", at)
	if err != nil {
		t.Fatalf("MarkUnknown() error = %v", err)
	}
	if result.CardsCreated != 2 {
		t.Fatalf("CardsCreated = %d, want 2", result.CardsCreated)
	}

	var cardCount int
	if err := database.QueryRowContext(context.Background(),
		`SELECT count(*) FROM review_cards WHERE knowledge_item_id = ?`, "know-1").Scan(&cardCount); err != nil {
		t.Fatalf("count review_cards: %v", err)
	}
	var state string
	var dueAt time.Time
	if err := database.QueryRowContext(context.Background(),
		`SELECT state, due_at FROM review_cards WHERE knowledge_item_id = ? LIMIT 1`, "know-1").
		Scan(&state, &dueAt); err != nil {
		t.Fatalf("query review_cards: %v", err)
	}
	if cardCount != 2 || state != "new" || !dueAt.Equal(at) {
		t.Fatalf("cards count=%d state=%q due=%v", cardCount, state, dueAt)
	}

	// All candidates must be marked consumed.
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

func TestKnowledgeRepositoryMarkUnknownIsIdempotentForCards(t *testing.T) {
	database := openMigratedDB(t)
	insertKnowledgeItemFixture(t, database, "know-1")
	if _, err := database.ExecContext(context.Background(),
		`INSERT INTO captures(id, selected_text, input_mode, text_hash, created_at) VALUES ('cap-1','hi','manual','h',?)`,
		time.Now().UTC()); err != nil {
		t.Fatalf("seed capture: %v", err)
	}
	seedCandidate(t, database, "cand-1", "know-1")
	repo := NewKnowledgeRepository(database)

	first, err := repo.MarkUnknown(context.Background(), "know-1", time.Now().UTC())
	if err != nil {
		t.Fatalf("first MarkUnknown() error = %v", err)
	}
	second, err := repo.MarkUnknown(context.Background(), "know-1", time.Now().UTC())
	if err != nil {
		t.Fatalf("second MarkUnknown() error = %v", err)
	}
	if first.CardsCreated != 1 || second.CardsCreated != 0 {
		t.Fatalf("cards created first=%d second=%d, want 1 then 0", first.CardsCreated, second.CardsCreated)
	}

	var cardCount int
	if err := database.QueryRowContext(context.Background(),
		`SELECT count(*) FROM review_cards WHERE knowledge_item_id = ?`, "know-1").Scan(&cardCount); err != nil {
		t.Fatalf("query review_cards: %v", err)
	}
	if cardCount != 1 {
		t.Fatalf("review_cards count = %d, want 1 (no duplicate)", cardCount)
	}
	// second mark-unknown still bumps wrong_count
	if second.WrongCount != 2 {
		t.Fatalf("wrong_count = %d, want 2", second.WrongCount)
	}
}

func TestKnowledgeRepositoryMarkUnknownConsumesLaterCandidates(t *testing.T) {
	database := openMigratedDB(t)
	insertKnowledgeItemFixture(t, database, "know-1")
	if _, err := database.ExecContext(context.Background(),
		`INSERT INTO captures(id, selected_text, input_mode, text_hash, created_at) VALUES ('cap-1','hi','manual','h',?)`,
		time.Now().UTC()); err != nil {
		t.Fatalf("seed capture: %v", err)
	}
	seedCandidate(t, database, "cand-1", "know-1")
	repo := NewKnowledgeRepository(database)

	first, err := repo.MarkUnknown(context.Background(), "know-1", time.Now().UTC())
	if err != nil {
		t.Fatalf("first MarkUnknown() error = %v", err)
	}

	// A later capture of the same word contributes a fresh (unconsumed) candidate.
	seedCandidate(t, database, "cand-2", "know-1")

	second, err := repo.MarkUnknown(context.Background(), "know-1", time.Now().UTC())
	if err != nil {
		t.Fatalf("second MarkUnknown() error = %v", err)
	}
	if first.CardsCreated != 1 || second.CardsCreated != 1 {
		t.Fatalf("cards created first=%d second=%d, want 1 then 1", first.CardsCreated, second.CardsCreated)
	}

	var cardCount int
	if err := database.QueryRowContext(context.Background(),
		`SELECT count(*) FROM review_cards WHERE knowledge_item_id = ?`, "know-1").Scan(&cardCount); err != nil {
		t.Fatalf("count review_cards: %v", err)
	}
	if cardCount != 2 {
		t.Fatalf("review_cards count = %d, want 2", cardCount)
	}
}

func TestKnowledgeRepositoryMarkUnknownCreatesLearnerItem(t *testing.T) {
	database := openMigratedDB(t)
	insertKnowledgeItemFixture(t, database, "know-1")
	repo := NewKnowledgeRepository(database)

	result, err := repo.MarkUnknown(context.Background(), "know-1", time.Now().UTC())
	if err != nil {
		t.Fatalf("MarkUnknown() error = %v", err)
	}
	if result.WrongCount != 1 || result.Status != knowledge.StatusActive {
		t.Fatalf("result = %#v", result)
	}
}

func TestKnowledgeRepositoryMarkKnown(t *testing.T) {
	database := openMigratedDB(t)
	insertKnowledgeItemFixture(t, database, "know-1")
	repo := NewKnowledgeRepository(database)

	result, err := repo.MarkKnown(context.Background(), "know-1", time.Now().UTC())
	if err != nil {
		t.Fatalf("MarkKnown() error = %v", err)
	}
	if result.Status != knowledge.StatusKnown {
		t.Fatalf("result status = %q, want known", result.Status)
	}

	var status string
	if err := database.QueryRowContext(context.Background(),
		`SELECT status FROM learner_items WHERE knowledge_item_id = ?`, "know-1").Scan(&status); err != nil {
		t.Fatalf("query learner item: %v", err)
	}
	if status != knowledge.StatusKnown {
		t.Fatalf("status = %q, want known", status)
	}
}

func TestKnowledgeRepositoryMarkUnknownNotFound(t *testing.T) {
	database := openMigratedDB(t)
	repo := NewKnowledgeRepository(database)

	_, err := repo.MarkUnknown(context.Background(), "missing", time.Now().UTC())
	if !errors.Is(err, knowledge.ErrKnowledgeItemNotFound) {
		t.Fatalf("MarkUnknown() error = %v, want ErrKnowledgeItemNotFound", err)
	}
}

func TestKnowledgeRepositoryListByCapture(t *testing.T) {
	database := openMigratedDB(t)
	ctx := context.Background()
	now := time.Now().UTC()
	if _, err := database.ExecContext(ctx,
		`INSERT INTO captures(id, selected_text, input_mode, text_hash, created_at) VALUES ('cap-1','hi','manual','h',?)`,
		now); err != nil {
		t.Fatalf("seed capture: %v", err)
	}
	insertKnowledgeItemFixture(t, database, "know-1")
	insertKnowledgeItemFixture(t, database, "know-2")
	// link both to the capture with different confidence (ordering) and set meaning.
	if _, err := database.ExecContext(ctx,
		`UPDATE knowledge_items SET meaning_ko='오래된', pronunciation='스테일' WHERE id='know-1'`); err != nil {
		t.Fatalf("set meaning: %v", err)
	}
	if _, err := database.ExecContext(ctx,
		`INSERT INTO capture_items(id, capture_id, knowledge_item_id, role, confidence, created_at) VALUES
('ci-1','cap-1','know-1','sub_item',0.9,?),
('ci-2','cap-1','know-2','sub_item',0.3,?)`, now, now); err != nil {
		t.Fatalf("seed capture_items: %v", err)
	}
	// know-1 has learner state (wrong_count 2, known); know-2 has none (defaults).
	if _, err := database.ExecContext(ctx,
		`INSERT INTO learner_items(id, knowledge_item_id, wrong_count, ask_count, status) VALUES ('l1','know-1',2,3,'known')`); err != nil {
		t.Fatalf("seed learner: %v", err)
	}

	repo := NewKnowledgeRepository(database)
	items, err := repo.ListByCapture(ctx, "cap-1")
	if err != nil {
		t.Fatalf("ListByCapture() error = %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("len(items) = %d, want 2", len(items))
	}
	// Highest confidence first.
	if items[0].KnowledgeItemID != "know-1" || items[0].MeaningKo != "오래된" ||
		items[0].PronunciationKo != "스테일" || items[0].Status != knowledge.StatusKnown || items[0].WrongCount != 2 {
		t.Fatalf("items[0] = %#v", items[0])
	}
	// Missing learner_items row falls back to active/0.
	if items[1].KnowledgeItemID != "know-2" || items[1].Status != knowledge.StatusActive || items[1].WrongCount != 0 {
		t.Fatalf("items[1] = %#v", items[1])
	}
}

func TestKnowledgeRepositoryListByCaptureNotFound(t *testing.T) {
	database := openMigratedDB(t)
	repo := NewKnowledgeRepository(database)
	if _, err := repo.ListByCapture(context.Background(), "missing"); !errors.Is(err, knowledge.ErrCaptureNotFound) {
		t.Fatalf("ListByCapture() error = %v, want ErrCaptureNotFound", err)
	}
}

func TestKnowledgeRepositoryListByCaptureEmpty(t *testing.T) {
	database := openMigratedDB(t)
	ctx := context.Background()
	if _, err := database.ExecContext(ctx,
		`INSERT INTO captures(id, selected_text, input_mode, text_hash, created_at) VALUES ('cap-1','hi','manual','h',?)`,
		time.Now().UTC()); err != nil {
		t.Fatalf("seed capture: %v", err)
	}
	repo := NewKnowledgeRepository(database)
	items, err := repo.ListByCapture(ctx, "cap-1")
	if err != nil {
		t.Fatalf("ListByCapture() error = %v", err)
	}
	if len(items) != 0 {
		t.Fatalf("len(items) = %d, want 0 (capture exists, no items)", len(items))
	}
}
