// Package suggest infers English candidate words from a Korean phonetic spelling
// (PRD §5.2-3, backlog #21): a developer who only heard a term types it in Hangul
// ("스테일") and gets English candidates ("stale"). Reverse transliteration is
// inherently ambiguous, so it returns several ranked candidates for the user to pick;
// the pick then enters the normal capture→explain pipeline. Phase 1 is AI-backed;
// a local pick cache is a later phase.
package suggest

import (
	"context"
	"errors"
)

var (
	ErrInvalidInput = errors.New("invalid suggest input")
	// ErrUnavailable means candidate inference is not configured (no AI provider).
	ErrUnavailable = errors.New("suggest is unavailable")
)

// MaxQueryLen bounds the phonetic input; suggestions are short words/terms.
const MaxQueryLen = 100

// Candidate is one inferred English word for a Korean phonetic query.
type Candidate struct {
	English    string  `json:"english"`
	Confidence float64 `json:"confidence"`
	GlossKo    string  `json:"gloss_ko"`
}

// Suggester infers candidates from a phonetic query (implemented by an AI provider).
type Suggester interface {
	Suggest(ctx context.Context, query string) ([]Candidate, error)
}
