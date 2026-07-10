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
	markUnknown   func(context.Context, string) (knowledge.MarkResult, error)
	markKnown     func(context.Context, string) (knowledge.MarkResult, error)
	listByCapture func(context.Context, string) ([]knowledge.CaptureItem, error)
}

func (f fakeKnowledgeService) MarkUnknown(ctx context.Context, id string) (knowledge.MarkResult, error) {
	return f.markUnknown(ctx, id)
}

func (f fakeKnowledgeService) MarkKnown(ctx context.Context, id string) (knowledge.MarkResult, error) {
	return f.markKnown(ctx, id)
}

func (f fakeKnowledgeService) ListByCapture(ctx context.Context, captureID string) ([]knowledge.CaptureItem, error) {
	return f.listByCapture(ctx, captureID)
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

func TestKnowledgeListByCaptureOK(t *testing.T) {
	handler := NewKnowledge(fakeKnowledgeService{listByCapture: func(_ context.Context, captureID string) ([]knowledge.CaptureItem, error) {
		if captureID != "cap-1" {
			t.Fatalf("captureID = %q", captureID)
		}
		return []knowledge.CaptureItem{
			{KnowledgeItemID: "k1", SurfaceText: "stale", ItemType: "word", MeaningKo: "오래된", Status: knowledge.StatusActive, WrongCount: 2},
		}, nil
	}}, slog.Default())
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/v1/captures/cap-1/knowledge", nil)
	request.SetPathValue("id", "cap-1")

	handler.ListByCapture(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusOK)
	}
	var body struct {
		CaptureID string `json:"capture_id"`
		Items     []struct {
			KnowledgeItemID string `json:"knowledge_item_id"`
			SurfaceText     string `json:"surface_text"`
			Status          string `json:"status"`
			WrongCount      int    `json:"wrong_count"`
		} `json:"items"`
	}
	if err := json.NewDecoder(recorder.Body).Decode(&body); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if body.CaptureID != "cap-1" || len(body.Items) != 1 || body.Items[0].KnowledgeItemID != "k1" || body.Items[0].WrongCount != 2 {
		t.Fatalf("body = %#v", body)
	}
}

func TestKnowledgeListByCaptureNotFound(t *testing.T) {
	handler := NewKnowledge(fakeKnowledgeService{listByCapture: func(_ context.Context, _ string) ([]knowledge.CaptureItem, error) {
		return nil, knowledge.ErrCaptureNotFound
	}}, slog.Default())
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/v1/captures/missing/knowledge", nil)
	request.SetPathValue("id", "missing")

	handler.ListByCapture(recorder, request)

	if recorder.Code != http.StatusNotFound || !strings.Contains(recorder.Body.String(), "capture not found") {
		t.Fatalf("status = %d body = %q", recorder.Code, recorder.Body.String())
	}
}
