package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"neulsang/desktopd/internal/domain/explain"
)

type fakeExplanationReader struct {
	getSnapshot func(context.Context, string) (explain.Snapshot, error)
}

func (f fakeExplanationReader) GetSnapshot(ctx context.Context, captureID string) (explain.Snapshot, error) {
	return f.getSnapshot(ctx, captureID)
}

func TestExplanationGetStatuses(t *testing.T) {
	tests := []struct {
		name       string
		snapshot   explain.Snapshot
		assertBody func(*testing.T, map[string]any)
	}{
		{
			name:     "queued",
			snapshot: explain.Snapshot{Status: "queued"},
			assertBody: func(t *testing.T, body map[string]any) {
				t.Helper()
				if _, ok := body["explanation"]; ok {
					t.Fatalf("explanation present: %#v", body)
				}
			},
		},
		{
			name:     "running",
			snapshot: explain.Snapshot{Status: "running"},
			assertBody: func(t *testing.T, body map[string]any) {
				t.Helper()
				if _, ok := body["error_message"]; ok {
					t.Fatalf("error_message present: %#v", body)
				}
			},
		},
		{
			name: "done",
			snapshot: explain.Snapshot{Status: "done", Result: &explain.ExplainResult{
				BriefKo:         "brief",
				DetailedKo:      "detailed",
				PronunciationKo: "pronunciation",
				DomainCategory:  "general",
				Difficulty:      0.5,
				Examples:        []explain.Example{{English: "hello", Korean: "안녕", Note: "note"}},
				SubItems:        []explain.SubItem{{SurfaceText: "hello", NormalizedKey: "hello", ItemType: "word", MeaningKo: "meaning", PronunciationKo: "pronunciation", Importance: 0.5}},
			}},
			assertBody: func(t *testing.T, body map[string]any) {
				t.Helper()
				explanationBody, ok := body["explanation"].(map[string]any)
				if !ok || explanationBody["brief_ko"] != "brief" || explanationBody["domain_category"] != "general" {
					t.Fatalf("explanation = %#v", body["explanation"])
				}
			},
		},
		{
			name:     "failed",
			snapshot: explain.Snapshot{Status: "failed", ErrorMessage: "provider failed"},
			assertBody: func(t *testing.T, body map[string]any) {
				t.Helper()
				if body["error_message"] != "provider failed" {
					t.Fatalf("error_message = %#v", body["error_message"])
				}
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler := NewExplanation(fakeExplanationReader{getSnapshot: func(_ context.Context, captureID string) (explain.Snapshot, error) {
				if captureID != "capture-id" {
					t.Fatalf("captureID = %q, want capture-id", captureID)
				}
				return tt.snapshot, nil
			}}, slog.Default())
			recorder := httptest.NewRecorder()
			request := httptest.NewRequest(http.MethodGet, "/v1/captures/capture-id/explanation", nil)
			request.SetPathValue("id", "capture-id")

			handler.Get(recorder, request)

			if recorder.Code != http.StatusOK {
				t.Fatalf("status = %d, want %d", recorder.Code, http.StatusOK)
			}
			var body map[string]any
			if err := json.NewDecoder(recorder.Body).Decode(&body); err != nil {
				t.Fatalf("decode body: %v", err)
			}
			if body["capture_id"] != "capture-id" || body["status"] != tt.snapshot.Status {
				t.Fatalf("body = %#v", body)
			}
			tt.assertBody(t, body)
		})
	}
}

func TestExplanationGetNotFound(t *testing.T) {
	handler := NewExplanation(fakeExplanationReader{getSnapshot: func(context.Context, string) (explain.Snapshot, error) {
		return explain.Snapshot{}, explain.ErrCaptureNotFound
	}}, slog.Default())
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/v1/captures/missing/explanation", nil)
	request.SetPathValue("id", "missing")

	handler.Get(recorder, request)

	if recorder.Code != http.StatusNotFound || !strings.Contains(recorder.Body.String(), "capture not found") {
		t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
	}
}

func TestExplanationGetInternalError(t *testing.T) {
	handler := NewExplanation(fakeExplanationReader{getSnapshot: func(context.Context, string) (explain.Snapshot, error) {
		return explain.Snapshot{}, errors.New("secret database detail")
	}}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/v1/captures/capture-id/explanation", nil)
	request.SetPathValue("id", "capture-id")

	handler.Get(recorder, request)

	if recorder.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusInternalServerError)
	}
	if strings.Contains(recorder.Body.String(), "secret") {
		t.Fatalf("body exposes internal detail: %s", recorder.Body.String())
	}
}
