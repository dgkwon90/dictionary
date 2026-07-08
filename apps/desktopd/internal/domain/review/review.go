// Package review owns spaced-repetition cards (PRD §14.3): generating cards from
// AI candidates when a word is marked unknown, and listing cards that are due.
// Grading and FSRS scheduling arrive in a later issue (#10).
package review

import (
	"context"
	"errors"
	"time"
)

var ErrInvalidInput = errors.New("invalid review input")

// Card states persisted in review_cards.state. A freshly generated card is New and
// immediately due; #10 introduces the learning/review/relearning transitions.
const (
	CardStateNew = "new"

	// DefaultCardType is used when an AI candidate omits card_type.
	DefaultCardType = "meaning"
)

const (
	DefaultDueLimit = 50
	MaxDueLimit     = 200
)

// Card is a due review card as surfaced to the client. The answer is intentionally
// omitted here; it is revealed during grading (#10).
type Card struct {
	CardID          string
	KnowledgeItemID string
	CardType        string
	Question        string
	State           string
	DueAt           time.Time
}

type Repository interface {
	// DueCards returns cards whose due_at is at or before now, soonest first.
	DueCards(ctx context.Context, now time.Time, limit int) ([]Card, error)
}
