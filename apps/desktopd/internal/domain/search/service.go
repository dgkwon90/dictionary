package search

import (
	"context"
	"errors"
	"fmt"
	"time"

	"neulsang/desktopd/internal/domain/capture"
	"neulsang/desktopd/internal/id"
)

type Service struct {
	repo  Repository
	now   func() time.Time
	newID func() string
}

func NewService(repo Repository) *Service {
	return &Service{repo: repo, now: time.Now, newID: id.New}
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

// SetLearnKind corrects the server's word/sentence call (D1).
//
// The classification is automatic because the product's whole promise is "단축키로 즉시"
// — asking before every search would spend the user's attention on a question that is
// nearly always about whitespace. The cost of being wrong is not symmetric, though: a
// sentence filed as a word skips word selection entirely, so the user loses the ability
// to pick what they did not understand. This is that escape hatch.
//
// It is only open before the decision is final. Once a capture is learning or
// discarded, its items and cards already exist and were built for the old kind;
// re-labelling it then would leave rows describing something that is no longer true.
func (s *Service) SetLearnKind(ctx context.Context, captureID, learnKind string) (TriageResult, error) {
	if captureID == "" {
		return TriageResult{}, fmt.Errorf("%w: capture id is required", ErrInvalidInput)
	}
	if !capture.ValidLearnKind(learnKind) {
		return TriageResult{}, fmt.Errorf("%w: unsupported kind %q", ErrInvalidInput, learnKind)
	}
	current, err := s.repo.LoadTriage(ctx, captureID)
	if err != nil {
		return TriageResult{}, err
	}
	if current.LearnKind == "" {
		return TriageResult{}, fmt.Errorf("%w: this search has not been explained yet", ErrInvalidInput)
	}
	if current.TriageState == capture.TriageLearning || current.TriageState == capture.TriageDiscarded {
		return TriageResult{}, fmt.Errorf("%w: this search has already been decided", ErrInvalidInput)
	}
	if current.LearnKind == learnKind {
		return TriageResult{CaptureID: captureID, TriageState: current.TriageState}, nil
	}

	// needs_selection only means anything for a sentence — a word has nothing to pick
	// from — so calling it a word puts it back to undecided.
	state := current.TriageState
	if learnKind == capture.LearnKindWord && state == capture.TriageNeedsSelection {
		state = capture.TriageUnseen
	}
	if err := s.repo.SetLearnKind(ctx, captureID, learnKind, state, s.now().UTC()); err != nil {
		return TriageResult{}, err
	}
	return TriageResult{CaptureID: captureID, TriageState: state}, nil
}

// Get returns one search opened up, for the detail panel.
func (s *Service) Get(ctx context.Context, captureID string) (Detail, error) {
	if captureID == "" {
		return Detail{}, fmt.Errorf("%w: capture id is required", ErrInvalidInput)
	}
	return s.repo.Get(ctx, captureID)
}

// Retry queues the lookup again for a search whose AI call failed.
//
// A failed lookup leaves a capture with no classification, which means the result
// screen has nothing to offer: no 학습할래요, no 단어 고르기, not even a way to throw it
// away. Retyping the same text into the popup does work, but it makes a second capture
// and leaves the failed one sitting in the history forever. This runs the same text
// against the same capture.
//
// Only a failed lookup is retryable. A successful explanation may already have
// produced knowledge items the user acted on, and re-running would overwrite it.
func (s *Service) Retry(ctx context.Context, captureID string) (RetryResult, error) {
	if captureID == "" {
		return RetryResult{}, fmt.Errorf("%w: capture id is required", ErrInvalidInput)
	}
	jobID := s.newID()
	text, err := s.repo.CreateRetryJob(ctx, captureID, jobID, s.now().UTC())
	if err != nil {
		return RetryResult{}, err
	}
	return RetryResult{CaptureID: captureID, LookupJobID: jobID, Text: text}, nil
}

// Select marks one of the capture's extracted terms as a word the user did not know.
// It is only meaningful for a sentence: a word capture has nothing to pick from.
func (s *Service) Select(ctx context.Context, captureID, knowledgeItemID string, selected bool) error {
	if captureID == "" || knowledgeItemID == "" {
		return fmt.Errorf("%w: capture id and knowledge item id are required", ErrInvalidInput)
	}
	current, err := s.repo.LoadTriage(ctx, captureID)
	if err != nil {
		return err
	}
	if current.LearnKind != capture.LearnKindSentence {
		return fmt.Errorf("%w: only a sentence has words to pick", ErrInvalidInput)
	}
	if current.TriageState == capture.TriageDiscarded {
		return fmt.Errorf("%w: this search was thrown away", ErrInvalidInput)
	}
	return s.repo.SetSelected(ctx, captureID, knowledgeItemID, selected, s.now().UTC())
}

// CompleteSelection is the sentence's counterpart to a word's "학습할래요": it turns the
// sentence and the words picked out of it into learning items, together.
func (s *Service) CompleteSelection(ctx context.Context, input CompleteInput) (TriageResult, error) {
	if input.CaptureID == "" {
		return TriageResult{}, fmt.Errorf("%w: capture id is required", ErrInvalidInput)
	}
	current, err := s.repo.LoadTriage(ctx, input.CaptureID)
	if err != nil {
		return TriageResult{}, err
	}
	if current.LearnKind != capture.LearnKindSentence {
		return TriageResult{}, fmt.Errorf("%w: only a sentence goes through word selection", ErrInvalidInput)
	}
	// Ask the state machine whether this transition is legal at all before doing the
	// work, so a discarded or already-finished capture cannot be re-registered.
	if _, err := capture.NextTriageState(current.TriageState, current.LearnKind, capture.TransitionLearn); err != nil {
		// A sentence still in "unseen" has not been opened; completing selection is
		// what moves it forward, so treat that as fine and let the repository run.
		if !errors.Is(err, capture.ErrSelectionRequired) {
			return TriageResult{}, err
		}
	}
	return s.repo.CompleteSelection(ctx, input, s.now().UTC())
}
