package review

import (
	"context"
	"fmt"
	"strings"
	"time"
)

type Service struct {
	repo      Repository
	intervals IntervalSource
	now       func() time.Time
}

// NewService builds the review use case. A nil intervals source means the default
// schedule, which is what the app runs on until the user saves their own.
func NewService(repo Repository, intervals IntervalSource) *Service {
	if intervals == nil {
		intervals = FixedIntervals(DefaultIntervals())
	}
	return &Service{repo: repo, intervals: intervals, now: time.Now}
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
	if err := validateGrade(input); err != nil {
		return GradeResult{}, err
	}
	intervals, err := s.intervals.ReviewIntervals(ctx)
	if err != nil {
		return GradeResult{}, fmt.Errorf("load review intervals: %w", err)
	}
	return s.repo.Grade(ctx, input.CardID, input.Rating, input.ElapsedMs, s.now().UTC(), intervals)
}

// GradePractice records a practice attempt. It takes the same input as Grade and
// applies the same rules to it — a practice answer is an answer — but leaves the
// card's schedule where it was.
func (s *Service) GradePractice(ctx context.Context, input GradeInput) (PracticeResult, error) {
	if err := validateGrade(input); err != nil {
		return PracticeResult{}, err
	}
	return s.repo.GradePractice(ctx, input.CardID, input.Rating, input.ElapsedMs, s.now().UTC())
}

func validateGrade(input GradeInput) error {
	if input.CardID == "" {
		return fmt.Errorf("%w: card id is required", ErrInvalidInput)
	}
	if !ValidRating(input.Rating) {
		return fmt.Errorf("%w: rating must be again/hard/good/easy", ErrInvalidInput)
	}
	if input.ElapsedMs < 0 {
		return fmt.Errorf("%w: elapsed_ms must be non-negative", ErrInvalidInput)
	}
	return nil
}
