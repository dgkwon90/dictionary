package review

import (
	"context"
	"errors"
	"testing"
	"time"
)

type fakeRepo struct {
	now   time.Time
	limit int
	cards []Card
	err   error
}

func (f *fakeRepo) DueCards(_ context.Context, now time.Time, limit int) ([]Card, error) {
	f.now = now
	f.limit = limit
	return f.cards, f.err
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
