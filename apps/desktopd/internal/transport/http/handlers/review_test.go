package handlers

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"neulsang/desktopd/internal/domain/review"
)

type fakeReviewService struct {
	due   func(context.Context, review.DueInput) ([]review.Card, error)
	grade func(context.Context, review.GradeInput) (review.GradeResult, error)
}

func (f fakeReviewService) Due(ctx context.Context, input review.DueInput) ([]review.Card, error) {
	return f.due(ctx, input)
}

func (f fakeReviewService) StartSession(ctx context.Context, input review.DueInput) ([]review.Card, error) {
	return f.due(ctx, input)
}

func (f fakeReviewService) Grade(ctx context.Context, input review.GradeInput) (review.GradeResult, error) {
	return f.grade(ctx, input)
}

func TestReviewDueOK(t *testing.T) {
	dueAt := time.Date(2026, 7, 8, 9, 0, 0, 0, time.UTC)
	handler := NewReview(fakeReviewService{due: func(_ context.Context, input review.DueInput) ([]review.Card, error) {
		if input.Limit != 10 {
			t.Fatalf("limit = %d", input.Limit)
		}
		return []review.Card{{
			CardID: "card-1", KnowledgeItemID: "know-1", CardType: "meaning",
			Question: "stale의 뜻은?", Answer: "신선하지 않은", Explanation: "데이터가 오래됨",
			State: review.CardStateNew, DueAt: dueAt,
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
	// #16: 자가 채점 복습을 위해 답/설명을 함께 내려준다(로컬 단일 사용자라 유출 개념 없음).
	if card["answer"] != "신선하지 않은" || card["explanation"] != "데이터가 오래됨" {
		t.Fatalf("due response must carry answer/explanation: %#v", card)
	}
}

func TestReviewGradeOK(t *testing.T) {
	dueAt := time.Date(2026, 7, 11, 9, 0, 0, 0, time.UTC)
	handler := NewReview(fakeReviewService{grade: func(_ context.Context, input review.GradeInput) (review.GradeResult, error) {
		if input.CardID != "card-1" || input.Rating != review.RatingGood || input.ElapsedMs != 3200 {
			t.Fatalf("input = %#v", input)
		}
		return review.GradeResult{CardID: input.CardID, Rating: input.Rating, State: review.CardStateReview, Reps: 1, DueAt: dueAt, MasteryScore: 0.2}, nil
	}}, slog.Default())
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/v1/reviews/card-1/grade", strings.NewReader(`{"rating":"good","elapsed_ms":3200}`))
	request.SetPathValue("id", "card-1")

	handler.Grade(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusOK)
	}
	var body map[string]any
	if err := json.NewDecoder(recorder.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body["card_id"] != "card-1" || body["rating"] != "good" || body["state"] != "review" || body["reps"] != float64(1) || body["mastery_score"] != 0.2 {
		t.Fatalf("body = %#v", body)
	}
}

func TestReviewGradeNotFound(t *testing.T) {
	handler := NewReview(fakeReviewService{grade: func(_ context.Context, _ review.GradeInput) (review.GradeResult, error) {
		return review.GradeResult{}, review.ErrCardNotFound
	}}, slog.Default())
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/v1/reviews/missing/grade", strings.NewReader(`{"rating":"good"}`))
	request.SetPathValue("id", "missing")

	handler.Grade(recorder, request)

	if recorder.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusNotFound)
	}
}

func TestReviewGradeBadRating(t *testing.T) {
	handler := NewReview(fakeReviewService{grade: func(_ context.Context, _ review.GradeInput) (review.GradeResult, error) {
		return review.GradeResult{}, review.ErrInvalidInput
	}}, slog.Default())
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/v1/reviews/card-1/grade", strings.NewReader(`{"rating":"bogus"}`))
	request.SetPathValue("id", "card-1")

	handler.Grade(recorder, request)

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusBadRequest)
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
