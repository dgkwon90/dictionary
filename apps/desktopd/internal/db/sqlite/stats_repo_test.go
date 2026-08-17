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
	insertLearner(t, database, "k-backend", 5, 3, 5, 1)
	insertLearner(t, database, "k-db", 2, 0, 10, 9)

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
		t.Errorf("most wrong = %#v (only k-backend has unknown_count>0)", raw.MostWrong)
	}
	if len(raw.Categories) != 2 {
		t.Fatalf("categories = %#v", raw.Categories)
	}
	byCat := map[string]stats.CategoryAggregate{}
	for _, c := range raw.Categories {
		byCat[c.Category] = c
	}
	if b := byCat["backend"]; b.ItemCount != 1 || b.AskSum != 5 || b.UnknownSum != 3 {
		t.Errorf("backend aggregate = %#v", b)
	}
}

func TestStatsRepositorySummaryExcludesKnownLearnerItems(t *testing.T) {
	database := openMigratedDB(t)
	now := time.Date(2026, 7, 8, 15, 0, 0, 0, time.UTC)
	todayStart := time.Date(2026, 7, 8, 0, 0, 0, 0, time.UTC)
	weekStart := now.AddDate(0, 0, -7)

	insertKnowledgeItemWithCategory(t, database, "k-active", "active", "backend")
	insertKnowledgeItemWithCategory(t, database, "k-known", "known", "backend")
	insertLearner(t, database, "k-active", 1, 1, 10, 1)
	insertLearnerWithStatus(t, database, "k-known", 99, 99, 0, 0, "known")
	insertDueCard(t, database, "rc-active", "k-active", now.Add(-time.Hour))
	insertDueCard(t, database, "rc-known", "k-known", now.Add(-time.Hour))

	repo := NewStatsRepository(database)
	raw, err := repo.Summary(context.Background(), stats.Window{Now: now, TodayStart: todayStart, WeekStart: weekStart}, 10)
	if err != nil {
		t.Fatalf("Summary() error = %v", err)
	}
	if raw.DueCardCount != 1 {
		t.Fatalf("DueCardCount = %d, want only active due card", raw.DueCardCount)
	}
	if len(raw.MostSearched) != 1 || raw.MostSearched[0].KnowledgeItemID != "k-active" {
		t.Fatalf("MostSearched = %#v, want only active item", raw.MostSearched)
	}
	if len(raw.Categories) != 1 || raw.Categories[0].ItemCount != 1 || raw.Categories[0].AskSum != 1 || raw.Categories[0].UnknownSum != 1 {
		t.Fatalf("Categories = %#v, want only active item aggregate", raw.Categories)
	}
}

func insertCaptureAt(t *testing.T, database *sql.DB, id string, at time.Time) {
	t.Helper()
	if _, err := database.ExecContext(context.Background(),
		`INSERT INTO captures(id, selected_text, input_mode, text_hash, created_at, updated_at) VALUES (?, 'x', 'manual', ?, ?, ?)`,
		id, id+"-h", at, at); err != nil {
		t.Fatalf("insert capture %s: %v", id, err)
	}
}

func insertKnowledgeItemWithCategory(t *testing.T, database *sql.DB, id, surface, category string) {
	t.Helper()
	at := time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)
	if _, err := database.ExecContext(context.Background(),
		`INSERT INTO knowledge_items(id, normalized_key, surface_text, learn_kind, language, domain_category, first_seen_at, last_seen_at, updated_at)
VALUES (?, ?, ?, 'word', 'en', ?, ?, ?, ?)`,
		id, id+"-key", surface, category, at, at, at); err != nil {
		t.Fatalf("insert knowledge item %s: %v", id, err)
	}
}

func insertLearner(t *testing.T, database *sql.DB, knowledgeID string, askCount, wrongCount, attempts, correct int) {
	t.Helper()
	insertLearnerWithStatus(t, database, knowledgeID, askCount, wrongCount, attempts, correct, "active")
}

func insertLearnerWithStatus(t *testing.T, database *sql.DB, knowledgeID string, askCount, wrongCount, attempts, correct int, status string) {
	t.Helper()
	if _, err := database.ExecContext(context.Background(),
		`INSERT INTO learner_items(id, knowledge_item_id, ask_count, unknown_count, attempt_count, correct_count, status, registered_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		knowledgeID+"-learner", knowledgeID, askCount, wrongCount, attempts, correct, status,
		time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC), time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)); err != nil {
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
		`INSERT INTO review_logs(id, review_card_id, source, rating, is_correct, reviewed_at) VALUES (?, ?, 'review', 'good', 1, ?)`,
		id, cardID, at); err != nil {
		t.Fatalf("insert review log %s: %v", id, err)
	}
}

// "오늘 완료한 복습"은 예정된 복습만 센다. 연습은 정답률에는 들어가지만(D5) 여기에는
// 안 들어간다 — 한 카드를 오후 내내 드릴해서 이 숫자를 채울 수 있으면 지표가 아니다.
func TestStatsRepositorySummaryExcludesPracticeFromCompletedReviews(t *testing.T) {
	database := openMigratedDB(t)
	now := time.Date(2026, 7, 8, 12, 0, 0, 0, time.UTC)
	todayStart := time.Date(2026, 7, 8, 0, 0, 0, 0, time.UTC)
	insertKnowledgeItemWithCategory(t, database, "k-1", "stale", "backend")
	insertDueCard(t, database, "rc-1", "k-1", now.Add(-time.Hour))
	insertReviewLogAt(t, database, "rl-review", "rc-1", todayStart.Add(time.Hour))
	if _, err := database.ExecContext(context.Background(),
		`INSERT INTO review_logs(id, review_card_id, source, rating, is_correct, reviewed_at)
VALUES ('rl-practice-1', 'rc-1', 'practice', 'good', 1, ?),
       ('rl-practice-2', 'rc-1', 'practice', 'good', 1, ?)`,
		todayStart.Add(2*time.Hour), todayStart.Add(3*time.Hour),
	); err != nil {
		t.Fatalf("insert practice logs: %v", err)
	}

	raw, err := NewStatsRepository(database).Summary(
		context.Background(),
		stats.Window{Now: now, TodayStart: todayStart, WeekStart: now.AddDate(0, 0, -7)},
		10,
	)
	if err != nil {
		t.Fatalf("Summary() error = %v", err)
	}
	if raw.TodayCompletedReviews != 1 {
		t.Errorf("today reviews = %d, want 1 — two practice drills must not count", raw.TodayCompletedReviews)
	}
}
