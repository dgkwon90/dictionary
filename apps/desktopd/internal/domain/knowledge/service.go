package knowledge

import (
	"context"
	"fmt"
)

type Service struct {
	repo Repository
}

func NewService(repo Repository) *Service {
	return &Service{repo: repo}
}

// ListByCapture returns the knowledge items extracted from a capture. Returns
// ErrCaptureNotFound for an unknown capture id.
func (s *Service) ListByCapture(ctx context.Context, captureID string) ([]CaptureItem, error) {
	if captureID == "" {
		return nil, fmt.Errorf("%w: capture id is required", ErrInvalidInput)
	}
	return s.repo.ListByCapture(ctx, captureID)
}
