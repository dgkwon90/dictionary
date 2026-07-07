package inbox

import (
	"context"
	"fmt"
)

const (
	DefaultLimit = 50
	MaxLimit     = 200
)

type Service struct {
	repo Repository
}

func NewService(repo Repository) *Service {
	return &Service{repo: repo}
}

func (s *Service) List(ctx context.Context, input ListInput) ([]Item, error) {
	if input.Status != "" && !validListStatus(input.Status) {
		return nil, fmt.Errorf("%w: unsupported status %q", ErrInvalidInput, input.Status)
	}
	if input.Limit < 0 {
		return nil, fmt.Errorf("%w: limit must be non-negative", ErrInvalidInput)
	}

	limit := input.Limit
	if limit == 0 {
		limit = DefaultLimit
	}
	if limit > MaxLimit {
		limit = MaxLimit
	}

	return s.repo.List(ctx, input.Status, limit)
}

func (s *Service) SetStatus(ctx context.Context, captureID, status string) error {
	if status != "saved" && status != "archived" {
		return fmt.Errorf("%w: status must be saved or archived", ErrInvalidInput)
	}
	return s.repo.SetStatus(ctx, captureID, status)
}

func validListStatus(status string) bool {
	switch status {
	case "new", "saved", "review_added", "archived", "failed":
		return true
	default:
		return false
	}
}
