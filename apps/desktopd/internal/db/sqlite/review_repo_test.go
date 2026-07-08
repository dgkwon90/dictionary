package sqlite

import (
	"context"
	"testing"
	"time"

	"neulsang/desktopd/internal/domain/review"
)

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
	if got[0].KnowledgeItemID != "know-1" || got[0].CardType != "meaning" || got[0].Question != "q" || got[0].State != review.CardStateNew {
		t.Fatalf("card fields = %#v", got[0])
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
