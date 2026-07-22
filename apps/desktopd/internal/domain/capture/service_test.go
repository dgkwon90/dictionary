package capture

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"
)

type fakeRepository struct {
	saveNew func(context.Context, Capture, LookupJob, OutboxEvent) error
}

func (f fakeRepository) SaveNew(ctx context.Context, c Capture, j LookupJob, e OutboxEvent) error {
	return f.saveNew(ctx, c, j, e)
}

func TestServiceCreate(t *testing.T) {
	now := time.Date(2026, 7, 7, 10, 11, 12, 0, time.FixedZone("KST", 9*60*60))
	ids := []string{"capture-id", "job-id", "event-id"}
	var savedCapture Capture
	var savedJob LookupJob
	var savedEvent OutboxEvent
	svc := &Service{
		repo: fakeRepository{saveNew: func(_ context.Context, c Capture, j LookupJob, e OutboxEvent) error {
			savedCapture = c
			savedJob = j
			savedEvent = e
			return nil
		}},
		now: now.Local,
		newID: func() string {
			next := ids[0]
			ids = ids[1:]
			return next
		},
	}

	got, err := svc.Create(context.Background(), CreateInput{
		Text:       "  hello  ",
		InputMode:  "clipboard",
		SourceApp:  "Safari",
		SourceType: "browser",
	})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if got != (CreateResult{CaptureID: "capture-id", LookupJobID: "job-id", Status: "queued"}) {
		t.Fatalf("Create() = %#v", got)
	}
	if savedCapture.SelectedText != "hello" {
		t.Errorf("SelectedText = %q, want trimmed text", savedCapture.SelectedText)
	}
	sum := sha256.Sum256([]byte("hello"))
	if savedCapture.TextHash != hex.EncodeToString(sum[:]) {
		t.Errorf("TextHash = %q", savedCapture.TextHash)
	}
	if savedCapture.InboxStatus != "new" {
		t.Errorf("InboxStatus = %q, want new", savedCapture.InboxStatus)
	}
	if savedCapture.CreatedAt.Location() != time.UTC {
		t.Errorf("CreatedAt location = %v, want UTC", savedCapture.CreatedAt.Location())
	}
	if savedJob.CaptureID != savedCapture.ID || savedJob.Status != "queued" {
		t.Errorf("LookupJob = %#v", savedJob)
	}
	if savedEvent.EventID != "event-id" || savedEvent.AggregateType != "capture" || savedEvent.AggregateID != savedCapture.ID || savedEvent.EventType != "capture_created" {
		t.Errorf("OutboxEvent = %#v", savedEvent)
	}
	var payload Capture
	if err := json.Unmarshal([]byte(savedEvent.PayloadJSON), &payload); err != nil {
		t.Fatalf("unmarshal payload: %v", err)
	}
	if payload.ID != savedCapture.ID || payload.SelectedText != savedCapture.SelectedText {
		t.Errorf("payload = %#v, capture = %#v", payload, savedCapture)
	}
}

func TestServiceCreateInvalidInput(t *testing.T) {
	tests := []struct {
		name  string
		input CreateInput
	}{
		{name: "empty text", input: CreateInput{Text: "  ", InputMode: "manual"}},
		{name: "bad input mode", input: CreateInput{Text: "hello", InputMode: "bad"}},
		{name: "bad source type", input: CreateInput{Text: "hello", InputMode: "manual", SourceType: "bad"}},
		{name: "text exceeds max length", input: CreateInput{Text: strings.Repeat("a", MaxTextLength+1), InputMode: "manual"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := NewService(fakeRepository{saveNew: func(context.Context, Capture, LookupJob, OutboxEvent) error {
				t.Fatal("SaveNew should not be called")
				return nil
			}})
			_, err := svc.Create(context.Background(), tt.input)
			if !errors.Is(err, ErrInvalidInput) {
				t.Fatalf("Create() error = %v, want ErrInvalidInput", err)
			}
		})
	}
}

func TestServiceCreatePropagatesRepositoryError(t *testing.T) {
	repoErr := errors.New("database failed")
	svc := NewService(fakeRepository{saveNew: func(context.Context, Capture, LookupJob, OutboxEvent) error {
		return repoErr
	}})

	_, err := svc.Create(context.Background(), CreateInput{Text: "hello", InputMode: "manual"})
	if !errors.Is(err, repoErr) {
		t.Fatalf("Create() error = %v, want repo error", err)
	}
}
