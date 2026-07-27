package knowledge

import (
	"context"
	"errors"
	"testing"
)

type fakeRepo struct {
	listCaptureID string
	items         []CaptureItem
	err           error
}

func (f *fakeRepo) ListByCapture(_ context.Context, captureID string) ([]CaptureItem, error) {
	f.listCaptureID = captureID
	return f.items, f.err
}

func TestServiceRejectsEmptyID(t *testing.T) {
	svc := NewService(&fakeRepo{})
	if _, err := svc.ListByCapture(context.Background(), ""); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("ListByCapture(\"\") error = %v, want ErrInvalidInput", err)
	}
}

func TestServiceListByCapturePassesThrough(t *testing.T) {
	repo := &fakeRepo{items: []CaptureItem{{KnowledgeItemID: "k1", SurfaceText: "stale"}}}
	svc := NewService(repo)
	items, err := svc.ListByCapture(context.Background(), "cap-1")
	if err != nil {
		t.Fatalf("ListByCapture() error = %v", err)
	}
	if repo.listCaptureID != "cap-1" || len(items) != 1 || items[0].KnowledgeItemID != "k1" {
		t.Fatalf("captureID=%q items=%#v", repo.listCaptureID, items)
	}
}
