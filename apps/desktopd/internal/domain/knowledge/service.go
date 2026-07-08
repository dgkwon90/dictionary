package knowledge

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

// MarkUnknown flags a knowledge item the user does not know: it bumps
// learner_items.wrong_count and last_wrong_at and keeps the item active so #9 can
// turn its candidates into review cards (§5.2-6 "모름 표시").
func (s *Service) MarkUnknown(ctx context.Context, knowledgeItemID string) (MarkResult, error) {
	if knowledgeItemID == "" {
		return MarkResult{}, fmt.Errorf("%w: knowledge item id is required", ErrInvalidInput)
	}
	return s.repo.MarkUnknown(ctx, knowledgeItemID, s.now().UTC())
}

// MarkKnown is the opposite action (§5.2-6 "알고 있음 표시"): it marks the item
// known so it is excluded from review scheduling.
func (s *Service) MarkKnown(ctx context.Context, knowledgeItemID string) (MarkResult, error) {
	if knowledgeItemID == "" {
		return MarkResult{}, fmt.Errorf("%w: knowledge item id is required", ErrInvalidInput)
	}
	return s.repo.MarkKnown(ctx, knowledgeItemID, s.now().UTC())
}
