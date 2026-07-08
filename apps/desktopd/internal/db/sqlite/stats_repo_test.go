package sqlite

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"neulsang/desktopd/internal/domain/stats"
)

func TestStatsRepositorySummary(t *testing.T) {
	database := openMigratedDB(t)
	now := time.Date(2026, 7, 8, 15, 0, 0, 0, time.UTC)
	todayStart := time.Date(2026, 7, 8, 0, 0, 0, 0, time.UTC)
	weekStart := now.AddDate(0, 0, -7)

	// captures: 2 today, 1 five days ago (in week), 1 ten days ago (outside week)
	insertCaptureAt(t, database, "cap-today-1", todayStart.Add(time.Hour))
	insertCaptureAt(t, database, "cap-today-2", todayStart.Add(2*time.Hour))
	insertCaptureAt(t, database, "cap-week", now.AddDate(0, 0, -5))
	insertCaptureAt(t, database, "cap-old", now.AddDate(0, 0, -10))

	// knowledge items + learner rows across categories
	insertKnowledgeItemWithCategory(t, database, "k-backend", "goroutine", "backend")
	insertKnowledgeItemWithCategory(t, database, "k-db", "index", "database")
	insertLearner(t, database, "k-backend", 5, 3, 0.2)
	insertLearner(t, database, "k-db", 2, 0, 0.9)

	// review card: one due, one future; one review log today
	insertDueCard(t, database, "rc-due", "k-backend", now.Add(-time.Hour))
	insertDueCard(t, database, "rc-future", "k-db", now.Add(48*time.Hour))
	insertReviewLogAt(t, database, "rl-1", "rc-due", todayStart.Add(3*time.Hour))

	repo := NewStatsRepository(database)
	raw, err := repo.Summary(context.Background(), stats.Window{Now: now, TodayStart: todayStart, WeekStart: weekStart}, 10)
	if err != nil {
		t.Fatalf("Summary() error = %v", err)
	}

	if raw.TodaySearchCount != 2 || raw.WeekSearchCount != 3 {
		t.Errorf("today=%d week=%d, want 2/3", raw.TodaySearchCount, raw.WeekSearchCount)
	}
	if raw.TodayCompletedReviews != 1 {
		t.Errorf("today reviews = %d, want 1", raw.TodayCompletedReviews)
	}
	if raw.DueCardCount != 1 {
		t.Errorf("due cards = %d, want 1", raw.DueCardCount)
	}
	if len(raw.MostSearched) != 2 || raw.MostSearched[0].KnowledgeItemID != "k-backend" || raw.MostSearched[0].Count != 5 {
		t.Errorf("most searched = %#v", raw.MostSearched)
	}
	if len(raw.MostWrong) != 1 || raw.MostWrong[0].KnowledgeItemID != "k-backend" || raw.MostWrong[0].Count != 3 {
		t.Errorf("most wrong = %#v (only k-backend has wrong_count>0)", raw.MostWrong)
	}
	if len(raw.Categories) != 2 {
		t.Fatalf("categories = %#v", raw.Categories)
	}
	byCat := map[string]stats.CategoryAggregate{}
	for _, c := range raw.Categories {
		byCat[c.Category] = c
	}
	if b := byCat["backend"]; b.ItemCount != 1 || b.AskSum != 5 || b.WrongSum != 3 {
		t.Errorf("backend aggregate = %#v", b)
	}
}

func insertCaptureAt(t *testing.T, database *sql.DB, id string, at time.Time) {
	t.Helper()
	if _, err := database.ExecContext(context.Background(),
		`INSERT INTO captures(id, selected_text, input_mode, text_hash, created_at) VALUES (?, 'x', 'manual', ?, ?)`,
		id, id+"-h", at); err != nil {
		t.Fatalf("insert capture %s: %v", id, err)
	}
}

func insertKnowledgeItemWithCategory(t *testing.T, database *sql.DB, id, surface, category string) {
	t.Helper()
	at := time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)
	if _, err := database.ExecContext(context.Background(),
		`INSERT INTO knowledge_items(id, normalized_key, surface_text, item_type, language, domain_category, first_seen_at, last_seen_at)
VALUES (?, ?, ?, 'word', 'en', ?, ?, ?)`,
		id, id+"-key", surface, category, at, at); err != nil {
		t.Fatalf("insert knowledge item %s: %v", id, err)
	}
}

func insertLearner(t *testing.T, database *sql.DB, knowledgeID string, askCount, wrongCount int, mastery float64) {
	t.Helper()
	if _, err := database.ExecContext(context.Background(),
		`INSERT INTO learner_items(id, knowledge_item_id, ask_count, wrong_count, mastery_score) VALUES (?, ?, ?, ?, ?)`,
		knowledgeID+"-learner", knowledgeID, askCount, wrongCount, mastery); err != nil {
		t.Fatalf("insert learner %s: %v", knowledgeID, err)
	}
}

func insertDueCard(t *testing.T, database *sql.DB, id, knowledgeID string, dueAt time.Time) {
	t.Helper()
	at := time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)
	if _, err := database.ExecContext(context.Background(),
		`INSERT INTO review_cards(id, knowledge_item_id, card_type, question, answer, state, due_at, created_at, updated_at)
VALUES (?, ?, 'meaning', 'q', 'a', 'new', ?, ?, ?)`,
		id, knowledgeID, dueAt, at, at); err != nil {
		t.Fatalf("insert card %s: %v", id, err)
	}
}

func insertReviewLogAt(t *testing.T, database *sql.DB, id, cardID string, at time.Time) {
	t.Helper()
	if _, err := database.ExecContext(context.Background(),
		`INSERT INTO review_logs(id, review_card_id, source, rating, reviewed_at) VALUES (?, ?, 'review', 'good', ?)`,
		id, cardID, at); err != nil {
		t.Fatalf("insert review log %s: %v", id, err)
	}
}
