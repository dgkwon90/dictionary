package search

import (
	"context"
	"fmt"
	"time"

	"neulsang/desktopd/internal/domain/capture"
)

type Service struct {
	repo Repository
	now  func() time.Time
}

func NewService(repo Repository) *Service {
	return &Service{repo: repo, now: time.Now}
}

// List returns the history. An empty view means the default (unresolved only).
func (s *Service) List(ctx context.Context, input ListInput) ([]Item, error) {
	if input.View == "" {
		input.View = ViewUnresolved
	}
	if !ValidView(input.View) {
		return nil, fmt.Errorf("%w: unsupported view %q", ErrInvalidInput, input.View)
	}
	if input.LearnKind != "" && !capture.ValidLearnKind(input.LearnKind) {
		return nil, fmt.Errorf("%w: unsupported kind %q", ErrInvalidInput, input.LearnKind)
	}
	if input.Limit < 0 {
		return nil, fmt.Errorf("%w: limit must not be negative", ErrInvalidInput)
	}
	if input.Limit == 0 {
		input.Limit = DefaultLimit
	}
	if input.Limit > MaxLimit {
		input.Limit = MaxLimit
	}
	return s.repo.List(ctx, input)
}

// Triage applies a user decision to a capture. The rules themselves live in
// capture.NextTriageState so they can be tested without a database; this method's job
// is to load the current state, ask for the next one, and persist the consequences.
func (s *Service) Triage(ctx context.Context, captureID string, transition capture.Transition) (TriageResult, error) {
	if captureID == "" {
		return TriageResult{}, fmt.Errorf("%w: capture id is required", ErrInvalidInput)
	}
	current, err := s.repo.LoadTriage(ctx, captureID)
	if err != nil {
		return TriageResult{}, err
	}

	// A sentence never becomes a learning item through this endpoint, even once its
	// words have been picked: registering it also has to create the sentence's own
	// knowledge item and the cloze cards for the chosen words. That is the
	// selection-complete path. Refusing here keeps a sentence from reaching
	// "learning" without those, which would leave it in the list with nothing to
	// review.
	if transition == capture.TransitionLearn &&
		current.LearnKind == capture.LearnKindSentence &&
		current.TriageState != capture.TriageLearning {
		return TriageResult{}, capture.ErrSelectionRequired
	}

	next, err := capture.NextTriageState(current.TriageState, current.LearnKind, transition)
	if err != nil {
		return TriageResult{}, err
	}

	// Committing a word to the learning list is more than a state change — it creates
	// the learner row and its cards — so it gets its own transactional path.
	if transition == capture.TransitionLearn && current.TriageState != capture.TriageLearning {
		return s.repo.RegisterWordForLearning(ctx, captureID, s.now().UTC())
	}

	if next == current.TriageState {
		return TriageResult{CaptureID: captureID, TriageState: next}, nil
	}
	if err := s.repo.SetTriageState(ctx, captureID, next, s.now().UTC()); err != nil {
		return TriageResult{}, err
	}
	return TriageResult{CaptureID: captureID, TriageState: next}, nil
}
