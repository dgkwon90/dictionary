package inbox

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"
)

type fakeRepository struct {
	list      func(context.Context, string, int) ([]Item, error)
	setStatus func(context.Context, string, string) error
}

func (f fakeRepository) List(ctx context.Context, status string, limit int) ([]Item, error) {
	return f.list(ctx, status, limit)
}

func (f fakeRepository) SetStatus(ctx context.Context, captureID, status string) error {
	return f.setStatus(ctx, captureID, status)
}

func TestServiceList(t *testing.T) {
	createdAt := time.Date(2026, 7, 7, 1, 2, 3, 0, time.UTC)
	wantItems := []Item{{
		CaptureID:    "capture-1",
		SelectedText: "hello",
		InputMode:    "manual",
		CreatedAt:    createdAt,
		JobStatus:    "done",
		Status:       "new",
	}}
	var gotStatus string
	var gotLimit int
	svc := NewService(fakeRepository{list: func(_ context.Context, status string, limit int) ([]Item, error) {
		gotStatus = status
		gotLimit = limit
		return wantItems, nil
	}})

	gotItems, err := svc.List(context.Background(), ListInput{Status: "new", Limit: 25})
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if gotStatus != "new" || gotLimit != 25 {
		t.Fatalf("repo List status=%q limit=%d", gotStatus, gotLimit)
	}
	if !reflect.DeepEqual(gotItems, wantItems) {
		t.Fatalf("List() = %#v, want %#v", gotItems, wantItems)
	}
}

func TestServiceListDefaultLimit(t *testing.T) {
	var gotLimit int
	svc := NewService(fakeRepository{list: func(_ context.Context, _ string, limit int) ([]Item, error) {
		gotLimit = limit
		return nil, nil
	}})

	if _, err := svc.List(context.Background(), ListInput{}); err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if gotLimit != DefaultLimit {
		t.Fatalf("limit = %d, want %d", gotLimit, DefaultLimit)
	}
}

func TestServiceListClampsMaxLimit(t *testing.T) {
	var gotLimit int
	svc := NewService(fakeRepository{list: func(_ context.Context, _ string, limit int) ([]Item, error) {
		gotLimit = limit
		return nil, nil
	}})

	if _, err := svc.List(context.Background(), ListInput{Limit: MaxLimit + 1}); err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if gotLimit != MaxLimit {
		t.Fatalf("limit = %d, want %d", gotLimit, MaxLimit)
	}
}

func TestServiceListInvalidInput(t *testing.T) {
	tests := []struct {
		name  string
		input ListInput
	}{
		{name: "bad status", input: ListInput{Status: "done", Limit: 10}},
		{name: "negative limit", input: ListInput{Limit: -1}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := NewService(fakeRepository{list: func(context.Context, string, int) ([]Item, error) {
				t.Fatal("List should not be called")
				return nil, nil
			}})

			_, err := svc.List(context.Background(), tt.input)
			if !errors.Is(err, ErrInvalidInput) {
				t.Fatalf("List() error = %v, want ErrInvalidInput", err)
			}
		})
	}
}

func TestServiceSetStatus(t *testing.T) {
	for _, status := range []string{"saved", "archived"} {
		t.Run(status, func(t *testing.T) {
			var gotCaptureID string
			var gotStatus string
			svc := NewService(fakeRepository{setStatus: func(_ context.Context, captureID, setStatus string) error {
				gotCaptureID = captureID
				gotStatus = setStatus
				return nil
			}})

			if err := svc.SetStatus(context.Background(), "capture-1", status); err != nil {
				t.Fatalf("SetStatus() error = %v", err)
			}
			if gotCaptureID != "capture-1" || gotStatus != status {
				t.Fatalf("repo SetStatus captureID=%q status=%q", gotCaptureID, gotStatus)
			}
		})
	}
}

func TestServiceSetStatusInvalidInput(t *testing.T) {
	for _, status := range []string{"", "new", "review_added", "failed"} {
		t.Run(status, func(t *testing.T) {
			svc := NewService(fakeRepository{setStatus: func(context.Context, string, string) error {
				t.Fatal("SetStatus should not be called")
				return nil
			}})

			err := svc.SetStatus(context.Background(), "capture-1", status)
			if !errors.Is(err, ErrInvalidInput) {
				t.Fatalf("SetStatus() error = %v, want ErrInvalidInput", err)
			}
		})
	}
}

func TestServiceSetStatusPropagatesNotFound(t *testing.T) {
	svc := NewService(fakeRepository{setStatus: func(context.Context, string, string) error {
		return ErrCaptureNotFound
	}})

	err := svc.SetStatus(context.Background(), "missing", "saved")
	if !errors.Is(err, ErrCaptureNotFound) {
		t.Fatalf("SetStatus() error = %v, want ErrCaptureNotFound", err)
	}
}
