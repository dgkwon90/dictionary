package notification

import (
	"context"
	"errors"
	"testing"
	"time"

	"neulsang/desktopd/internal/domain/settings"
)

type fakeRepo struct {
	unacked   []Notification
	ackID     string
	ackFound  bool
	ackErr    error
	enqueued  []Notification
	ackedCapt string
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

func (f *fakeRepo) AckByCapture(_ context.Context, captureID string) error {
	f.ackedCapt = captureID
	return nil
}

func enabled() PreferencesReader {
	return fakePrefs{prefs: enabledPrefs("09:00", "21:00")}
}

func TestServicePending(t *testing.T) {
	repo := &fakeRepo{unacked: []Notification{
		{ID: "n1", Kind: KindResultReady},
		{ID: "n2", Kind: KindReviewDue},
	}}
	svc := NewService(repo, enabled())
	got, err := svc.Pending(context.Background())
	if err != nil {
		t.Fatalf("Pending() error = %v", err)
	}
	if got.UnackedCount != 2 || len(got.Notifications) != 2 {
		t.Fatalf("Pending() = %+v, want 2 notifications/count", got)
	}
}

func TestServicePendingSuppressedWhenDisabled(t *testing.T) {
	repo := &fakeRepo{unacked: []Notification{{ID: "n1", Kind: KindResultReady}}}
	prefs := fakePrefs{prefs: settings.Preferences{NotificationsEnabled: false, MorningReviewTime: "09:00", EveningReviewTime: "21:00"}}
	got, err := NewService(repo, prefs).Pending(context.Background())
	if err != nil {
		t.Fatalf("Pending() error = %v", err)
	}
	if got.UnackedCount != 0 || len(got.Notifications) != 0 {
		t.Fatalf("Pending() = %+v, want empty when notifications disabled", got)
	}
}

func TestServiceAckFound(t *testing.T) {
	repo := &fakeRepo{ackFound: true}
	if err := NewService(repo, enabled()).Ack(context.Background(), "n1"); err != nil {
		t.Fatalf("Ack() error = %v", err)
	}
	if repo.ackID != "n1" {
		t.Fatalf("Ack() id = %q, want n1", repo.ackID)
	}
}

func TestServiceAckNotFound(t *testing.T) {
	repo := &fakeRepo{ackFound: false}
	if err := NewService(repo, enabled()).Ack(context.Background(), "missing"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("Ack() error = %v, want ErrNotFound", err)
	}
}

func TestServiceAckCapture(t *testing.T) {
	repo := &fakeRepo{}
	if err := NewService(repo, enabled()).AckCapture(context.Background(), "cap-1"); err != nil {
		t.Fatalf("AckCapture() error = %v", err)
	}
	if repo.ackedCapt != "cap-1" {
		t.Fatalf("AckByCapture id = %q, want cap-1", repo.ackedCapt)
	}
}
