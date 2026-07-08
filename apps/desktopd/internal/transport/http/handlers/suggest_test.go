package handlers

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"neulsang/desktopd/internal/domain/suggest"
)

type fakeSuggestService struct {
	suggest func(context.Context, string) ([]suggest.Candidate, error)
}

func (f fakeSuggestService) Suggest(ctx context.Context, query string) ([]suggest.Candidate, error) {
	return f.suggest(ctx, query)
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
