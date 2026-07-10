// Package knowledge owns the learner-facing state of extracted words (PRD §14.3):
// marking an item as unknown (a review target) or known. Extraction/upsert itself
// lives with the explanation pipeline; this package only mutates learner_items.
package knowledge

import (
	"context"
	"errors"
	"time"
)

var (
	ErrInvalidInput          = errors.New("invalid knowledge input")
	ErrKnowledgeItemNotFound = errors.New("knowledge item not found")
	ErrCaptureNotFound       = errors.New("capture not found")
)

// Learner status values persisted in learner_items.status.
const (
	StatusActive = "active" // default; eligible for review scheduling
	StatusKnown  = "known"  // user marked as known; excluded from review
)

// MarkResult reports the learner state after a mark-unknown/mark-known call, plus
// how many stored review_card_candidates are anchored to the item (proof that #9
// has material to build cards from).
type MarkResult struct {
	KnowledgeItemID string
	Status          string
	AskCount        int
	WrongCount      int
	CandidateCount  int
	// CardsCreated is how many review_cards this call generated from the item's
	// candidates (mark-unknown only; PRD Task06). Zero for mark-known.
	CardsCreated int
}

// CaptureItem is one knowledge item extracted from a capture, with the learner's
// current state — enough for the Inbox UI (#15) to render each word and call
// mark-unknown/mark-known on it by ID.
type CaptureItem struct {
	KnowledgeItemID string
	SurfaceText     string
	ItemType        string
	PronunciationKo string
	MeaningKo       string
	Role            string
	Confidence      float64
	Status          string
	AskCount        int
	WrongCount      int
}

type Repository interface {
	MarkUnknown(ctx context.Context, knowledgeItemID string, at time.Time) (MarkResult, error)
	MarkKnown(ctx context.Context, knowledgeItemID string, at time.Time) (MarkResult, error)
	// ListByCapture returns the capture's linked knowledge items (learner state
	// joined). It returns ErrCaptureNotFound if the capture itself does not exist,
	// distinguishing that from a capture with no extracted items yet.
	ListByCapture(ctx context.Context, captureID string) ([]CaptureItem, error)
}
