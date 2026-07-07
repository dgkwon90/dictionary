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
	"time"

	"neulsang/desktopd/internal/domain/inbox"
)

type fakeInboxService struct {
	list      func(context.Context, inbox.ListInput) ([]inbox.Item, error)
	setStatus func(context.Context, string, string) error
}

func (f fakeInboxService) List(ctx context.Context, input inbox.ListInput) ([]inbox.Item, error) {
	return f.list(ctx, input)
}

func (f fakeInboxService) SetStatus(ctx context.Context, captureID, status string) error {
	return f.setStatus(ctx, captureID, status)
}

func TestInboxListOK(t *testing.T) {
	createdAt := time.Date(2026, 7, 7, 1, 2, 3, 0, time.UTC)
	handler := NewInbox(fakeInboxService{list: func(_ context.Context, input inbox.ListInput) ([]inbox.Item, error) {
		if input.Status != "new" || input.Limit != 25 {
			t.Fatalf("input = %#v", input)
		}
		return []inbox.Item{{
			CaptureID:    "capture-id",
			SelectedText: "hello",
			SourceApp:    "Safari",
			SourceType:   "browser",
			InputMode:    "manual",
			CreatedAt:    createdAt,
			JobStatus:    "done",
			BriefKo:      "brief",
			Status:       "new",
		}}, nil
	}}, slog.Default())
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/v1/inbox?status=new&limit=25", nil)

	handler.List(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusOK)
	}
	var body struct {
		Items []map[string]any `json:"items"`
	}
	if err := json.NewDecoder(recorder.Body).Decode(&body); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if len(body.Items) != 1 {
		t.Fatalf("items = %#v", body.Items)
	}
	item := body.Items[0]
	if item["capture_id"] != "capture-id" || item["selected_text"] != "hello" || item["source_app"] != "Safari" || item["source_type"] != "browser" || item["input_mode"] != "manual" || item["status"] != "new" || item["job_status"] != "done" || item["brief_ko"] != "brief" || item["created_at"] == "" {
		t.Fatalf("item = %#v", item)
	}
}

func TestInboxListInvalidLimit(t *testing.T) {
	handler := NewInbox(fakeInboxService{list: func(context.Context, inbox.ListInput) ([]inbox.Item, error) {
		t.Fatal("List should not be called")
		return nil, nil
	}}, slog.Default())
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/v1/inbox?limit=bad", nil)

	handler.List(recorder, request)

	if recorder.Code != http.StatusBadRequest || !strings.Contains(recorder.Body.String(), "invalid limit") {
		t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
	}
}

func TestInboxListInvalidInput(t *testing.T) {
	handler := NewInbox(fakeInboxService{list: func(context.Context, inbox.ListInput) ([]inbox.Item, error) {
		return nil, inbox.ErrInvalidInput
	}}, slog.Default())
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/v1/inbox?status=bad", nil)

	handler.List(recorder, request)

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusBadRequest)
	}
}

func TestInboxListInternalError(t *testing.T) {
	handler := NewInbox(fakeInboxService{list: func(context.Context, inbox.ListInput) ([]inbox.Item, error) {
		return nil, errors.New("secret database detail")
	}}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/v1/inbox", nil)

	handler.List(recorder, request)

	if recorder.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusInternalServerError)
	}
	if strings.Contains(recorder.Body.String(), "secret") {
		t.Fatalf("body exposes internal detail: %s", recorder.Body.String())
	}
}

func TestInboxSaveArchiveOK(t *testing.T) {
	tests := []struct {
		name        string
		status      string
		callHandler func(*Inbox, http.ResponseWriter, *http.Request)
	}{
		{name: "save", status: "saved", callHandler: (*Inbox).Save},
		{name: "archive", status: "archived", callHandler: (*Inbox).Archive},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler := NewInbox(fakeInboxService{setStatus: func(_ context.Context, captureID, status string) error {
				if captureID != "capture-id" || status != tt.status {
					t.Fatalf("SetStatus(%q, %q)", captureID, status)
				}
				return nil
			}}, slog.Default())
			recorder := httptest.NewRecorder()
			request := httptest.NewRequest(http.MethodPost, "/v1/inbox/capture-id/"+tt.name, nil)
			request.SetPathValue("id", "capture-id")

			tt.callHandler(handler, recorder, request)

			if recorder.Code != http.StatusOK {
				t.Fatalf("status = %d, want %d", recorder.Code, http.StatusOK)
			}
			var body map[string]string
			if err := json.NewDecoder(recorder.Body).Decode(&body); err != nil {
				t.Fatalf("decode body: %v", err)
			}
			if body["capture_id"] != "capture-id" || body["status"] != tt.status {
				t.Fatalf("body = %#v", body)
			}
		})
	}
}

func TestInboxSaveArchiveNotFound(t *testing.T) {
	handler := NewInbox(fakeInboxService{setStatus: func(context.Context, string, string) error {
		return inbox.ErrCaptureNotFound
	}}, slog.Default())
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/v1/inbox/missing/save", nil)
	request.SetPathValue("id", "missing")

	handler.Save(recorder, request)

	if recorder.Code != http.StatusNotFound || !strings.Contains(recorder.Body.String(), "capture not found") {
		t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
	}
}

func TestInboxSaveArchiveInternalError(t *testing.T) {
	handler := NewInbox(fakeInboxService{setStatus: func(context.Context, string, string) error {
		return errors.New("secret database detail")
	}}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/v1/inbox/capture-id/archive", nil)
	request.SetPathValue("id", "capture-id")

	handler.Archive(recorder, request)

	if recorder.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusInternalServerError)
	}
	if strings.Contains(recorder.Body.String(), "secret") {
		t.Fatalf("body exposes internal detail: %s", recorder.Body.String())
	}
}
