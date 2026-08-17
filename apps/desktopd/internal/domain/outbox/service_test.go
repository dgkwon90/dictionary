package outbox

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"slices"
	"strconv"
	"strings"
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
	failed    []string
	reasons   []string
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

func (r *fakeRepository) MarkFailed(_ context.Context, eventID string, _ time.Time, reason string) error {
	r.failed = append(r.failed, eventID)
	r.reasons = append(r.reasons, reason)
	remaining := r.events[:0]
	for _, event := range r.events {
		if event.EventID != eventID {
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

func (r *fakeRepository) FailedCount(context.Context) (int, error) {
	return len(r.failed), nil
}

type fakePublisher struct {
	err        error
	batchSizes []int
	// reject names the events this publisher refuses permanently; everything else
	// succeeds. Empty means "behave according to err".
	reject map[string]error
}

func (p *fakePublisher) Publish(_ context.Context, events []Event) error {
	p.batchSizes = append(p.batchSizes, len(events))
	if p.reject != nil {
		for _, event := range events {
			if err, bad := p.reject[event.EventID]; bad {
				return err
			}
		}
		return nil
	}
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

// 서버가 영원히 거절하는 이벤트 하나가 그 뒤의 정상 이벤트를 전부 막아서는 안 된다.
// 아웃박스는 오래된 것부터 보내므로, 격리가 없으면 큐는 그 지점에서 영구히 멈춘다.
func TestServiceFlushQuarantinesPermanentlyRejectedEventAndSendsTheRest(t *testing.T) {
	events := makeOutboxEvents(3)
	repo := &fakeRepository{events: events, pending: len(events)}
	publisher := &fakePublisher{reject: map[string]error{
		"event-2": fmt.Errorf("post sync events: %w: status 400 Bad Request", ErrPermanent),
	}}
	svc := NewService(repo, publisher, testLogger())

	acked, err := svc.Flush(context.Background())
	if err != nil {
		t.Fatalf("Flush() error = %v", err)
	}

	if acked != 2 {
		t.Errorf("acked = %d, want 2 (the two good events)", acked)
	}
	if len(repo.failed) != 1 || repo.failed[0] != "event-2" {
		t.Fatalf("quarantined = %v, want [event-2]", repo.failed)
	}
	if got := strings.Join(repo.marked, ","); got != "event-1,event-3" {
		t.Errorf("acked events = %q, want event-1,event-3 — the good ones must not be held back", got)
	}
	// 배치 실패 후에는 하나씩 보내 누가 문제인지 서버에게 물어본다.
	if len(publisher.batchSizes) != 4 || publisher.batchSizes[0] != 3 {
		t.Errorf("batch sizes = %v, want [3 1 1 1]", publisher.batchSizes)
	}
	if repo.reasons[0] == "" {
		t.Error("quarantine reason is empty — the ledger must say why it was rejected")
	}
}

// 일시적 실패는 격리 대상이 아니다. 서버가 잠깐 죽은 것으로 이벤트를 버리면 데이터가
// 조용히 사라진다 — 큐에 그대로 두고 다음 틱에 다시 보낸다.
func TestServiceFlushKeepsTransientFailuresQueued(t *testing.T) {
	repo := &fakeRepository{events: makeOutboxEvents(2), pending: 2}
	publisher := &fakePublisher{err: errors.New("post sync events: status 503 Service Unavailable")}
	svc := NewService(repo, publisher, testLogger())

	acked, err := svc.Flush(context.Background())
	if err == nil {
		t.Fatal("Flush() error = nil, want the transient failure surfaced")
	}
	if acked != 0 || len(repo.failed) != 0 || len(repo.marked) != 0 {
		t.Fatalf("acked=%d quarantined=%v marked=%v — nothing may change on a transient failure",
			acked, repo.failed, repo.marked)
	}
}

// 격리된 것이 있으면 상태에 드러나야 한다. pending만 보고하면 큐가 비어 보이는데
// 실제로는 사람이 봐야 할 이벤트가 남아 있는 상태가 된다.
func TestServiceStatusReportsQuarantined(t *testing.T) {
	repo := &fakeRepository{events: makeOutboxEvents(1), pending: 1}
	publisher := &fakePublisher{reject: map[string]error{
		"event-1": fmt.Errorf("%w: status 422", ErrPermanent),
	}}
	svc := NewService(repo, publisher, testLogger())
	if _, err := svc.Flush(context.Background()); err != nil {
		t.Fatalf("Flush() error = %v", err)
	}

	status, err := svc.Status(context.Background())
	if err != nil {
		t.Fatalf("Status() error = %v", err)
	}
	if status.Pending != 0 || status.Failed != 1 {
		t.Fatalf("status = %+v, want pending 0 / failed 1", status)
	}
}
