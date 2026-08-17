package sqlite

import (
	"context"
	"testing"
	"time"

	"neulsang/desktopd/internal/domain/review"
)

// The product doc asks for items at 100% accuracy to be dropped from review. Taken
// literally that is a time bomb: one "good" puts a card at 100% and it never comes
// back, so the review list quietly drains and the app stops asking anything. These
// tests pin the chosen reading — 잘 아는 것은 뒤로 밀되 빼지는 않는다 (D6).

func TestDueCardsKeepsPerfectItemInRotation(t *testing.T) {
	database := openMigratedDB(t)
	now := time.Date(2026, 7, 27, 12, 0, 0, 0, time.UTC)
	seedLearnerItem(t, database, learnerFixture{
		knowledgeID: "know-perfect", surfaceText: "stale",
		attemptCount: 1, correctCount: 1,
	})
	seedReviewCard(t, database, "card-perfect", "know-perfect", "meaning", "", now.Add(-time.Hour))

	got, err := NewReviewRepository(database).DueCards(context.Background(), now, 50)
	if err != nil {
		t.Fatalf("DueCards() error = %v", err)
	}
	if len(got) != 1 || got[0].CardID != "card-perfect" {
		t.Fatalf("an item answered correctly once must still come up for review, got %#v", got)
	}
}

func TestDueCardsKeepsMasteredItemInRotation(t *testing.T) {
	database := openMigratedDB(t)
	now := time.Date(2026, 7, 27, 12, 0, 0, 0, time.UTC)
	attempts := review.MinAttemptsForMastery + 2
	seedLearnerItem(t, database, learnerFixture{
		knowledgeID: "know-mastered", surfaceText: "stale",
		attemptCount: attempts, correctCount: attempts,
	})
	seedReviewCard(t, database, "card-mastered", "know-mastered", "meaning", "", now.Add(-time.Hour))

	got, err := NewReviewRepository(database).DueCards(context.Background(), now, 50)
	if err != nil {
		t.Fatalf("DueCards() error = %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("a mastered item must still be reachable, otherwise it is stranded forever: %#v", got)
	}
}

func TestDueCardsOrdersMasteredItemsLast(t *testing.T) {
	database := openMigratedDB(t)
	now := time.Date(2026, 7, 27, 12, 0, 0, 0, time.UTC)
	attempts := review.MinAttemptsForMastery
	// The mastered card is due *earlier*, so plain due_at ordering would put it first.
	seedLearnerItem(t, database, learnerFixture{
		knowledgeID: "know-mastered", surfaceText: "known well",
		attemptCount: attempts, correctCount: attempts,
	})
	seedReviewCard(t, database, "card-mastered", "know-mastered", "meaning", "", now.Add(-10*time.Hour))
	seedLearnerItem(t, database, learnerFixture{
		knowledgeID: "know-shaky", surfaceText: "not so much",
		attemptCount: attempts, correctCount: attempts - 1,
	})
	seedReviewCard(t, database, "card-shaky", "know-shaky", "meaning", "", now.Add(-time.Hour))

	got, err := NewReviewRepository(database).DueCards(context.Background(), now, 50)
	if err != nil {
		t.Fatalf("DueCards() error = %v", err)
	}
	if len(got) != 2 || got[0].CardID != "card-shaky" || got[1].CardID != "card-mastered" {
		t.Fatalf("DueCards() order = %#v, want the shaky card first even though the mastered one is older", got)
	}
}

// One miss is enough to lose mastery: correct_count no longer reaches attempt_count.
func TestDueCardsOneMissEndsMastery(t *testing.T) {
	database := openMigratedDB(t)
	now := time.Date(2026, 7, 27, 12, 0, 0, 0, time.UTC)
	seedLearnerItem(t, database, learnerFixture{
		knowledgeID: "know-slipped", surfaceText: "slipped",
		attemptCount: 10, correctCount: 9,
	})
	seedReviewCard(t, database, "card-slipped", "know-slipped", "meaning", "", now.Add(-10*time.Hour))
	seedLearnerItem(t, database, learnerFixture{
		knowledgeID: "know-clean", surfaceText: "clean",
		attemptCount: review.MinAttemptsForMastery, correctCount: review.MinAttemptsForMastery,
	})
	seedReviewCard(t, database, "card-clean", "know-clean", "meaning", "", now.Add(-11*time.Hour))

	got, err := NewReviewRepository(database).DueCards(context.Background(), now, 50)
	if err != nil {
		t.Fatalf("DueCards() error = %v", err)
	}
	if len(got) != 2 || got[0].CardID != "card-slipped" {
		t.Fatalf("DueCards() order = %#v, want the 9/10 item ahead of the spotless one", got)
	}
}

// Mastery is a sort key, so it must not change how many cards are due. If it ever
// became a filter here, the tray badge and the review screen would disagree — the user
// gets told "3 cards waiting", opens the app and finds an empty list.
func TestDueCardCountMatchesNotificationBadge(t *testing.T) {
	database := openMigratedDB(t)
	now := time.Date(2026, 7, 27, 12, 0, 0, 0, time.UTC)
	attempts := review.MinAttemptsForMastery
	for _, fixture := range []learnerFixture{
		{knowledgeID: "know-fresh", surfaceText: "fresh"},
		{knowledgeID: "know-mastered", surfaceText: "mastered", attemptCount: attempts, correctCount: attempts},
		{knowledgeID: "know-shaky", surfaceText: "shaky", attemptCount: attempts, correctCount: 1},
	} {
		seedLearnerItem(t, database, fixture)
		seedReviewCard(t, database, "card-"+fixture.knowledgeID, fixture.knowledgeID, "meaning", "", now.Add(-time.Hour))
	}

	cards, err := NewReviewRepository(database).DueCards(context.Background(), now, 50)
	if err != nil {
		t.Fatalf("DueCards() error = %v", err)
	}
	count, err := NewNotificationRepository(database).CountDueCards(context.Background(), now)
	if err != nil {
		t.Fatalf("CountDueCards() error = %v", err)
	}
	if count != len(cards) || count != 3 {
		t.Fatalf("due badge = %d, due list = %d, want both 3", count, len(cards))
	}
}

// The SQL in learnerIsMastered and the Go in review.IsMastered are the same rule
// written twice, which is exactly the kind of pair that drifts. This walks the
// boundary cases through the database and compares.
func TestDueCardsMasteryMatchesDomainPredicate(t *testing.T) {
	now := time.Date(2026, 7, 27, 12, 0, 0, 0, time.UTC)
	cases := []struct{ attempts, correct int }{
		{0, 0},
		{1, 1},
		{review.MinAttemptsForMastery - 1, review.MinAttemptsForMastery - 1},
		{review.MinAttemptsForMastery, review.MinAttemptsForMastery},
		{review.MinAttemptsForMastery, review.MinAttemptsForMastery - 1},
		{20, 20},
	}

	for _, tc := range cases {
		// A fresh database per case, holding the subject and one plain control item
		// that is due *later*. Due order alone would put the subject first, so the
		// subject leading means "not mastered" and trailing means "mastered" — no ties
		// to make the result depend on row order.
		database := openMigratedDB(t)
		seedLearnerItem(t, database, learnerFixture{knowledgeID: "know-control", surfaceText: "control"})
		seedReviewCard(t, database, "card-control", "know-control", "meaning", "", now.Add(-time.Hour))
		seedLearnerItem(t, database, learnerFixture{
			knowledgeID: "know-subject", surfaceText: "subject",
			attemptCount: tc.attempts, correctCount: tc.correct,
		})
		seedReviewCard(t, database, "card-subject", "know-subject", "meaning", "", now.Add(-2*time.Hour))

		got, err := NewReviewRepository(database).DueCards(context.Background(), now, 50)
		if err != nil {
			t.Fatalf("DueCards() error = %v", err)
		}
		if len(got) != 2 {
			t.Fatalf("attempts=%d correct=%d: DueCards() = %#v, want both cards", tc.attempts, tc.correct, got)
		}
		sortedLast := got[1].CardID == "card-subject"
		if want := review.IsMastered(tc.attempts, tc.correct); sortedLast != want {
			t.Fatalf("attempts=%d correct=%d: SQL sorted-last=%v but review.IsMastered=%v", tc.attempts, tc.correct, sortedLast, want)
		}
	}
}
