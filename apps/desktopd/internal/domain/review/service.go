package review

import (
	"context"
	"fmt"
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
