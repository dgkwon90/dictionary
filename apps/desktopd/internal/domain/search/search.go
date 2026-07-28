// Package search is the search history: the list of things the user looked up and
// what they decided about each one.
//
// It replaces the old inbox domain. That model sorted captures into new/saved/
// archived — an "organize your stuff" axis borrowed from read-later apps, which the
// user found meaningless in practice ("저장/보관의 개념이 애매하다"). The axis that
// actually matters is whether a search has been *resolved*: did the user look at the
// result and decide something about it. Saving and archiving are gone entirely.
package search

import (
	"context"
	"errors"
	"time"
)

var (
	ErrInvalidInput    = errors.New("invalid search input")
	ErrCaptureNotFound = errors.New("capture not found")
)

// Views of the history. The default is deliberately the unresolved one — the point
// of the list is to finish what you started, not to browse everything you ever typed.
const (
	ViewUnresolved = "unresolved"
	ViewAll        = "all"
)

const (
	DefaultLimit = 50
	MaxLimit     = 200
)

// Item is one row of the history list.
type Item struct {
	CaptureID    string
	SelectedText string
	SourceApp    string
	SourceType   string
	InputMode    string
	// LearnKind is empty until the AI result lands and the server classifies it.
	LearnKind   string
	TriageState string
	// JobStatus is the AI lookup's own state (queued/running/done/failed), derived at
	// read time rather than stored on the capture — it answers a different question
	// than TriageState and the two must not be conflated.
	JobStatus string
	BriefKo   string
	CreatedAt time.Time
}

type ListInput struct {
	View      string
	LearnKind string
	Limit     int
}

// Triage is the capture state the transition rules need, loaded before applying one.
type Triage struct {
	CaptureID   string
	LearnKind   string
	TriageState string
}

// TriageResult reports where a capture ended up.
type TriageResult struct {
	CaptureID   string
	TriageState string
	// LearningItemIDs and CardsCreated are filled when a transition registered
	// something for learning.
	LearningItemIDs []string
	CardsCreated    int
}

// Detail is one search opened up: the result plus the terms found in it and which
// of them the user has picked as unknown.
type Detail struct {
	Item
	DetailedKo string
	Items      []DetailItem
}

// DetailItem is a term the AI found inside the capture.
type DetailItem struct {
	KnowledgeItemID string
	SurfaceText     string
	MeaningKo       string
	DescriptionKo   string
	PronunciationKo string
	// CharStart/CharEnd locate the term inside the capture text, in runes, so the UI
	// can highlight it. Both are -1 when the term was not found verbatim (the AI
	// returns dictionary forms, which do not always appear literally).
	CharStart int
	CharEnd   int
	Selected  bool
}

// CompleteInput finishes a sentence's word selection.
type CompleteInput struct {
	CaptureID string
	// NoUnknownWords records that the user could not point at any single word — the
	// sentence itself was the problem (structure, idiom, nuance). Without it the
	// sentence could never leave the unresolved list, so the user would be pushed
	// into picking a word at random just to clear it.
	NoUnknownWords bool
}

type Repository interface {
	List(ctx context.Context, input ListInput) ([]Item, error)
	// Get returns one search with its extracted terms and selection state.
	Get(ctx context.Context, captureID string) (Detail, error)
	// SetSelected marks or unmarks one extracted term as a word the user did not know.
	SetSelected(ctx context.Context, captureID, knowledgeItemID string, selected bool, at time.Time) error
	// CompleteSelection registers the sentence and every selected word for learning.
	CompleteSelection(ctx context.Context, input CompleteInput, at time.Time) (TriageResult, error)
	// LoadTriage returns the capture's current classification and state.
	LoadTriage(ctx context.Context, captureID string) (Triage, error)
	// SetTriageState moves a capture to a new state without registering anything.
	SetTriageState(ctx context.Context, captureID, state string, at time.Time) error
	// SetLearnKind rewrites the word/sentence classification, and the triage state
	// along with it when the old state only made sense for the old kind.
	SetLearnKind(ctx context.Context, captureID, learnKind, triageState string, at time.Time) error
	// RegisterWordForLearning commits a word capture to the learning list: it creates
	// the learner row, turns that word's candidates into cards, and sets the capture
	// to learning — all in one transaction, so a half-registered word cannot exist.
	RegisterWordForLearning(ctx context.Context, captureID string, at time.Time) (TriageResult, error)
}

func ValidView(value string) bool {
	return value == ViewUnresolved || value == ViewAll
}
