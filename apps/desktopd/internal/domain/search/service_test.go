package search

import (
	"context"
	"errors"
	"testing"
	"time"

	"neulsang/desktopd/internal/domain/capture"
)

// stubRepo records what the service asked for. Only the calls SetLearnKind makes are
// meaningful here; the rest exist to satisfy Repository.
type stubRepo struct {
	triage    Triage
	loadErr   error
	kindCalls []setKindCall
}

type setKindCall struct {
	captureID   string
	learnKind   string
	triageState string
}

func (s *stubRepo) List(context.Context, ListInput) ([]Item, error) { return nil, nil }
func (s *stubRepo) Get(context.Context, string) (Detail, error)     { return Detail{}, nil }
func (s *stubRepo) SetSelected(context.Context, string, string, bool, time.Time) error {
	return nil
}

func (s *stubRepo) CompleteSelection(context.Context, CompleteInput, time.Time) (TriageResult, error) {
	return TriageResult{}, nil
}

func (s *stubRepo) LoadTriage(_ context.Context, captureID string) (Triage, error) {
	if s.loadErr != nil {
		return Triage{}, s.loadErr
	}
	triage := s.triage
	triage.CaptureID = captureID
	return triage, nil
}

func (s *stubRepo) SetTriageState(context.Context, string, string, time.Time) error { return nil }

func (s *stubRepo) SetLearnKind(_ context.Context, captureID, learnKind, triageState string, _ time.Time) error {
	s.kindCalls = append(s.kindCalls, setKindCall{captureID, learnKind, triageState})
	return nil
}

func (s *stubRepo) RegisterWordForLearning(context.Context, string, time.Time) (TriageResult, error) {
	return TriageResult{}, nil
}

func TestSetLearnKindCorrectsTheClassification(t *testing.T) {
	repo := &stubRepo{triage: Triage{LearnKind: capture.LearnKindWord, TriageState: capture.TriageUnseen}}
	svc := NewService(repo)

	result, err := svc.SetLearnKind(context.Background(), "cap-1", capture.LearnKindSentence)
	if err != nil {
		t.Fatalf("SetLearnKind() error = %v", err)
	}
	if len(repo.kindCalls) != 1 || repo.kindCalls[0].learnKind != capture.LearnKindSentence {
		t.Fatalf("repo calls = %#v", repo.kindCalls)
	}
	if result.TriageState != capture.TriageUnseen {
		t.Fatalf("state = %q, want the state to be left alone", result.TriageState)
	}
}

// needs_selection describes a sentence waiting for its words to be picked. Calling that
// capture a word leaves a state with no meaning — and one the schema refuses to store.
func TestSetLearnKindToWordClearsNeedsSelection(t *testing.T) {
	repo := &stubRepo{
		triage: Triage{LearnKind: capture.LearnKindSentence, TriageState: capture.TriageNeedsSelection},
	}
	svc := NewService(repo)

	result, err := svc.SetLearnKind(context.Background(), "cap-1", capture.LearnKindWord)
	if err != nil {
		t.Fatalf("SetLearnKind() error = %v", err)
	}
	if result.TriageState != capture.TriageUnseen {
		t.Fatalf("state = %q, want unseen", result.TriageState)
	}
	if len(repo.kindCalls) != 1 || repo.kindCalls[0].triageState != capture.TriageUnseen {
		t.Fatalf("repo calls = %#v", repo.kindCalls)
	}
}

func TestSetLearnKindSameKindIsANoOp(t *testing.T) {
	repo := &stubRepo{triage: Triage{LearnKind: capture.LearnKindWord, TriageState: capture.TriageUnseen}}
	svc := NewService(repo)

	if _, err := svc.SetLearnKind(context.Background(), "cap-1", capture.LearnKindWord); err != nil {
		t.Fatalf("SetLearnKind() error = %v", err)
	}
	if len(repo.kindCalls) != 0 {
		t.Fatalf("repo calls = %#v, want no write", repo.kindCalls)
	}
}

// Once a capture is learning or discarded its items and cards already exist and were
// built for the old kind; re-labelling it would leave rows describing something untrue.
func TestSetLearnKindRefusesAfterTheDecision(t *testing.T) {
	for _, state := range []string{capture.TriageLearning, capture.TriageDiscarded} {
		t.Run(state, func(t *testing.T) {
			repo := &stubRepo{triage: Triage{LearnKind: capture.LearnKindWord, TriageState: state}}
			svc := NewService(repo)

			_, err := svc.SetLearnKind(context.Background(), "cap-1", capture.LearnKindSentence)
			if !errors.Is(err, ErrInvalidInput) {
				t.Fatalf("error = %v, want ErrInvalidInput", err)
			}
			if len(repo.kindCalls) != 0 {
				t.Fatalf("repo calls = %#v, want no write", repo.kindCalls)
			}
		})
	}
}

func TestSetLearnKindRejectsBadInput(t *testing.T) {
	repo := &stubRepo{triage: Triage{LearnKind: capture.LearnKindWord, TriageState: capture.TriageUnseen}}
	svc := NewService(repo)

	if _, err := svc.SetLearnKind(context.Background(), "", capture.LearnKindWord); !errors.Is(err, ErrInvalidInput) {
		t.Errorf("empty capture id: error = %v, want ErrInvalidInput", err)
	}
	if _, err := svc.SetLearnKind(context.Background(), "cap-1", "paragraph"); !errors.Is(err, ErrInvalidInput) {
		t.Errorf("bad kind: error = %v, want ErrInvalidInput", err)
	}
	// Before the lookup finishes there is no classification to correct.
	unexplained := &stubRepo{triage: Triage{LearnKind: "", TriageState: capture.TriageUnseen}}
	if _, err := NewService(unexplained).SetLearnKind(context.Background(), "cap-1", capture.LearnKindWord); !errors.Is(err, ErrInvalidInput) {
		t.Errorf("unexplained capture: error = %v, want ErrInvalidInput", err)
	}
	if len(repo.kindCalls) != 0 {
		t.Fatalf("repo calls = %#v, want no write", repo.kindCalls)
	}
}
