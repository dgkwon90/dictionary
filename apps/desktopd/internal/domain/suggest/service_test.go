package suggest

import (
	"context"
	"errors"
	"strings"
	"testing"
)

type fakeSuggester struct {
	gotQuery string
	result   []Candidate
	err      error
}

func (f *fakeSuggester) Suggest(_ context.Context, query string) ([]Candidate, error) {
	f.gotQuery = query
	return f.result, f.err
}

func TestServiceSuggestDelegatesTrimmed(t *testing.T) {
	repo := &fakeSuggester{result: []Candidate{{English: "stale", Confidence: 0.9}}}
	svc := NewService(repo)

	got, err := svc.Suggest(context.Background(), "  스테일  ")
	if err != nil {
		t.Fatalf("Suggest() error = %v", err)
	}
	if repo.gotQuery != "스테일" {
		t.Errorf("query = %q, want trimmed 스테일", repo.gotQuery)
	}
	if len(got) != 1 || got[0].English != "stale" {
		t.Fatalf("got = %#v", got)
	}
}

func TestServiceSuggestValidation(t *testing.T) {
	svc := NewService(&fakeSuggester{})
	if _, err := svc.Suggest(context.Background(), "   "); !errors.Is(err, ErrInvalidInput) {
		t.Errorf("empty: err = %v, want ErrInvalidInput", err)
	}
	if _, err := svc.Suggest(context.Background(), strings.Repeat("가", MaxQueryLen+1)); !errors.Is(err, ErrInvalidInput) {
		t.Errorf("too long: err = %v, want ErrInvalidInput", err)
	}
}

func TestServiceSuggestUnavailableWithoutSuggester(t *testing.T) {
	svc := NewService(nil)
	if _, err := svc.Suggest(context.Background(), "뮤텍스"); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("err = %v, want ErrUnavailable", err)
	}
}
