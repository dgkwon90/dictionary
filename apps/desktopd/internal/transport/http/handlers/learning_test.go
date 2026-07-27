package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"neulsang/desktopd/internal/domain/learning"
)

type fakeLearningService struct {
	input    learning.ListInput
	items    []learning.Item
	item     learning.Item
	err      error
	lastCall string
}

func (f *fakeLearningService) List(_ context.Context, input learning.ListInput) ([]learning.Item, error) {
	f.input = input
	return f.items, f.err
}

func (f *fakeLearningService) Retire(_ context.Context, knowledgeItemID string) (learning.Item, error) {
	f.lastCall = "retire:" + knowledgeItemID
	return f.item, f.err
}

func (f *fakeLearningService) Remove(_ context.Context, knowledgeItemID string) (learning.Item, error) {
	f.lastCall = "remove:" + knowledgeItemID
	return f.item, f.err
}

func TestLearningListPassesFiltersThrough(t *testing.T) {
	svc := &fakeLearningService{}
	handler := NewLearning(svc, slog.Default())
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/v1/learning?scope=weak&kind=sentence&q=stale&limit=12", nil)

	handler.List(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusOK)
	}
	if svc.input.Scope != learning.ScopeWeak || svc.input.LearnKind != "sentence" ||
		svc.input.Query != "stale" || svc.input.Limit != 12 {
		t.Fatalf("service got %#v", svc.input)
	}
}

func TestLearningListEmptyIsAnEmptyArray(t *testing.T) {
	handler := NewLearning(&fakeLearningService{}, slog.Default())
	recorder := httptest.NewRecorder()
	handler.List(recorder, httptest.NewRequest(http.MethodGet, "/v1/learning", nil))

	// `"items":null` would make the screen branch on null before it can map over the
	// list, so an empty list stays an empty list.
	if !strings.Contains(recorder.Body.String(), `"items":[]`) {
		t.Fatalf("body = %q", recorder.Body.String())
	}
}

func TestLearningListRejectsBadInput(t *testing.T) {
	tests := []struct {
		name string
		url  string
		svc  *fakeLearningService
	}{
		{name: "unparsable limit", url: "/v1/learning?limit=many", svc: &fakeLearningService{}},
		{
			name: "rejected scope",
			url:  "/v1/learning?scope=yesterday",
			svc:  &fakeLearningService{err: fmt.Errorf("%w: unsupported scope", learning.ErrInvalidInput)},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler := NewLearning(tt.svc, slog.Default())
			recorder := httptest.NewRecorder()
			handler.List(recorder, httptest.NewRequest(http.MethodGet, tt.url, nil))
			if recorder.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, want %d (body %q)", recorder.Code, http.StatusBadRequest, recorder.Body.String())
			}
		})
	}
}

func TestLearningItemBodyCarriesCountsWithDerivedScores(t *testing.T) {
	graded := time.Date(2026, 7, 26, 9, 0, 0, 0, time.UTC)
	due := time.Date(2026, 7, 28, 9, 0, 0, 0, time.UTC)
	svc := &fakeLearningService{items: []learning.Item{{
		KnowledgeItemID: "k1", SurfaceText: "stale", LearnKind: "word", MeaningKo: "오래된",
		Status: learning.StatusActive, AskCount: 3, UnknownCount: 2,
		AttemptCount: 4, CorrectCount: 3, Accuracy: 0.75, WeaknessScore: 1.075,
		RegisteredAt: graded, LastGradedAt: graded, NextDueAt: due, CardCount: 2,
	}}}
	handler := NewLearning(svc, slog.Default())
	recorder := httptest.NewRecorder()
	handler.List(recorder, httptest.NewRequest(http.MethodGet, "/v1/learning", nil))

	var body struct {
		Items []map[string]any `json:"items"`
	}
	if err := json.NewDecoder(recorder.Body).Decode(&body); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	item := body.Items[0]
	if item["knowledge_item_id"] != "k1" || item["accuracy"] != 0.75 ||
		item["attempt_count"] != float64(4) || item["card_count"] != float64(2) {
		t.Fatalf("item = %#v", item)
	}
	if item["next_due_at"] == nil || item["last_graded_at"] == nil {
		t.Fatalf("timestamps missing: %#v", item)
	}
}

// An item nobody has reviewed has no last_graded_at and no due date. Sending the zero
// time would render as year 1 in the UI, so the fields are left out entirely.
func TestLearningItemBodyOmitsUnsetTimestamps(t *testing.T) {
	svc := &fakeLearningService{items: []learning.Item{{
		KnowledgeItemID: "k1", SurfaceText: "stale", Status: learning.StatusActive,
		RegisteredAt: time.Date(2026, 7, 26, 9, 0, 0, 0, time.UTC),
	}}}
	handler := NewLearning(svc, slog.Default())
	recorder := httptest.NewRecorder()
	handler.List(recorder, httptest.NewRequest(http.MethodGet, "/v1/learning", nil))

	body := recorder.Body.String()
	if strings.Contains(body, "last_graded_at") || strings.Contains(body, "next_due_at") {
		t.Fatalf("body = %q, want no unset timestamps", body)
	}
}

func TestLearningRetireAndRemove(t *testing.T) {
	tests := []struct {
		name     string
		call     func(*Learning, http.ResponseWriter, *http.Request)
		wantCall string
	}{
		{name: "retire", call: (*Learning).Retire, wantCall: "retire:k1"},
		{name: "remove", call: (*Learning).Remove, wantCall: "remove:k1"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := &fakeLearningService{item: learning.Item{KnowledgeItemID: "k1", Status: learning.StatusKnown}}
			handler := NewLearning(svc, slog.Default())
			recorder := httptest.NewRecorder()
			request := httptest.NewRequest(http.MethodPost, "/v1/learning/k1/retire", nil)
			request.SetPathValue("id", "k1")

			tt.call(handler, recorder, request)

			if recorder.Code != http.StatusOK || svc.lastCall != tt.wantCall {
				t.Fatalf("status = %d lastCall = %q", recorder.Code, svc.lastCall)
			}
		})
	}
}

func TestLearningRetireNotFound(t *testing.T) {
	svc := &fakeLearningService{err: learning.ErrItemNotFound}
	handler := NewLearning(svc, slog.Default())
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/v1/learning/missing/retire", nil)
	request.SetPathValue("id", "missing")

	handler.Retire(recorder, request)

	if recorder.Code != http.StatusNotFound || !strings.Contains(recorder.Body.String(), "learning item not found") {
		t.Fatalf("status = %d body = %q", recorder.Code, recorder.Body.String())
	}
}
