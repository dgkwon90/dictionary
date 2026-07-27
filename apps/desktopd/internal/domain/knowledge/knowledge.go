// Package knowledge exposes the words and sentences extracted from a capture, joined
// with what the learner has done about each one.
//
// It used to own the learner state as well — "모름"/"알아요" wrote learner_items from
// here. Both doors are gone: an item now enters the learning list through triage
// (a word's "학습할래요", a sentence's selection-complete) and leaves it through the
// learning domain. What is left is the read side: given a capture, what did the AI
// find in it.
package knowledge

import (
	"context"
	"errors"
)

var (
	ErrInvalidInput    = errors.New("invalid knowledge input")
	ErrCaptureNotFound = errors.New("capture not found")
)

// CaptureItem is one knowledge item extracted from a capture, with the learner's
// current state.
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
	UnknownCount    int
}

type Repository interface {
	// ListByCapture returns the capture's linked knowledge items (learner state
	// joined). It returns ErrCaptureNotFound if the capture itself does not exist,
	// distinguishing that from a capture with no extracted items yet.
	ListByCapture(ctx context.Context, captureID string) ([]CaptureItem, error)
}
