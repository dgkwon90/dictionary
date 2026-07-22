package suggest

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

type fakeSuggester struct {
	gotQuery string
	called   bool
	result   []Candidate
	err      error
}

func (f *fakeSuggester) Suggest(_ context.Context, query string) ([]Candidate, error) {
	f.called = true
	f.gotQuery = query
	return f.result, f.err
}

type fakeRepo struct {
	cached      []Candidate
	savedQuery  string
	savedEng    string
	savedGloss  string
	cachedErr   error
	saveErr     error
	saveCalled  bool
	cacheCalled bool
}

func (f *fakeRepo) Cached(_ context.Context, normalizedQuery string) ([]Candidate, error) {
	f.cacheCalled = true
	return f.cached, f.cachedErr
}

func (f *fakeRepo) SavePick(_ context.Context, normalizedQuery, english, glossKo string, _ time.Time) error {
	f.saveCalled = true
	f.savedQuery = normalizedQuery
	f.savedEng = english
	f.savedGloss = glossKo
	return f.saveErr
}

func TestServiceSuggestCacheFirst(t *testing.T) {
	suggester := &fakeSuggester{result: []Candidate{{English: "ai"}}}
	fallback := &fakeSuggester{result: []Candidate{{English: "local"}}}
	repo := &fakeRepo{cached: []Candidate{{English: "stale", Source: SourceCache}}}
	svc := NewService(suggester, fallback, repo)

	got, err := svc.Suggest(context.Background(), "스테일")
	if err != nil {
		t.Fatalf("Suggest() error = %v", err)
	}
	if len(got) != 1 || got[0].English != "stale" || got[0].Source != SourceCache {
		t.Fatalf("got = %#v, want cache hit", got)
	}
	if suggester.called {
		t.Fatal("suggester must not be called on cache hit")
	}
	if fallback.called {
		t.Fatal("fallback must not be called on cache hit")
	}
}

func TestServiceSuggestFallsBackToAIOnMiss(t *testing.T) {
	suggester := &fakeSuggester{result: []Candidate{{English: "stale", Source: SourceAI}}}
	fallback := &fakeSuggester{result: []Candidate{{English: "local"}}}
	repo := &fakeRepo{cached: nil}
	svc := NewService(suggester, fallback, repo)

	got, err := svc.Suggest(context.Background(), "  스테일  ")
	if err != nil {
		t.Fatalf("Suggest() error = %v", err)
	}
	if !repo.cacheCalled || !suggester.called {
		t.Fatalf("cacheCalled=%v suggesterCalled=%v, want both", repo.cacheCalled, suggester.called)
	}
	if suggester.gotQuery != "스테일" {
		t.Errorf("suggester query = %q, want normalized 스테일", suggester.gotQuery)
	}
	if len(got) != 1 || got[0].Source != SourceAI {
		t.Fatalf("got = %#v", got)
	}
	if fallback.called {
		t.Fatal("fallback must not be called when AI returns candidates")
	}
}

func TestServiceSuggestUsesFallbackWhenAIEmpty(t *testing.T) {
	suggester := &fakeSuggester{result: nil}
	fallback := &fakeSuggester{result: []Candidate{{English: "stale", Source: SourceLocal}}}
	svc := NewService(suggester, fallback, nil)

	got, err := svc.Suggest(context.Background(), "스테일")
	if err != nil {
		t.Fatalf("Suggest() error = %v", err)
	}
	if !suggester.called || !fallback.called {
		t.Fatalf("suggesterCalled=%v fallbackCalled=%v, want both", suggester.called, fallback.called)
	}
	if len(got) != 1 || got[0].Source != SourceLocal {
		t.Fatalf("got = %#v, want local fallback", got)
	}
}

func TestServiceSuggestUsesFallbackWhenAIErrors(t *testing.T) {
	suggester := &fakeSuggester{err: errors.New("ai failed")}
	fallback := &fakeSuggester{result: []Candidate{{English: "stale", Source: SourceLocal}}}
	svc := NewService(suggester, fallback, nil)

	got, err := svc.Suggest(context.Background(), "스테일")
	if err != nil {
		t.Fatalf("Suggest() error = %v", err)
	}
	if !suggester.called || !fallback.called {
		t.Fatalf("suggesterCalled=%v fallbackCalled=%v, want both", suggester.called, fallback.called)
	}
	if len(got) != 1 || got[0].Source != SourceLocal {
		t.Fatalf("got = %#v, want local fallback", got)
	}
}

func TestServiceSuggestValidation(t *testing.T) {
	svc := NewService(&fakeSuggester{}, nil, nil)
	if _, err := svc.Suggest(context.Background(), "   "); !errors.Is(err, ErrInvalidInput) {
		t.Errorf("empty: err = %v", err)
	}
	if _, err := svc.Suggest(context.Background(), strings.Repeat("가", MaxQueryLen+1)); !errors.Is(err, ErrInvalidInput) {
		t.Errorf("too long: err = %v", err)
	}
}

func TestServiceSuggestUnavailableWithoutSuggester(t *testing.T) {
	svc := NewService(nil, nil, nil) // no cache, no AI, no local fallback
	if _, err := svc.Suggest(context.Background(), "뮤텍스"); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("err = %v, want ErrUnavailable", err)
	}
}

func TestServiceSuggestUsesFallbackWithoutAI(t *testing.T) {
	fallback := &fakeSuggester{result: []Candidate{{English: "mutex", Source: SourceLocal}}}
	svc := NewService(nil, fallback, nil)

	got, err := svc.Suggest(context.Background(), "뮤텍스")
	if err != nil {
		t.Fatalf("Suggest() error = %v", err)
	}
	if len(got) != 1 || got[0].Source != SourceLocal {
		t.Fatalf("got = %#v, want local fallback", got)
	}
}

func TestServiceConfirmPick(t *testing.T) {
	repo := &fakeRepo{}
	svc := NewService(&fakeSuggester{}, nil, repo)

	if err := svc.ConfirmPick(context.Background(), "  스테일 ", "  stale ", "오래된"); err != nil {
		t.Fatalf("ConfirmPick() error = %v", err)
	}
	if !repo.saveCalled || repo.savedQuery != "스테일" || repo.savedEng != "stale" || repo.savedGloss != "오래된" {
		t.Fatalf("repo saved query=%q eng=%q gloss=%q", repo.savedQuery, repo.savedEng, repo.savedGloss)
	}
}

func TestServiceConfirmPickValidation(t *testing.T) {
	repo := &fakeRepo{}
	svc := NewService(&fakeSuggester{}, nil, repo)
	if err := svc.ConfirmPick(context.Background(), "스테일", "  ", ""); !errors.Is(err, ErrInvalidInput) {
		t.Errorf("empty english: err = %v", err)
	}
	// no cache configured → unavailable
	svcNoRepo := NewService(&fakeSuggester{}, nil, nil)
	if err := svcNoRepo.ConfirmPick(context.Background(), "스테일", "stale", ""); !errors.Is(err, ErrUnavailable) {
		t.Errorf("nil repo: err = %v, want ErrUnavailable", err)
	}
}
