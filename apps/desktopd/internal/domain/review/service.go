package review

import (
	"context"
	"fmt"
	"strings"
	"time"
)

type Service struct {
	repo Repository
	now  func() time.Time
}

func NewService(repo Repository) *Service {
	return &Service{repo: repo, now: time.Now}
}

type DueInput struct {
	Limit int
}

type PracticeInput struct {
	Query string
	Limit int
}

// Due lists review cards that are ready to study now (PRD Task06 "due card 조회").
func (s *Service) Due(ctx context.Context, input DueInput) ([]Card, error) {
	if input.Limit < 0 {
		return nil, fmt.Errorf("%w: limit must be non-negative", ErrInvalidInput)
	}
	limit := input.Limit
	if limit == 0 {
		limit = DefaultDueLimit
	}
	if limit > MaxDueLimit {
		limit = MaxDueLimit
	}
	return s.repo.DueCards(ctx, s.now().UTC(), limit)
}

// StartSession returns the cards to review in a session (PRD §15.5). For now this is
// simply the current due list; session bookkeeping is out of MVP scope.
func (s *Service) StartSession(ctx context.Context, input DueInput) ([]Card, error) {
	return s.Due(ctx, input)
}

// Practice lists review cards for read-only practice, ignoring due time.
func (s *Service) Practice(ctx context.Context, input PracticeInput) ([]Card, error) {
	if input.Limit < 0 {
		return nil, fmt.Errorf("%w: limit must be non-negative", ErrInvalidInput)
	}
	limit := input.Limit
	if limit == 0 {
		limit = DefaultDueLimit
	}
	if limit > MaxDueLimit {
		limit = MaxDueLimit
	}
	return s.repo.PracticeCards(ctx, strings.TrimSpace(input.Query), limit)
}

type GradeInput struct {
	CardID    string
	Rating    string
	ElapsedMs int
}

// Grade applies a rating to a card and reschedules it (PRD §15.6, §13.1).
func (s *Service) Grade(ctx context.Context, input GradeInput) (GradeResult, error) {
	if input.CardID == "" {
		return GradeResult{}, fmt.Errorf("%w: card id is required", ErrInvalidInput)
	}
	if !ValidRating(input.Rating) {
		return GradeResult{}, fmt.Errorf("%w: rating must be again/hard/good/easy", ErrInvalidInput)
	}
	if input.ElapsedMs < 0 {
		return GradeResult{}, fmt.Errorf("%w: elapsed_ms must be non-negative", ErrInvalidInput)
	}
	return s.repo.Grade(ctx, input.CardID, input.Rating, input.ElapsedMs, s.now().UTC())
}
