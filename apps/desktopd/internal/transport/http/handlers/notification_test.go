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
	recent       []notification.Notification
	recentLimit  int
	ackErr       error
	ackedID      string
	ackedCapture string
}

func (f *fakeNotificationService) Pending(context.Context) (notification.Pending, error) {
	return f.pending, nil
}

func (f *fakeNotificationService) Recent(_ context.Context, limit int) ([]notification.Notification, error) {
	f.recentLimit = limit
	return f.recent, nil
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

func TestNotificationHistoryOK(t *testing.T) {
	acked := time.Now().Add(-time.Hour)
	svc := &fakeNotificationService{recent: []notification.Notification{
		{ID: "n2", Kind: notification.KindReviewDue, Title: "복습", Body: "b", CreatedAt: time.Now()},
		{ID: "n1", Kind: notification.KindResultReady, Title: "준비", Body: "b", CreatedAt: acked, AckedAt: acked},
	}}
	handler := NewNotification(svc, slog.Default())
	recorder := httptest.NewRecorder()
	handler.History(recorder, httptest.NewRequest(http.MethodGet, "/v1/notifications/history?limit=10", nil))

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", recorder.Code)
	}
	if svc.recentLimit != 10 {
		t.Fatalf("recentLimit = %d, want 10", svc.recentLimit)
	}
	var body struct {
		Notifications []struct {
			ID    string `json:"id"`
			Acked bool   `json:"acked"`
		} `json:"notifications"`
		UnackedCount int `json:"unacked_count"`
	}
	if err := json.NewDecoder(recorder.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(body.Notifications) != 2 || body.UnackedCount != 1 {
		t.Fatalf("body = %#v", body)
	}
	if body.Notifications[0].ID != "n2" || body.Notifications[0].Acked {
		t.Fatalf("first (unacked, newest) = %#v", body.Notifications[0])
	}
	if !body.Notifications[1].Acked {
		t.Fatalf("second should be acked = %#v", body.Notifications[1])
	}
}

func TestNotificationHistoryInvalidLimit(t *testing.T) {
	handler := NewNotification(&fakeNotificationService{}, slog.Default())
	recorder := httptest.NewRecorder()
	handler.History(recorder, httptest.NewRequest(http.MethodGet, "/v1/notifications/history?limit=abc", nil))
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", recorder.Code)
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
