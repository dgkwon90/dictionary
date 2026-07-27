package learning

import (
	"context"
	"errors"
	"testing"
	"time"
)

type fakeRepo struct {
	input    ListInput
	items    []Item
	statusID string
	status   string
	at       time.Time
	item     Item
	err      error
}

func (f *fakeRepo) List(_ context.Context, input ListInput) ([]Item, error) {
	f.input = input
	return f.items, f.err
}

func (f *fakeRepo) SetStatus(_ context.Context, knowledgeItemID, status string, at time.Time) (Item, error) {
	f.statusID, f.status, f.at = knowledgeItemID, status, at
	return f.item, f.err
}

func newTestService(repo Repository, now time.Time, loc *time.Location) *Service {
	svc := NewService(repo)
	svc.now = func() time.Time { return now }
	svc.loc = loc
	return svc
}

func TestServiceListDefaultsAndClamps(t *testing.T) {
	tests := []struct {
		name      string
		input     ListInput
		wantScope string
		wantLimit int
	}{
		{name: "empty scope becomes all", input: ListInput{}, wantScope: ScopeAll, wantLimit: DefaultLimit},
		{name: "limit clamped to max", input: ListInput{Limit: 9999}, wantScope: ScopeAll, wantLimit: MaxLimit},
		{name: "limit passes through", input: ListInput{Scope: ScopeWeak, Limit: 7}, wantScope: ScopeWeak, wantLimit: 7},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &fakeRepo{}
			svc := newTestService(repo, time.Now(), time.UTC)
			if _, err := svc.List(context.Background(), tt.input); err != nil {
				t.Fatalf("List() error = %v", err)
			}
			if repo.input.Scope != tt.wantScope || repo.input.Limit != tt.wantLimit {
				t.Fatalf("repo got scope=%q limit=%d, want %q/%d", repo.input.Scope, repo.input.Limit, tt.wantScope, tt.wantLimit)
			}
		})
	}
}

func TestServiceListRejectsBadInput(t *testing.T) {
	svc := NewService(&fakeRepo{})
	ctx := context.Background()
	for _, input := range []ListInput{
		{Scope: "yesterday"},
		{LearnKind: "phrase"},
		{Limit: -1},
	} {
		if _, err := svc.List(ctx, input); !errors.Is(err, ErrInvalidInput) {
			t.Errorf("List(%#v) error = %v, want ErrInvalidInput", input, err)
		}
	}
}

// "오늘" has to mean the user's today. With a UTC boundary the day would roll over at
// 09:00 KST, so a word registered at 08:00 this morning would still be filed under
// yesterday — the bug the dashboard already fixed once (RW-07).
func TestServiceListTodayUsesLocalMidnight(t *testing.T) {
	seoul := time.FixedZone("KST", 9*60*60)
	// 2026-07-27 08:00 KST is still 2026-07-26 in UTC.
	now := time.Date(2026, 7, 27, 8, 0, 0, 0, seoul)
	repo := &fakeRepo{}
	svc := newTestService(repo, now, seoul)

	if _, err := svc.List(context.Background(), ListInput{Scope: ScopeToday}); err != nil {
		t.Fatalf("List() error = %v", err)
	}
	want := time.Date(2026, 7, 27, 0, 0, 0, 0, seoul).UTC()
	if !repo.input.Since.Equal(want) {
		t.Fatalf("Since = %v, want local midnight %v", repo.input.Since, want)
	}
	if repo.input.Since.Location() != time.UTC {
		t.Fatalf("Since location = %v, want UTC (the column stores UTC)", repo.input.Since.Location())
	}
}

func TestServiceListWeekIsRollingSevenDays(t *testing.T) {
	now := time.Date(2026, 7, 27, 8, 0, 0, 0, time.UTC)
	repo := &fakeRepo{}
	svc := newTestService(repo, now, time.UTC)

	if _, err := svc.List(context.Background(), ListInput{Scope: ScopeWeek}); err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if want := now.AddDate(0, 0, -7); !repo.input.Since.Equal(want) {
		t.Fatalf("Since = %v, want %v", repo.input.Since, want)
	}
}

func TestServiceListLeavesUndatedScopesUnbounded(t *testing.T) {
	for _, scope := range []string{ScopeAll, ScopeWeak} {
		repo := &fakeRepo{}
		svc := newTestService(repo, time.Now(), time.UTC)
		if _, err := svc.List(context.Background(), ListInput{Scope: scope}); err != nil {
			t.Fatalf("List(%q) error = %v", scope, err)
		}
		if !repo.input.Since.IsZero() {
			t.Errorf("scope %q Since = %v, want zero", scope, repo.input.Since)
		}
	}
}

func TestServiceListDerivesScores(t *testing.T) {
	repo := &fakeRepo{items: []Item{
		{KnowledgeItemID: "graded", AskCount: 2, UnknownCount: 1, AttemptCount: 4, CorrectCount: 3},
		{KnowledgeItemID: "never-graded", AskCount: 2, UnknownCount: 1},
	}}
	svc := newTestService(repo, time.Now(), time.UTC)

	items, err := svc.List(context.Background(), ListInput{})
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if got := items[0].Accuracy; got != 0.75 {
		t.Errorf("graded accuracy = %v, want 0.75", got)
	}
	// An item nobody has reviewed reads 0% too, so its weakness must not include an
	// accuracy penalty — otherwise "never tried" would outrank "always wrong".
	if items[1].Accuracy != 0 || items[1].WeaknessScore <= items[0].WeaknessScore {
		t.Errorf("never-graded = %#v, want no accuracy term and a higher weakness than the graded item", items[1])
	}
}

func TestServiceRetireAndRemove(t *testing.T) {
	fixed := time.Date(2026, 7, 27, 3, 0, 0, 0, time.UTC)
	tests := []struct {
		name       string
		call       func(*Service, context.Context) (Item, error)
		wantStatus string
	}{
		{
			name:       "retire",
			call:       func(s *Service, ctx context.Context) (Item, error) { return s.Retire(ctx, "k1") },
			wantStatus: StatusKnown,
		},
		{
			name:       "remove",
			call:       func(s *Service, ctx context.Context) (Item, error) { return s.Remove(ctx, "k1") },
			wantStatus: StatusRemoved,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &fakeRepo{item: Item{KnowledgeItemID: "k1", AttemptCount: 2, CorrectCount: 1}}
			svc := newTestService(repo, fixed, time.UTC)

			item, err := tt.call(svc, context.Background())
			if err != nil {
				t.Fatalf("%s error = %v", tt.name, err)
			}
			if repo.statusID != "k1" || repo.status != tt.wantStatus || !repo.at.Equal(fixed) {
				t.Fatalf("repo got id=%q status=%q at=%v", repo.statusID, repo.status, repo.at)
			}
			if item.Accuracy != 0.5 {
				t.Fatalf("returned item accuracy = %v, want 0.5 (scores derived on the way out too)", item.Accuracy)
			}
		})
	}
}

func TestServiceSetStatusRejectsEmptyID(t *testing.T) {
	svc := NewService(&fakeRepo{})
	if _, err := svc.Retire(context.Background(), ""); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("Retire(\"\") error = %v, want ErrInvalidInput", err)
	}
	if _, err := svc.Remove(context.Background(), ""); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("Remove(\"\") error = %v, want ErrInvalidInput", err)
	}
}
