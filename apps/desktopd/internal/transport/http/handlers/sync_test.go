package handlers

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"neulsang/desktopd/internal/domain/outbox"
)

func TestSyncStatus(t *testing.T) {
	handler := NewSync(fakeSyncService{status: outbox.Status{Enabled: true, Pending: 3}}, slog.Default())
	recorder := httptest.NewRecorder()

	handler.Status(recorder, httptest.NewRequest(http.MethodGet, "/v1/sync/status", nil))

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusOK)
	}
	var body outbox.Status
	if err := json.NewDecoder(recorder.Body).Decode(&body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if !body.Enabled || body.Pending != 3 {
		t.Fatalf("body = %+v, want enabled true pending 3", body)
	}
}

type fakeSyncService struct {
	status outbox.Status
	err    error
}

func (s fakeSyncService) Status(context.Context) (outbox.Status, error) {
	return s.status, s.err
}
