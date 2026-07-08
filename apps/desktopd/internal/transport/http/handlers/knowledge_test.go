package handlers

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"neulsang/desktopd/internal/domain/knowledge"
)

type fakeKnowledgeService struct {
	markUnknown func(context.Context, string) (knowledge.MarkResult, error)
	markKnown   func(context.Context, string) (knowledge.MarkResult, error)
}

func (f fakeKnowledgeService) MarkUnknown(ctx context.Context, id string) (knowledge.MarkResult, error) {
	return f.markUnknown(ctx, id)
}

func (f fakeKnowledgeService) MarkKnown(ctx context.Context, id string) (knowledge.MarkResult, error) {
	return f.markKnown(ctx, id)
}

func TestKnowledgeMarkUnknownOK(t *testing.T) {
	handler := NewKnowledge(fakeKnowledgeService{markUnknown: func(_ context.Context, id string) (knowledge.MarkResult, error) {
		if id != "item-1" {
			t.Fatalf("id = %q", id)
		}
		return knowledge.MarkResult{KnowledgeItemID: id, Status: knowledge.StatusActive, AskCount: 2, WrongCount: 1, CandidateCount: 3, CardsCreated: 3}, nil
	}}, slog.Default())
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/v1/knowledge/item-1/mark-unknown", nil)
	request.SetPathValue("id", "item-1")

	handler.MarkUnknown(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusOK)
	}
	var body map[string]any
	if err := json.NewDecoder(recorder.Body).Decode(&body); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if body["knowledge_item_id"] != "item-1" || body["status"] != knowledge.StatusActive || body["wrong_count"] != float64(1) || body["candidate_count"] != float64(3) || body["cards_created"] != float64(3) {
		t.Fatalf("body = %#v", body)
	}
}

func TestKnowledgeMarkKnownOK(t *testing.T) {
	handler := NewKnowledge(fakeKnowledgeService{markKnown: func(_ context.Context, id string) (knowledge.MarkResult, error) {
		return knowledge.MarkResult{KnowledgeItemID: id, Status: knowledge.StatusKnown}, nil
	}}, slog.Default())
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/v1/knowledge/item-1/mark-known", nil)
	request.SetPathValue("id", "item-1")

	handler.MarkKnown(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusOK)
	}
	if !strings.Contains(recorder.Body.String(), `"status":"known"`) {
		t.Fatalf("body = %q", recorder.Body.String())
	}
}

func TestKnowledgeMarkUnknownNotFound(t *testing.T) {
	handler := NewKnowledge(fakeKnowledgeService{markUnknown: func(_ context.Context, _ string) (knowledge.MarkResult, error) {
		return knowledge.MarkResult{}, knowledge.ErrKnowledgeItemNotFound
	}}, slog.Default())
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/v1/knowledge/missing/mark-unknown", nil)
	request.SetPathValue("id", "missing")

	handler.MarkUnknown(recorder, request)

	if recorder.Code != http.StatusNotFound || !strings.Contains(recorder.Body.String(), "knowledge item not found") {
		t.Fatalf("status = %d body = %q", recorder.Code, recorder.Body.String())
	}
}
