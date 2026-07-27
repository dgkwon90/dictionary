package review

import (
	"context"
	"errors"
	"testing"
	"time"
)

type fakeRepo struct {
	now         time.Time
	limit       int
	query       string
	cards       []Card
	err         error
	gradeCardID string
	gradeRating string
	gradeResult GradeResult

	practiceGraded bool
	practiceResult PracticeResult
}

func (f *fakeRepo) DueCards(_ context.Context, now time.Time, limit int) ([]Card, error) {
	f.now = now
	f.limit = limit
	return f.cards, f.err
}

func (f *fakeRepo) PracticeCards(_ context.Context, query string, limit int) ([]Card, error) {
	f.query = query
	f.limit = limit
	return f.cards, f.err
}

func (f *fakeRepo) Grade(_ context.Context, cardID, rating string, _ int, now time.Time) (GradeResult, error) {
	f.gradeCardID = cardID
	f.gradeRating = rating
	f.now = now
	return f.gradeResult, f.err
}

func (f *fakeRepo) GradePractice(_ context.Context, cardID, rating string, _ int, now time.Time) (PracticeResult, error) {
	f.practiceGraded = true
	f.gradeCardID = cardID
	f.gradeRating = rating
	f.now = now
	return f.practiceResult, f.err
}

func TestServiceDueDefaultsLimit(t *testing.T) {
	repo := &fakeRepo{cards: []Card{{CardID: "c1"}}}
	svc := NewService(repo)
	fixed := time.Date(2026, 7, 8, 0, 0, 0, 0, time.UTC)
	svc.now = func() time.Time { return fixed }

	cards, err := svc.Due(context.Background(), DueInput{})
	if err != nil {
		t.Fatalf("Due() error = %v", err)
	}
	if repo.limit != DefaultDueLimit || !repo.now.Equal(fixed) {
		t.Fatalf("repo limit=%d now=%v", repo.limit, repo.now)
	}
	if len(cards) != 1 {
		t.Fatalf("cards = %#v", cards)
	}
}

func TestServiceDueClampsLimit(t *testing.T) {
	repo := &fakeRepo{}
	svc := NewService(repo)
	if _, err := svc.Due(context.Background(), DueInput{Limit: MaxDueLimit + 100}); err != nil {
		t.Fatalf("Due() error = %v", err)
	}
	if repo.limit != MaxDueLimit {
		t.Fatalf("limit = %d, want clamped to %d", repo.limit, MaxDueLimit)
	}
}

func TestServiceDueRejectsNegativeLimit(t *testing.T) {
	svc := NewService(&fakeRepo{})
	if _, err := svc.Due(context.Background(), DueInput{Limit: -1}); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("Due(-1) error = %v, want ErrInvalidInput", err)
	}
}

func TestServicePracticeDefaultsLimitAndTrimsQuery(t *testing.T) {
	repo := &fakeRepo{cards: []Card{{CardID: "c1"}}}
	svc := NewService(repo)
	svc.now = func() time.Time {
		t.Fatal("Practice() should not read current time")
		return time.Time{}
	}

	cards, err := svc.Practice(context.Background(), PracticeInput{Query: "  stale  "})
	if err != nil {
		t.Fatalf("Practice() error = %v", err)
	}
	if repo.limit != DefaultDueLimit || repo.query != "stale" {
		t.Fatalf("repo limit=%d query=%q", repo.limit, repo.query)
	}
	if len(cards) != 1 {
		t.Fatalf("cards = %#v", cards)
	}
}

func TestServicePracticeClampsLimit(t *testing.T) {
	repo := &fakeRepo{}
	svc := NewService(repo)
	if _, err := svc.Practice(context.Background(), PracticeInput{Limit: MaxDueLimit + 100}); err != nil {
		t.Fatalf("Practice() error = %v", err)
	}
	if repo.limit != MaxDueLimit {
		t.Fatalf("limit = %d, want clamped to %d", repo.limit, MaxDueLimit)
	}
}

func TestServicePracticeRejectsNegativeLimit(t *testing.T) {
	svc := NewService(&fakeRepo{})
	if _, err := svc.Practice(context.Background(), PracticeInput{Limit: -1}); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("Practice(-1) error = %v, want ErrInvalidInput", err)
	}
}

func TestServiceGradeDelegates(t *testing.T) {
	repo := &fakeRepo{gradeResult: GradeResult{CardID: "c1", Rating: RatingGood, Reps: 1}}
	svc := NewService(repo)
	fixed := time.Date(2026, 7, 8, 0, 0, 0, 0, time.UTC)
	svc.now = func() time.Time { return fixed }

	result, err := svc.Grade(context.Background(), GradeInput{CardID: "c1", Rating: RatingGood, ElapsedMs: 100})
	if err != nil {
		t.Fatalf("Grade() error = %v", err)
	}
	if repo.gradeCardID != "c1" || repo.gradeRating != RatingGood || !repo.now.Equal(fixed) {
		t.Fatalf("repo got card=%q rating=%q now=%v", repo.gradeCardID, repo.gradeRating, repo.now)
	}
	if result.Reps != 1 {
		t.Fatalf("result = %#v", result)
	}
}

func TestServiceGradeValidation(t *testing.T) {
	svc := NewService(&fakeRepo{})
	if _, err := svc.Grade(context.Background(), GradeInput{CardID: "", Rating: RatingGood}); !errors.Is(err, ErrInvalidInput) {
		t.Errorf("empty card id: err = %v, want ErrInvalidInput", err)
	}
	if _, err := svc.Grade(context.Background(), GradeInput{CardID: "c1", Rating: "bogus"}); !errors.Is(err, ErrInvalidInput) {
		t.Errorf("bad rating: err = %v, want ErrInvalidInput", err)
	}
	if _, err := svc.Grade(context.Background(), GradeInput{CardID: "c1", Rating: RatingGood, ElapsedMs: -1}); !errors.Is(err, ErrInvalidInput) {
		t.Errorf("negative elapsed: err = %v, want ErrInvalidInput", err)
	}
}

func TestServiceGradePracticeReachesRepository(t *testing.T) {
	repo := &fakeRepo{practiceResult: PracticeResult{CardID: "c1", AttemptCount: 3, CorrectCount: 2, Accuracy: 2.0 / 3.0}}
	svc := NewService(repo)
	fixed := time.Date(2026, 7, 27, 12, 0, 0, 0, time.UTC)
	svc.now = func() time.Time { return fixed }

	result, err := svc.GradePractice(context.Background(), GradeInput{CardID: "c1", Rating: RatingGood, ElapsedMs: 100})
	if err != nil {
		t.Fatalf("GradePractice() error = %v", err)
	}
	if !repo.practiceGraded || repo.gradeCardID != "c1" || repo.gradeRating != RatingGood || !repo.now.Equal(fixed) {
		t.Fatalf("repo got practice=%v card=%q rating=%q now=%v", repo.practiceGraded, repo.gradeCardID, repo.gradeRating, repo.now)
	}
	if result.AttemptCount != 3 || result.CorrectCount != 2 {
		t.Fatalf("result = %#v", result)
	}
}

// A practice answer is still an answer, so it is held to the same input rules as a
// review grade — otherwise the practice route becomes a way to write junk ratings.
func TestServiceGradePracticeValidation(t *testing.T) {
	repo := &fakeRepo{}
	svc := NewService(repo)
	if _, err := svc.GradePractice(context.Background(), GradeInput{CardID: "", Rating: RatingGood}); !errors.Is(err, ErrInvalidInput) {
		t.Errorf("empty card id: err = %v, want ErrInvalidInput", err)
	}
	if _, err := svc.GradePractice(context.Background(), GradeInput{CardID: "c1", Rating: "bogus"}); !errors.Is(err, ErrInvalidInput) {
		t.Errorf("bad rating: err = %v, want ErrInvalidInput", err)
	}
	if _, err := svc.GradePractice(context.Background(), GradeInput{CardID: "c1", Rating: RatingGood, ElapsedMs: -1}); !errors.Is(err, ErrInvalidInput) {
		t.Errorf("negative elapsed: err = %v, want ErrInvalidInput", err)
	}
	if repo.practiceGraded {
		t.Error("invalid input must not reach the repository")
	}
}
