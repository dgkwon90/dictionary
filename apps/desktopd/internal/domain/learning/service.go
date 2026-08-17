package learning

import (
	"context"
	"fmt"
	"strings"
	"time"

	"neulsang/desktopd/internal/domain/capture"
	"neulsang/desktopd/internal/domain/review"
)

type Service struct {
	repo Repository
	now  func() time.Time
	loc  *time.Location
}

func NewService(repo Repository) *Service {
	return &Service{repo: repo, now: time.Now, loc: time.Local}
}

// List returns the learning list for one filter combination.
//
// "오늘"/"이번 주" are resolved here rather than in SQL. The boundary is local
// midnight, not UTC midnight: with UTC the day would roll over at 09:00 KST and
// "오늘 등록한 단어" would keep showing yesterday's until mid-morning (the same bug
// the dashboard already fixed in RW-07). The instant is converted to UTC before it
// reaches the query, because that is how the timestamps are stored.
func (s *Service) List(ctx context.Context, input ListInput) ([]Item, error) {
	if input.Scope == "" {
		input.Scope = ScopeAll
	}
	if !ValidScope(input.Scope) {
		return nil, fmt.Errorf("%w: unsupported scope %q", ErrInvalidInput, input.Scope)
	}
	if input.Membership == "" {
		input.Membership = MembershipActive
	}
	if !ValidMembership(input.Membership) {
		return nil, fmt.Errorf("%w: unsupported membership %q", ErrInvalidInput, input.Membership)
	}
	if input.LearnKind != "" && !capture.ValidLearnKind(input.LearnKind) {
		return nil, fmt.Errorf("%w: unsupported kind %q", ErrInvalidInput, input.LearnKind)
	}
	if input.Limit < 0 {
		return nil, fmt.Errorf("%w: limit must not be negative", ErrInvalidInput)
	}
	if input.Limit == 0 {
		input.Limit = DefaultLimit
	}
	if input.Limit > MaxLimit {
		input.Limit = MaxLimit
	}
	input.Query = strings.TrimSpace(input.Query)

	now := s.now()
	switch input.Scope {
	case ScopeToday:
		local := now.In(s.loc)
		input.Since = time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, s.loc).UTC()
	case ScopeWeek:
		// A rolling 7 days rather than a calendar week, matching the dashboard so the
		// two screens cannot disagree about what "이번 주" covers.
		input.Since = now.UTC().AddDate(0, 0, -7)
	}

	items, err := s.repo.List(ctx, input)
	if err != nil {
		return nil, err
	}
	for i := range items {
		items[i] = withScores(items[i])
	}
	return items, nil
}

// Retire is "알겠어요": the item is learned, stop scheduling it. The row stays so the
// history behind it — how often it was asked, how often it was missed — survives.
func (s *Service) Retire(ctx context.Context, knowledgeItemID string) (Item, error) {
	return s.setStatus(ctx, knowledgeItemID, StatusKnown)
}

// Remove takes an item out of the learning list without claiming it was learned. It
// is a soft delete (D8).
func (s *Service) Remove(ctx context.Context, knowledgeItemID string) (Item, error) {
	return s.setStatus(ctx, knowledgeItemID, StatusRemoved)
}

// Restore puts an item back into the rotation, undoing either exit.
//
// The counters and the cards were never touched by leaving, so coming back needs no
// repair: the item resumes with the history it had. What it does not restore is the
// schedule the cards would have had if they had stayed — due dates kept ticking, so a
// long-retired item comes back due, which is the right answer for something the user
// just said they do not know after all.
func (s *Service) Restore(ctx context.Context, knowledgeItemID string) (Item, error) {
	return s.setStatus(ctx, knowledgeItemID, StatusActive)
}

func (s *Service) setStatus(ctx context.Context, knowledgeItemID, status string) (Item, error) {
	if knowledgeItemID == "" {
		return Item{}, fmt.Errorf("%w: knowledge item id is required", ErrInvalidInput)
	}
	item, err := s.repo.SetStatus(ctx, knowledgeItemID, status, s.now().UTC())
	if err != nil {
		return Item{}, err
	}
	return withScores(item), nil
}

// withScores fills the derived numbers. They are computed from the same pure
// functions the review domain and the dashboard use, so an item's weakness means the
// same thing everywhere it is shown.
func withScores(item Item) Item {
	item.Accuracy = review.Accuracy(item.AttemptCount, item.CorrectCount)
	item.WeaknessScore = review.WeaknessScore(
		float64(item.AskCount), float64(item.UnknownCount), item.Accuracy, item.Attempted(),
	)
	return item
}
