package handlers

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"neulsang/desktopd/internal/domain/review"
)

type fakeReviewService struct {
	due func(context.Context, review.DueInput) ([]review.Card, error)
}

func (f fakeReviewService) Due(ctx context.Context, input review.DueInput) ([]review.Card, error) {
	return f.due(ctx, input)
}

func TestReviewDueOK(t *testing.T) {
	dueAt := time.Date(2026, 7, 8, 9, 0, 0, 0, time.UTC)
	handler := NewReview(fakeReviewService{due: func(_ context.Context, input review.DueInput) ([]review.Card, error) {
		if input.Limit != 10 {
			t.Fatalf("limit = %d", input.Limit)
		}
		return []review.Card{{
			CardID: "card-1", KnowledgeItemID: "know-1", CardType: "meaning",
			Question: "stale의 뜻은?", State: review.CardStateNew, DueAt: dueAt,
		}}, nil
	}}, slog.Default())
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/v1/reviews/due?limit=10", nil)

	handler.Due(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusOK)
	}
	var body struct {
		Cards []map[string]any `json:"cards"`
	}
	if err := json.NewDecoder(recorder.Body).Decode(&body); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if len(body.Cards) != 1 {
		t.Fatalf("cards = %#v", body.Cards)
	}
	card := body.Cards[0]
	if card["card_id"] != "card-1" || card["knowledge_item_id"] != "know-1" || card["card_type"] != "meaning" || card["question"] != "stale의 뜻은?" || card["state"] != "new" {
		t.Fatalf("card = %#v", card)
	}
	if _, hasAnswer := card["answer"]; hasAnswer {
		t.Fatalf("due response must not leak answer: %#v", card)
	}
}

func TestReviewDueInvalidLimit(t *testing.T) {
	handler := NewReview(fakeReviewService{due: func(_ context.Context, _ review.DueInput) ([]review.Card, error) {
		t.Fatal("service should not be called on invalid limit")
		return nil, nil
	}}, slog.Default())
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/v1/reviews/due?limit=abc", nil)

	handler.Due(recorder, request)

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusBadRequest)
	}
}
