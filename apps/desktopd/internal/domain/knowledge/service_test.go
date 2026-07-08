package knowledge

import (
	"context"
	"errors"
	"testing"
	"time"
)

type fakeRepo struct {
	unknownID string
	knownID   string
	at        time.Time
	result    MarkResult
	err       error
}

func (f *fakeRepo) MarkUnknown(_ context.Context, id string, at time.Time) (MarkResult, error) {
	f.unknownID = id
	f.at = at
	return f.result, f.err
}

func (f *fakeRepo) MarkKnown(_ context.Context, id string, at time.Time) (MarkResult, error) {
	f.knownID = id
	f.at = at
	return f.result, f.err
}

func TestServiceMarkUnknownDelegates(t *testing.T) {
	repo := &fakeRepo{result: MarkResult{KnowledgeItemID: "k1", Status: StatusActive, WrongCount: 3}}
	svc := NewService(repo)
	fixed := time.Date(2026, 7, 8, 0, 0, 0, 0, time.UTC)
	svc.now = func() time.Time { return fixed }

	result, err := svc.MarkUnknown(context.Background(), "k1")
	if err != nil {
		t.Fatalf("MarkUnknown() error = %v", err)
	}
	if repo.unknownID != "k1" || !repo.at.Equal(fixed) {
		t.Fatalf("repo received id=%q at=%v", repo.unknownID, repo.at)
	}
	if result.WrongCount != 3 || result.Status != StatusActive {
		t.Fatalf("result = %#v", result)
	}
}

func TestServiceMarkKnownDelegates(t *testing.T) {
	repo := &fakeRepo{result: MarkResult{KnowledgeItemID: "k1", Status: StatusKnown}}
	svc := NewService(repo)

	result, err := svc.MarkKnown(context.Background(), "k1")
	if err != nil {
		t.Fatalf("MarkKnown() error = %v", err)
	}
	if repo.knownID != "k1" || result.Status != StatusKnown {
		t.Fatalf("repo id=%q result=%#v", repo.knownID, result)
	}
}

func TestServiceRejectsEmptyID(t *testing.T) {
	svc := NewService(&fakeRepo{})
	if _, err := svc.MarkUnknown(context.Background(), ""); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("MarkUnknown(\"\") error = %v, want ErrInvalidInput", err)
	}
	if _, err := svc.MarkKnown(context.Background(), ""); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("MarkKnown(\"\") error = %v, want ErrInvalidInput", err)
	}
}
