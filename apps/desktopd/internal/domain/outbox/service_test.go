package outbox

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"slices"
	"strconv"
	"testing"
	"time"
)

func TestServiceFlushPublishesAndMarksAllBatches(t *testing.T) {
	repo := &fakeRepository{events: makeOutboxEvents(205), pending: 205}
	publisher := &fakePublisher{}
	service := NewService(repo, publisher, testLogger())

	acked, err := service.Flush(context.Background())
	if err != nil {
		t.Fatalf("Flush() error = %v", err)
	}
	if acked != 205 {
		t.Fatalf("Flush() acked = %d, want 205", acked)
	}
	if len(repo.events) != 0 {
		t.Fatalf("remaining events = %d, want 0", len(repo.events))
	}
	if got, want := publisher.batchSizes, []int{100, 100, 5}; !slices.Equal(got, want) {
		t.Fatalf("publish batch sizes = %v, want %v", got, want)
	}
	if len(repo.marked) != 205 {
		t.Fatalf("marked = %d, want 205", len(repo.marked))
	}
	for _, at := range repo.markedAt {
		if at.Location() != time.UTC {
			t.Fatalf("mark time location = %v, want UTC", at.Location())
		}
	}
}

func TestServiceFlushStopsOnPublishErrorWithoutMarking(t *testing.T) {
	publishErr := errors.New("remote down")
	repo := &fakeRepository{events: makeOutboxEvents(3), pending: 3}
	publisher := &fakePublisher{err: publishErr}
	service := NewService(repo, publisher, testLogger())

	acked, err := service.Flush(context.Background())
	if !errors.Is(err, publishErr) {
		t.Fatalf("Flush() error = %v, want %v", err, publishErr)
	}
	if acked != 0 {
		t.Fatalf("Flush() acked = %d, want 0", acked)
	}
	if len(repo.marked) != 0 {
		t.Fatalf("marked = %v, want none", repo.marked)
	}
	if len(repo.events) != 3 {
		t.Fatalf("remaining events = %d, want 3", len(repo.events))
	}
}

func TestServiceFlushDisabledNoopAndStatus(t *testing.T) {
	repo := &fakeRepository{events: makeOutboxEvents(2), pending: 2}
	service := NewService(repo, nil, testLogger())

	acked, err := service.Flush(context.Background())
	if err != nil {
		t.Fatalf("Flush() error = %v", err)
	}
	if acked != 0 {
		t.Fatalf("Flush() acked = %d, want 0", acked)
	}
	if repo.listCalls != 0 {
		t.Fatalf("ListUnsent calls = %d, want 0", repo.listCalls)
	}

	status, err := service.Status(context.Background())
	if err != nil {
		t.Fatalf("Status() error = %v", err)
	}
	if status.Enabled {
		t.Fatalf("Status().Enabled = true, want false")
	}
	if status.Pending != 2 {
		t.Fatalf("Status().Pending = %d, want 2", status.Pending)
	}
}

type fakeRepository struct {
	events    []Event
	pending   int
	listCalls int
	marked    []string
	markedAt  []time.Time
}

func (r *fakeRepository) ListUnsent(_ context.Context, limit int) ([]Event, error) {
	r.listCalls++
	if len(r.events) < limit {
		limit = len(r.events)
	}
	out := make([]Event, limit)
	copy(out, r.events[:limit])
	return out, nil
}

func (r *fakeRepository) MarkAcked(_ context.Context, eventIDs []string, at time.Time) error {
	r.marked = append(r.marked, eventIDs...)
	r.markedAt = append(r.markedAt, at)
	acked := make(map[string]struct{}, len(eventIDs))
	for _, eventID := range eventIDs {
		acked[eventID] = struct{}{}
	}
	remaining := r.events[:0]
	for _, event := range r.events {
		if _, ok := acked[event.EventID]; !ok {
			remaining = append(remaining, event)
		}
	}
	r.events = remaining
	r.pending = len(remaining)
	return nil
}

func (r *fakeRepository) PendingCount(context.Context) (int, error) {
	return r.pending, nil
}

type fakePublisher struct {
	err        error
	batchSizes []int
}

func (p *fakePublisher) Publish(_ context.Context, events []Event) error {
	p.batchSizes = append(p.batchSizes, len(events))
	return p.err
}

func makeOutboxEvents(count int) []Event {
	events := make([]Event, 0, count)
	for i := range count {
		eventID := "event-" + strconv.Itoa(i+1)
		events = append(events, Event{
			ID:            int64(i + 1),
			EventID:       eventID,
			AggregateType: "capture",
			AggregateID:   "capture-" + eventID,
			EventType:     "capture_created",
			PayloadJSON:   `{}`,
			CreatedAt:     time.Date(2026, 7, 17, 1, 0, i, 0, time.UTC),
		})
	}
	return events
}

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}
