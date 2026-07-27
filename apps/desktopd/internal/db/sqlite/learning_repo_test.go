package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"neulsang/desktopd/internal/domain/learning"
	"neulsang/desktopd/internal/domain/review"
)

type learnerFixture struct {
	knowledgeID  string
	surfaceText  string
	learnKind    string
	meaningKo    string
	askCount     int
	unknownCount int
	attemptCount int
	correctCount int
	status       string
	registeredAt time.Time
}

func seedLearnerItem(t *testing.T, database *sql.DB, fixture learnerFixture) {
	t.Helper()
	ctx := context.Background()
	if fixture.learnKind == "" {
		fixture.learnKind = "word"
	}
	if fixture.status == "" {
		fixture.status = learning.StatusActive
	}
	if fixture.registeredAt.IsZero() {
		fixture.registeredAt = time.Date(2026, 7, 20, 0, 0, 0, 0, time.UTC)
	}
	if _, err := database.ExecContext(ctx,
		`INSERT INTO knowledge_items(id, normalized_key, surface_text, learn_kind, language,
 meaning_ko, first_seen_at, last_seen_at, updated_at)
VALUES (?, ?, ?, ?, 'en', ?, ?, ?, ?)`,
		fixture.knowledgeID, fixture.knowledgeID+"-key", fixture.surfaceText, fixture.learnKind,
		fixture.meaningKo, fixture.registeredAt, fixture.registeredAt, fixture.registeredAt,
	); err != nil {
		t.Fatalf("seed knowledge item %s: %v", fixture.knowledgeID, err)
	}
	if _, err := database.ExecContext(ctx,
		`INSERT INTO learner_items(id, knowledge_item_id, ask_count, unknown_count, attempt_count,
 correct_count, registered_at, status, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		"li-"+fixture.knowledgeID, fixture.knowledgeID, fixture.askCount, fixture.unknownCount,
		fixture.attemptCount, fixture.correctCount, fixture.registeredAt, fixture.status, fixture.registeredAt,
	); err != nil {
		t.Fatalf("seed learner item %s: %v", fixture.knowledgeID, err)
	}
}

func seedReviewCard(t *testing.T, database *sql.DB, cardID, knowledgeID, cardType, contextID string, dueAt time.Time) {
	t.Helper()
	var contextItem any
	if contextID != "" {
		contextItem = contextID
	}
	if _, err := database.ExecContext(context.Background(),
		`INSERT INTO review_cards(id, knowledge_item_id, context_knowledge_item_id, card_type,
 question, answer, state, due_at, created_at, updated_at)
VALUES (?, ?, ?, ?, 'q', 'a', 'new', ?, ?, ?)`,
		cardID, knowledgeID, contextItem, cardType, dueAt, dueAt, dueAt,
	); err != nil {
		t.Fatalf("seed review card %s: %v", cardID, err)
	}
}

func learningIDs(items []learning.Item) []string {
	ids := make([]string, 0, len(items))
	for _, item := range items {
		ids = append(ids, item.KnowledgeItemID)
	}
	return ids
}

func TestLearningRepositoryListScopes(t *testing.T) {
	database := openMigratedDB(t)
	now := time.Date(2026, 7, 27, 12, 0, 0, 0, time.UTC)
	// unknown_count 2 = registered once, then met again and still not known.
	seedLearnerItem(t, database, learnerFixture{
		knowledgeID: "today-word", surfaceText: "stale", registeredAt: now.Add(-2 * time.Hour),
		askCount: 2, unknownCount: 2,
	})
	// Registered once and never missed since: not a "자주 틀림" row even though
	// registration itself recorded one unknown mark.
	seedLearnerItem(t, database, learnerFixture{
		knowledgeID: "week-word", surfaceText: "idempotent", registeredAt: now.AddDate(0, 0, -3),
		askCount: 1, unknownCount: 1,
	})
	seedLearnerItem(t, database, learnerFixture{
		knowledgeID: "old-word", surfaceText: "cardinality", registeredAt: now.AddDate(0, 0, -30),
		askCount: 1, unknownCount: 1, attemptCount: 4, correctCount: 1,
	})

	todayStart := time.Date(2026, 7, 27, 0, 0, 0, 0, time.UTC)
	tests := []struct {
		name  string
		input learning.ListInput
		want  []string
	}{
		{
			name:  "all",
			input: learning.ListInput{Scope: learning.ScopeAll, Limit: 10},
			want:  []string{"today-word", "week-word", "old-word"},
		},
		{
			name:  "today",
			input: learning.ListInput{Scope: learning.ScopeToday, Limit: 10, Since: todayStart},
			want:  []string{"today-word"},
		},
		{
			name:  "week",
			input: learning.ListInput{Scope: learning.ScopeWeek, Limit: 10, Since: now.AddDate(0, 0, -7)},
			want:  []string{"today-word", "week-word"},
		},
		{
			// week-word has never been missed and never been graded, so it is not a
			// "자주 틀림" row at all — the tab shows evidence of difficulty, not a
			// reordering of everything.
			//
			// today-word ranks above old-word even though old-word fails most of its
			// reviews: WeaknessScore only adds for asks and unknown marks, and review
			// accuracy can only subtract, so an item with no unknown marks floors at 0
			// however badly it is reviewed. That is the formula as PRD §13.3 defines it
			// today; making review failure raise weakness belongs with the accuracy work
			// in the next step, not to a query that has to agree with the shared function.
			name:  "weak",
			input: learning.ListInput{Scope: learning.ScopeWeak, Limit: 10},
			want:  []string{"today-word", "old-word"},
		},
	}
	repo := NewLearningRepository(database)
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			items, err := repo.List(context.Background(), tt.input)
			if err != nil {
				t.Fatalf("List() error = %v", err)
			}
			got := learningIDs(items)
			if len(got) != len(tt.want) {
				t.Fatalf("ids = %v, want %v", got, tt.want)
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Fatalf("ids = %v, want %v", got, tt.want)
				}
			}
		})
	}
}

func TestLearningRepositoryListFiltersByKindAndQuery(t *testing.T) {
	database := openMigratedDB(t)
	seedLearnerItem(t, database, learnerFixture{knowledgeID: "w1", surfaceText: "stale", meaningKo: "오래된"})
	seedLearnerItem(t, database, learnerFixture{
		knowledgeID: "s1", surfaceText: "The cache became stale.", learnKind: "sentence", meaningKo: "캐시가 오래되었다.",
	})
	repo := NewLearningRepository(database)
	ctx := context.Background()

	words, err := repo.List(ctx, learning.ListInput{Scope: learning.ScopeAll, LearnKind: "word", Limit: 10})
	if err != nil {
		t.Fatalf("List(kind=word) error = %v", err)
	}
	if ids := learningIDs(words); len(ids) != 1 || ids[0] != "w1" {
		t.Fatalf("kind=word ids = %v, want [w1]", ids)
	}

	// The query matches the Korean meaning as well as the English text: the user often
	// remembers what a word meant, not how it was spelled.
	byMeaning, err := repo.List(ctx, learning.ListInput{Scope: learning.ScopeAll, Query: "오래된", Limit: 10})
	if err != nil {
		t.Fatalf("List(q) error = %v", err)
	}
	if ids := learningIDs(byMeaning); len(ids) != 1 || ids[0] != "w1" {
		t.Fatalf("q=오래된 ids = %v, want [w1]", ids)
	}
}

func TestLearningRepositoryListExcludesRetiredAndRemoved(t *testing.T) {
	database := openMigratedDB(t)
	seedLearnerItem(t, database, learnerFixture{knowledgeID: "active", surfaceText: "stale"})
	seedLearnerItem(t, database, learnerFixture{knowledgeID: "known", surfaceText: "known-word", status: learning.StatusKnown})
	seedLearnerItem(t, database, learnerFixture{knowledgeID: "gone", surfaceText: "gone-word", status: learning.StatusRemoved})

	items, err := NewLearningRepository(database).List(context.Background(), learning.ListInput{Scope: learning.ScopeAll, Limit: 10})
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if ids := learningIDs(items); len(ids) != 1 || ids[0] != "active" {
		t.Fatalf("ids = %v, want [active]", ids)
	}
}

// R5: a word inside a sentence owns a meaning card and a cloze card. Reading the card
// figures through a join would return the word twice and let it be counted twice in
// any ranking; they are read as scalar subqueries so the item stays one row.
func TestLearningRepositoryListCountsCardsWithoutDoublingItems(t *testing.T) {
	database := openMigratedDB(t)
	seedLearnerItem(t, database, learnerFixture{
		knowledgeID: "word", surfaceText: "stale", askCount: 1, unknownCount: 1, attemptCount: 3, correctCount: 1,
	})
	seedLearnerItem(t, database, learnerFixture{knowledgeID: "sentence", surfaceText: "The cache became stale.", learnKind: "sentence"})
	soon := time.Date(2026, 7, 28, 9, 0, 0, 0, time.UTC)
	later := time.Date(2026, 8, 1, 9, 0, 0, 0, time.UTC)
	seedReviewCard(t, database, "card-meaning", "word", "meaning", "", later)
	seedReviewCard(t, database, "card-cloze", "word", "cloze", "sentence", soon)

	items, err := NewLearningRepository(database).List(context.Background(),
		learning.ListInput{Scope: learning.ScopeWeak, Limit: 10})
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("items = %v, want exactly one row for the word", learningIDs(items))
	}
	if items[0].CardCount != 2 {
		t.Errorf("CardCount = %d, want 2", items[0].CardCount)
	}
	if !items[0].NextDueAt.Equal(soon) {
		t.Errorf("NextDueAt = %v, want the soonest card %v", items[0].NextDueAt, soon)
	}
}

// The weak scope has to rank in SQL, because ordering happens before LIMIT. This holds
// that expression to review.WeaknessScore so the two cannot drift into disagreeing
// about which word the user is worst at.
func TestLearningRepositoryWeakScopeOrderingMatchesDomainScore(t *testing.T) {
	database := openMigratedDB(t)
	fixtures := []learnerFixture{
		{knowledgeID: "k1", surfaceText: "one", askCount: 1, unknownCount: 2},
		{knowledgeID: "k2", surfaceText: "two", askCount: 5, unknownCount: 3, attemptCount: 4, correctCount: 3},
		{knowledgeID: "k3", surfaceText: "three", askCount: 2, unknownCount: 4, attemptCount: 2, correctCount: 0},
		{knowledgeID: "k4", surfaceText: "four", askCount: 9, unknownCount: 2, attemptCount: 10, correctCount: 10},
		{knowledgeID: "k5", surfaceText: "five", askCount: 3, unknownCount: 3},
	}
	for _, fixture := range fixtures {
		seedLearnerItem(t, database, fixture)
	}

	items, err := NewLearningRepository(database).List(context.Background(),
		learning.ListInput{Scope: learning.ScopeWeak, Limit: 10})
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(items) != len(fixtures) {
		t.Fatalf("got %d items, want %d", len(items), len(fixtures))
	}

	previous := -1.0
	for _, item := range items {
		accuracy := review.Accuracy(item.AttemptCount, item.CorrectCount)
		score := review.WeaknessScore(
			float64(item.AskCount), float64(item.UnknownCount), accuracy, item.AttemptCount > 0,
		)
		if previous >= 0 && score > previous {
			t.Fatalf("SQL put %s (domain score %.3f) after an item scoring %.3f — the orderings disagree",
				item.KnowledgeItemID, score, previous)
		}
		previous = score
	}
}

func TestLearningRepositorySetStatus(t *testing.T) {
	database := openMigratedDB(t)
	seedLearnerItem(t, database, learnerFixture{
		knowledgeID: "k1", surfaceText: "stale", askCount: 3, unknownCount: 2, attemptCount: 4, correctCount: 3,
	})
	at := time.Date(2026, 7, 27, 10, 0, 0, 0, time.UTC)

	item, err := NewLearningRepository(database).SetStatus(context.Background(), "k1", learning.StatusKnown, at)
	if err != nil {
		t.Fatalf("SetStatus() error = %v", err)
	}
	if item.Status != learning.StatusKnown || item.KnowledgeItemID != "k1" {
		t.Fatalf("item = %#v", item)
	}
	// Retiring must not erase the history behind the item.
	if item.AskCount != 3 || item.UnknownCount != 2 || item.AttemptCount != 4 || item.CorrectCount != 3 {
		t.Fatalf("counters lost: %#v", item)
	}

	var status string
	var updatedAt time.Time
	if err := database.QueryRowContext(context.Background(),
		`SELECT status, updated_at FROM learner_items WHERE knowledge_item_id = ?`, "k1").
		Scan(&status, &updatedAt); err != nil {
		t.Fatalf("query learner item: %v", err)
	}
	if status != learning.StatusKnown || !updatedAt.Equal(at) {
		t.Fatalf("row status=%q updated_at=%v", status, updatedAt)
	}
}

// A knowledge item that exists but was never registered has nothing to retire, and
// saying "not found" is the honest answer — it is not in the learning list.
func TestLearningRepositorySetStatusMissingItem(t *testing.T) {
	database := openMigratedDB(t)
	insertKnowledgeItemFixture(t, database, "never-registered")

	_, err := NewLearningRepository(database).SetStatus(
		context.Background(), "never-registered", learning.StatusKnown, time.Now().UTC())
	if !errors.Is(err, learning.ErrItemNotFound) {
		t.Fatalf("SetStatus() error = %v, want ErrItemNotFound", err)
	}
}
