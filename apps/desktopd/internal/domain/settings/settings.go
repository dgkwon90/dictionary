// Package settings manages user-editable behavior preferences persisted in the
// app_settings table (PRD §10.7 / §11.1).
//
// Only "동작 정책·운영 튜닝" values live here. Infra/bootstrap config (DB path,
// listen addr, AI provider, and the Gemini API key) stays in environment variables:
// app_settings is a plaintext policy store and must not hold secrets. This split is
// recorded in ADR-0004 부록 (#17). The read-only reflection of that env config is
// surfaced by the transport layer, not this domain.
package settings

import (
	"context"
	"errors"
	"fmt"
	"regexp"
)

// ErrInvalidPreferences wraps validation failures so the transport layer can map them
// to 400 without importing validation details.
var ErrInvalidPreferences = errors.New("invalid preferences")

// Preferences are the persisted, user-editable behavior settings. Review times are
// "HH:MM" 24h local strings; they are stored now and consumed by the notification
// scheduler in #18.
type Preferences struct {
	NotificationsEnabled bool
	MorningReviewTime    string
	EveningReviewTime    string
}

// Defaults returns the preferences used before the user has saved anything.
func Defaults() Preferences {
	return Preferences{
		NotificationsEnabled: true,
		MorningReviewTime:    "09:00",
		EveningReviewTime:    "21:00",
	}
}

var timeOfDay = regexp.MustCompile(`^([01]\d|2[0-3]):[0-5]\d$`)

// Validate checks the review-time fields are well-formed 24h HH:MM values.
func (p Preferences) Validate() error {
	if !timeOfDay.MatchString(p.MorningReviewTime) {
		return fmt.Errorf("%w: morning_review_time must be HH:MM 24h, got %q", ErrInvalidPreferences, p.MorningReviewTime)
	}
	if !timeOfDay.MatchString(p.EveningReviewTime) {
		return fmt.Errorf("%w: evening_review_time must be HH:MM 24h, got %q", ErrInvalidPreferences, p.EveningReviewTime)
	}
	return nil
}

// Repository persists preferences. Load returns Defaults()-filled values for any keys
// not yet stored, so callers never see zero values for unset settings.
type Repository interface {
	Load(ctx context.Context) (Preferences, error)
	Save(ctx context.Context, prefs Preferences) error
}

// Service is the preferences use case. It has no infra dependency (PRD §18.1).
type Service struct {
	repo Repository
}

func NewService(repo Repository) *Service {
	return &Service{repo: repo}
}

// Get returns the current preferences (defaults where unset).
func (s *Service) Get(ctx context.Context) (Preferences, error) {
	return s.repo.Load(ctx)
}

// Update validates then persists a full preferences set (PUT semantics: full replace).
func (s *Service) Update(ctx context.Context, prefs Preferences) (Preferences, error) {
	if err := prefs.Validate(); err != nil {
		return Preferences{}, err
	}
	if err := s.repo.Save(ctx, prefs); err != nil {
		return Preferences{}, err
	}
	return prefs, nil
}
