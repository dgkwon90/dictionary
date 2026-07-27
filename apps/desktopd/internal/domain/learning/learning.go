// Package learning is the learning list: the words and sentences the user actually
// committed to studying, and what has happened to each one since.
//
// It is the destination of the whole redesign. A search is something you looked up;
// a learning item is something you decided to keep. The two were conflated before —
// every lookup silently created a learner row — which made the list a log of
// everything the user had ever typed instead of a list of what they meant to learn.
// Membership is granted only by an explicit action (a word's "학습할래요", a
// sentence's selection-complete), and this package is the view and the exit door for
// what is already in.
package learning

import (
	"context"
	"errors"
	"time"
)

var (
	ErrInvalidInput = errors.New("invalid learning input")
	ErrItemNotFound = errors.New("learning item not found")
)

// Learner status values persisted in learner_items.status. They live here rather
// than with knowledge extraction because this package owns every transition between
// them.
const (
	StatusActive = "active" // being learned; eligible for review scheduling
	StatusKnown  = "known"  // "알겠어요" — graduated, no longer scheduled
	// StatusRemoved is a soft delete: the user took the item out of the list. The row
	// is kept rather than deleted so the removal can be replicated to other devices
	// later — a row that simply vanishes is indistinguishable from one that never
	// synced (D8).
	StatusRemoved = "removed"
)

// Scopes the list can be narrowed to.
const (
	// ScopeAll is everything currently being learned.
	ScopeAll = "all"
	// ScopeToday and ScopeWeek filter by when the item was registered, which is what
	// "오늘 등록한 단어를 복습하자" is asking for — not when it was last reviewed.
	ScopeToday = "today"
	ScopeWeek  = "week"
	// ScopeWeak is the "자주 틀림" tab: only items with evidence of difficulty, worst
	// first. Items the user has never missed are excluded rather than ranked last, so
	// the tab means what its name says; an empty result is a real answer.
	ScopeWeak = "weak"
)

const (
	DefaultLimit = 50
	MaxLimit     = 200
)

// Item is one row of the learning list.
type Item struct {
	KnowledgeItemID string
	SurfaceText     string
	LearnKind       string
	MeaningKo       string
	PronunciationKo string
	DescriptionKo   string
	Status          string
	AskCount        int
	UnknownCount    int
	AttemptCount    int
	CorrectCount    int
	// Accuracy and WeaknessScore are derived by the service from the counts above,
	// never stored: a persisted score and the counters it came from drift apart the
	// moment one of them is written without the other (D5).
	Accuracy      float64
	WeaknessScore float64
	RegisteredAt  time.Time
	// LastGradedAt is zero when the item has never been reviewed or practiced.
	LastGradedAt time.Time
	// NextDueAt is the soonest due card for this item, zero when it has no cards.
	NextDueAt time.Time
	CardCount int
}

// Attempted reports whether the item has ever been graded, which is what tells
// "never tried" apart from "always wrong" — both show 0% accuracy.
func (i Item) Attempted() bool {
	return i.AttemptCount > 0
}

// ListInput is a resolved query: the service has already validated the scope and
// turned the dated scopes into explicit instants.
type ListInput struct {
	Scope     string
	LearnKind string
	// Query filters on the item text and its Korean meaning, case-insensitively.
	Query string
	Limit int
	// Since is the lower bound on registered_at for the dated scopes, and zero for
	// the others. It is an absolute instant computed by the service rather than a SQL
	// date() expression so that both sides of the comparison are produced the same
	// way ([[modernc-time-utc]]).
	Since time.Time
}

type Repository interface {
	// List returns the learning items matching input, already ordered and limited.
	List(ctx context.Context, input ListInput) ([]Item, error)
	// SetStatus moves one item to a new status and returns it as it now stands. It
	// returns ErrItemNotFound when the knowledge item is not in the learning list —
	// which is different from it not existing at all, and is the honest answer to
	// "retire this": there is nothing to retire.
	SetStatus(ctx context.Context, knowledgeItemID, status string, at time.Time) (Item, error)
}

// ValidScope reports whether scope is one the list understands.
func ValidScope(scope string) bool {
	switch scope {
	case ScopeAll, ScopeToday, ScopeWeek, ScopeWeak:
		return true
	}
	return false
}
