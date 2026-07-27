package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"neulsang/desktopd/internal/domain/knowledge"
	"neulsang/desktopd/internal/domain/learning"
)

func insertKnowledgeItemFixture(t *testing.T, database *sql.DB, knowledgeID string) {
	t.Helper()
	at := time.Date(2026, 7, 7, 1, 0, 0, 0, time.UTC)
	if _, err := database.ExecContext(
		context.Background(),
		`INSERT INTO knowledge_items(id, normalized_key, surface_text, learn_kind, language, first_seen_at, last_seen_at, updated_at)
VALUES (?, ?, ?, 'word', 'en', ?, ?, ?)`,
		knowledgeID, knowledgeID+"-key", knowledgeID, at, at, at,
	); err != nil {
		t.Fatalf("insert knowledge item fixture: %v", err)
	}
}

func seedCandidate(t *testing.T, database *sql.DB, candidateID, knowledgeID string) {
	t.Helper()
	seedCandidateOfType(t, database, candidateID, knowledgeID, "meaning")
}

func seedCandidateOfType(t *testing.T, database *sql.DB, candidateID, knowledgeID, cardType string) {
	t.Helper()
	if _, err := database.ExecContext(context.Background(),
		`INSERT INTO review_card_candidates(id, capture_id, knowledge_item_id, card_type, question, answer, created_at)
VALUES (?, 'cap-1', ?, ?, 'q-'||?, 'a-'||?, ?)`,
		candidateID, knowledgeID, cardType, candidateID, candidateID, time.Now().UTC()); err != nil {
		t.Fatalf("seed candidate %s: %v", candidateID, err)
	}
}

func TestKnowledgeRepositoryListByCapture(t *testing.T) {
	database := openMigratedDB(t)
	ctx := context.Background()
	now := time.Now().UTC()
	if _, err := database.ExecContext(ctx,
		`INSERT INTO captures(id, selected_text, input_mode, text_hash, created_at, updated_at) VALUES ('cap-1','hi','manual','h',?,?)`,
		now, now); err != nil {
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
		`INSERT INTO capture_items(id, capture_id, knowledge_item_id, role, confidence, created_at, updated_at) VALUES
('ci-1','cap-1','know-1','sub_item',0.9,?,?),
('ci-2','cap-1','know-2','sub_item',0.3,?,?)`, now, now, now, now); err != nil {
		t.Fatalf("seed capture_items: %v", err)
	}
	// know-1 has learner state (unknown_count 2, known); know-2 has none (defaults).
	if _, err := database.ExecContext(ctx,
		`INSERT INTO learner_items(id, knowledge_item_id, unknown_count, ask_count, status, registered_at, updated_at) VALUES ('l1','know-1',2,3,'known','2026-07-07T01:00:00Z','2026-07-07T01:00:00Z')`); err != nil {
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
		items[0].PronunciationKo != "스테일" || items[0].Status != learning.StatusKnown || items[0].UnknownCount != 2 {
		t.Fatalf("items[0] = %#v", items[0])
	}
	// Missing learner_items row falls back to active/0.
	if items[1].KnowledgeItemID != "know-2" || items[1].Status != learning.StatusActive || items[1].UnknownCount != 0 {
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
		`INSERT INTO captures(id, selected_text, input_mode, text_hash, created_at, updated_at) VALUES ('cap-1','hi','manual','h',?,?)`,
		time.Now().UTC(), time.Now().UTC()); err != nil {
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
