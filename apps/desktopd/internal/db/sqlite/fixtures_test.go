package sqlite

// Fixtures shared by more than one repository test.
//
// They lived in knowledge_repo_test.go until that repository was deleted with the
// endpoint it served (ADR-0010: a capture's items now come from the search domain).
// The helpers outlived it because knowledge items and card candidates are seeded by
// the learner-registration, review and learning tests too.

import (
	"context"
	"database/sql"
	"testing"
	"time"
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
