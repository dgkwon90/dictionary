package suggest

import (
	"context"
	"fmt"
	"strings"
	"time"
)

type Service struct {
	suggester Suggester
	repo      Repository // optional cache; nil = AI-only
	now       func() time.Time
}

func NewService(suggester Suggester, repo Repository) *Service {
	return &Service{suggester: suggester, repo: repo, now: time.Now}
}

// Suggest returns candidate English words for a Korean phonetic query. It answers
// from the confirmed-pick cache first (instant, offline, no AI cost) and falls back
// to the AI suggester on a cache miss (Phase 2, backlog #21).
func (s *Service) Suggest(ctx context.Context, query string) ([]Candidate, error) {
	normalized, err := normalizeQuery(query)
	if err != nil {
		return nil, err
	}

	if s.repo != nil {
		cached, err := s.repo.Cached(ctx, normalized)
		if err != nil {
			return nil, err
		}
		if len(cached) > 0 {
			return cached, nil
		}
	}

	if s.suggester == nil {
		return nil, ErrUnavailable
	}
	return s.suggester.Suggest(ctx, normalized)
}

// ConfirmPick records that the user chose english for query, so the cache can answer
// the same query next time. Requires a configured cache.
func (s *Service) ConfirmPick(ctx context.Context, query, english, glossKo string) error {
	normalized, err := normalizeQuery(query)
	if err != nil {
		return err
	}
	if strings.TrimSpace(english) == "" {
		return fmt.Errorf("%w: english is required", ErrInvalidInput)
	}
	if s.repo == nil {
		return ErrUnavailable
	}
	return s.repo.SavePick(ctx, normalized, strings.TrimSpace(english), strings.TrimSpace(glossKo), s.now().UTC())
}

// normalizeQuery trims and lowercases the query for a stable exact-match cache key.
// Lowercasing is a no-op for Hangul but normalizes any latin the user typed.
func normalizeQuery(query string) (string, error) {
	trimmed := strings.TrimSpace(query)
	if trimmed == "" {
		return "", fmt.Errorf("%w: query is required", ErrInvalidInput)
	}
	if len([]rune(trimmed)) > MaxQueryLen {
		return "", fmt.Errorf("%w: query too long", ErrInvalidInput)
	}
	return strings.ToLower(trimmed), nil
}
