package handlers

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"neulsang/desktopd/internal/domain/notification"
)

type fakeNotificationService struct {
	pending      notification.Pending
	ackErr       error
	ackedID      string
	ackedCapture string
}

func (f *fakeNotificationService) Pending(context.Context) (notification.Pending, error) {
	return f.pending, nil
}

func (f *fakeNotificationService) Ack(_ context.Context, id string) error {
	f.ackedID = id
	return f.ackErr
}

func (f *fakeNotificationService) AckCapture(_ context.Context, captureID string) error {
	f.ackedCapture = captureID
	return f.ackErr
}

func TestNotificationListOK(t *testing.T) {
	svc := &fakeNotificationService{pending: notification.Pending{
		Notifications: []notification.Notification{
			{ID: "n1", Kind: notification.KindResultReady, Title: "준비 완료", Body: "b", Route: "Inbox", PayloadID: "cap-1", CreatedAt: time.Now()},
		},
		UnackedCount: 1,
	}}
	handler := NewNotification(svc, slog.Default())
	recorder := httptest.NewRecorder()
	handler.List(recorder, httptest.NewRequest(http.MethodGet, "/v1/notifications", nil))

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", recorder.Code)
	}
	var body struct {
		Notifications []struct {
			ID    string `json:"id"`
			Kind  string `json:"kind"`
			Route string `json:"route"`
		} `json:"notifications"`
		UnackedCount int `json:"unacked_count"`
	}
	if err := json.NewDecoder(recorder.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.UnackedCount != 1 || len(body.Notifications) != 1 {
		t.Fatalf("body = %#v", body)
	}
	if body.Notifications[0].ID != "n1" || body.Notifications[0].Route != "Inbox" {
		t.Fatalf("notification = %#v", body.Notifications[0])
	}
}

func TestNotificationAckOK(t *testing.T) {
	svc := &fakeNotificationService{}
	handler := NewNotification(svc, slog.Default())
	recorder := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/notifications/n1/ack", nil)
	req.SetPathValue("id", "n1")
	handler.Ack(recorder, req)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", recorder.Code)
	}
	if svc.ackedID != "n1" {
		t.Fatalf("acked id = %q, want n1", svc.ackedID)
	}
}

func TestNotificationAckByCaptureOK(t *testing.T) {
	svc := &fakeNotificationService{}
	handler := NewNotification(svc, slog.Default())
	recorder := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/captures/cap-1/notification-ack", nil)
	req.SetPathValue("id", "cap-1")
	handler.AckByCapture(recorder, req)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", recorder.Code)
	}
	if svc.ackedCapture != "cap-1" {
		t.Fatalf("acked capture = %q, want cap-1", svc.ackedCapture)
	}
}

func TestNotificationAckNotFound(t *testing.T) {
	svc := &fakeNotificationService{ackErr: notification.ErrNotFound}
	handler := NewNotification(svc, slog.Default())
	recorder := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/notifications/missing/ack", nil)
	req.SetPathValue("id", "missing")
	handler.Ack(recorder, req)

	if recorder.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", recorder.Code)
	}
}
