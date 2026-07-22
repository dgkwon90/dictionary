package syncpush

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"neulsang/desktopd/internal/domain/outbox"
)

func TestClientPublishPostsEvents(t *testing.T) {
	var gotPath string
	var gotContentType string
	var gotBody struct {
		Events []struct {
			EventID       string    `json:"event_id"`
			AggregateType string    `json:"aggregate_type"`
			AggregateID   string    `json:"aggregate_id"`
			EventType     string    `json:"event_type"`
			PayloadJSON   string    `json:"payload_json"`
			CreatedAt     time.Time `json:"created_at"`
		} `json:"events"`
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotContentType = r.Header.Get("Content-Type")
		if r.Method != http.MethodPost {
			t.Errorf("method = %s, want POST", r.Method)
		}
		if err := json.NewDecoder(r.Body).Decode(&gotBody); err != nil {
			t.Errorf("decode request body: %v", err)
		}
		w.WriteHeader(http.StatusAccepted)
	}))
	defer server.Close()

	createdAt := time.Date(2026, 7, 17, 1, 2, 3, 0, time.UTC)
	client := NewClient(server.URL + "/sync")
	err := client.Publish(context.Background(), []outbox.Event{{
		EventID:       "event-1",
		AggregateType: "capture",
		AggregateID:   "capture-1",
		EventType:     "capture_created",
		PayloadJSON:   `{"id":"capture-1"}`,
		CreatedAt:     createdAt,
	}})
	if err != nil {
		t.Fatalf("Publish() error = %v", err)
	}
	if gotPath != "/sync" {
		t.Fatalf("path = %q, want /sync", gotPath)
	}
	if gotContentType != "application/json" {
		t.Fatalf("Content-Type = %q, want application/json", gotContentType)
	}
	if len(gotBody.Events) != 1 {
		t.Fatalf("events = %d, want 1", len(gotBody.Events))
	}
	event := gotBody.Events[0]
	if event.EventID != "event-1" || event.AggregateType != "capture" || event.AggregateID != "capture-1" ||
		event.EventType != "capture_created" || event.PayloadJSON != `{"id":"capture-1"}` || !event.CreatedAt.Equal(createdAt) {
		t.Fatalf("posted event = %+v", event)
	}
}

func TestClientPublishReturnsErrorForNon2xx(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	err := NewClient(server.URL).Publish(context.Background(), []outbox.Event{{EventID: "event-1"}})
	if err == nil {
		t.Fatalf("Publish() error = nil, want error")
	}
	if !strings.Contains(err.Error(), "500") {
		t.Fatalf("Publish() error = %q, want status", err.Error())
	}
}
