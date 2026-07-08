package suggest

import (
	"context"
	"fmt"
	"strings"
)

type Service struct {
	suggester Suggester
}

func NewService(suggester Suggester) *Service {
	return &Service{suggester: suggester}
}

// Suggest validates the query and returns candidate English words. The suggester
// (AI) does the inference and ranks by confidence; this layer only guards input.
func (s *Service) Suggest(ctx context.Context, query string) ([]Candidate, error) {
	trimmed := strings.TrimSpace(query)
	if trimmed == "" {
		return nil, fmt.Errorf("%w: query is required", ErrInvalidInput)
	}
	if len([]rune(trimmed)) > MaxQueryLen {
		return nil, fmt.Errorf("%w: query too long", ErrInvalidInput)
	}
	if s.suggester == nil {
		return nil, ErrUnavailable
	}
	return s.suggester.Suggest(ctx, trimmed)
}
