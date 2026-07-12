package notification

import (
	"context"
	"errors"
	"testing"
	"time"
)

type fakeRepo struct {
	unacked  []Notification
	ackID    string
	ackFound bool
	ackErr   error
	enqueued []Notification
}

func (f *fakeRepo) Enqueue(_ context.Context, n Notification) error {
	f.enqueued = append(f.enqueued, n)
	return nil
}

func (f *fakeRepo) ListUnacked(context.Context, time.Time) ([]Notification, error) {
	return f.unacked, nil
}

func (f *fakeRepo) Ack(_ context.Context, id string) (bool, error) {
	f.ackID = id
	return f.ackFound, f.ackErr
}

func TestServicePending(t *testing.T) {
	repo := &fakeRepo{unacked: []Notification{
		{ID: "n1", Kind: KindResultReady},
		{ID: "n2", Kind: KindReviewDue},
	}}
	svc := NewService(repo)
	got, err := svc.Pending(context.Background())
	if err != nil {
		t.Fatalf("Pending() error = %v", err)
	}
	if got.UnackedCount != 2 || len(got.Notifications) != 2 {
		t.Fatalf("Pending() = %+v, want 2 notifications/count", got)
	}
}

func TestServiceAckFound(t *testing.T) {
	repo := &fakeRepo{ackFound: true}
	if err := NewService(repo).Ack(context.Background(), "n1"); err != nil {
		t.Fatalf("Ack() error = %v", err)
	}
	if repo.ackID != "n1" {
		t.Fatalf("Ack() id = %q, want n1", repo.ackID)
	}
}

func TestServiceAckNotFound(t *testing.T) {
	repo := &fakeRepo{ackFound: false}
	if err := NewService(repo).Ack(context.Background(), "missing"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("Ack() error = %v, want ErrNotFound", err)
	}
}
