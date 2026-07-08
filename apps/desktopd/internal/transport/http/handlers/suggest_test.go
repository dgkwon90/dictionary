package handlers

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"neulsang/desktopd/internal/domain/suggest"
)

type fakeSuggestService struct {
	suggest func(context.Context, string) ([]suggest.Candidate, error)
	confirm func(context.Context, string, string, string) error
}

func (f fakeSuggestService) Suggest(ctx context.Context, query string) ([]suggest.Candidate, error) {
	return f.suggest(ctx, query)
}

func (f fakeSuggestService) ConfirmPick(ctx context.Context, query, english, glossKo string) error {
	return f.confirm(ctx, query, english, glossKo)
}

func TestSuggestGetOK(t *testing.T) {
	handler := NewSuggest(fakeSuggestService{suggest: func(_ context.Context, q string) ([]suggest.Candidate, error) {
		if q != "스테일" {
			t.Fatalf("query = %q", q)
		}
		return []suggest.Candidate{{English: "stale", Confidence: 0.9, GlossKo: "오래된"}}, nil
	}}, slog.Default())
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/v1/suggest?q=스테일", nil)

	handler.Get(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusOK)
	}
	var body struct {
		Candidates []struct {
			English    string  `json:"english"`
			Confidence float64 `json:"confidence"`
			GlossKo    string  `json:"gloss_ko"`
		} `json:"candidates"`
	}
	if err := json.NewDecoder(recorder.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(body.Candidates) != 1 || body.Candidates[0].English != "stale" || body.Candidates[0].Confidence != 0.9 {
		t.Fatalf("candidates = %#v", body.Candidates)
	}
}

func TestSuggestConfirmOK(t *testing.T) {
	var gotQuery, gotEng string
	handler := NewSuggest(fakeSuggestService{confirm: func(_ context.Context, q, e, _ string) error {
		gotQuery, gotEng = q, e
		return nil
	}}, slog.Default())
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/v1/suggest/confirm", strings.NewReader(`{"query":"스테일","english":"stale","gloss_ko":"오래된"}`))

	handler.Confirm(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusOK)
	}
	if gotQuery != "스테일" || gotEng != "stale" {
		t.Fatalf("confirm got query=%q eng=%q", gotQuery, gotEng)
	}
}

func TestSuggestGetInvalid(t *testing.T) {
	handler := NewSuggest(fakeSuggestService{suggest: func(_ context.Context, _ string) ([]suggest.Candidate, error) {
		return nil, suggest.ErrInvalidInput
	}}, slog.Default())
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/v1/suggest?q=", nil)

	handler.Get(recorder, request)

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusBadRequest)
	}
}

func TestSuggestGetUnavailable(t *testing.T) {
	handler := NewSuggest(fakeSuggestService{suggest: func(_ context.Context, _ string) ([]suggest.Candidate, error) {
		return nil, suggest.ErrUnavailable
	}}, slog.Default())
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/v1/suggest?q=뮤텍스", nil)

	handler.Get(recorder, request)

	if recorder.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusServiceUnavailable)
	}
}
