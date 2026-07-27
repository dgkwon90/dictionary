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
	"neulsang/desktopd/internal/domain/learning"
)

type fakeKnowledgeService struct {
	listByCapture func(context.Context, string) ([]knowledge.CaptureItem, error)
}

func (f fakeKnowledgeService) ListByCapture(ctx context.Context, captureID string) ([]knowledge.CaptureItem, error) {
	return f.listByCapture(ctx, captureID)
}

func TestKnowledgeListByCaptureOK(t *testing.T) {
	handler := NewKnowledge(fakeKnowledgeService{listByCapture: func(_ context.Context, captureID string) ([]knowledge.CaptureItem, error) {
		if captureID != "cap-1" {
			t.Fatalf("captureID = %q", captureID)
		}
		return []knowledge.CaptureItem{
			{KnowledgeItemID: "k1", SurfaceText: "stale", ItemType: "word", MeaningKo: "오래된", Status: learning.StatusActive, UnknownCount: 2},
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
			UnknownCount    int    `json:"unknown_count"`
		} `json:"items"`
	}
	if err := json.NewDecoder(recorder.Body).Decode(&body); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if body.CaptureID != "cap-1" || len(body.Items) != 1 || body.Items[0].KnowledgeItemID != "k1" || body.Items[0].UnknownCount != 2 {
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
