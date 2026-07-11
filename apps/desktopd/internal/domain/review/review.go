// Package review owns spaced-repetition cards (PRD §14.3): generating cards from
// AI candidates when a word is marked unknown, and listing cards that are due.
// Grading and FSRS scheduling arrive in a later issue (#10).
package review

import (
	"context"
	"errors"
	"time"
)

var (
	ErrInvalidInput = errors.New("invalid review input")
	ErrCardNotFound = errors.New("review card not found")
)

// Card states persisted in review_cards.state. A freshly generated card is New and
// immediately due; grading moves it to Learning (on Again) or Review.
const (
	CardStateNew      = "new"
	CardStateLearning = "learning"
	CardStateReview   = "review"

	// DefaultCardType is used when an AI candidate omits card_type.
	DefaultCardType = "meaning"
)

// Ratings a user can give a card (PRD §13.1).
const (
	RatingAgain = "again"
	RatingHard  = "hard"
	RatingGood  = "good"
	RatingEasy  = "easy"
)

const (
	DefaultDueLimit = 50
	MaxDueLimit     = 200
)

// GradeResult reports a card's state after grading.
type GradeResult struct {
	CardID       string
	Rating       string
	State        string
	Reps         int
	IntervalDays float64
	DueAt        time.Time
	// MasteryScore is the knowledge item's recomputed mastery after this grade
	// (PRD §13.2), clamped to [0,1].
	MasteryScore float64
}

// Card is a due review card as surfaced to the client. Answer/Explanation are the
// back of the flashcard: the UI hides them until the user asks to reveal, then grades
// (PRD §9.5). Neulsang is local-first single-user, so there is no privacy boundary
// that would justify a separate reveal round-trip (#16).
type Card struct {
	CardID          string
	KnowledgeItemID string
	CardType        string
	Question        string
	Answer          string
	Explanation     string
	State           string
	DueAt           time.Time
}

type Repository interface {
	// DueCards returns cards whose due_at is at or before now, soonest first.
	DueCards(ctx context.Context, now time.Time, limit int) ([]Card, error)
	// Grade applies a rating to a card: it reschedules the card (NextSchedule),
	// appends a review_logs row, and bumps the card/learner review counters, all
	// atomically. It returns ErrCardNotFound when the card does not exist.
	Grade(ctx context.Context, cardID, rating string, elapsedMs int, now time.Time) (GradeResult, error)
}
