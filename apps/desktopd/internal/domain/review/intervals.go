package review

import (
	"context"
	"fmt"
)

// Intervals is the review schedule: how long to wait after each grade.
//
// Units differ per field on purpose, because that is how the numbers are thought
// about. A missed card comes back in minutes; a remembered one in days. Naming the
// unit in the field beats a uniform "everything in days" where Again would read as
// 0.00694.
//
// The multipliers apply from the second successful review onward: the previous
// interval is multiplied rather than replaced, so a card the user keeps getting right
// drifts further and further out.
type Intervals struct {
	AgainMinutes   float64
	FirstHardDays  float64
	FirstGoodDays  float64
	FirstEasyDays  float64
	HardMultiplier float64
	GoodMultiplier float64
	EasyMultiplier float64
}

// Default schedule (PRD §13.1). These were the constants NextSchedule used before the
// schedule became configurable, so an install that never touches Settings behaves
// exactly as it did.
func DefaultIntervals() Intervals {
	return Intervals{
		AgainMinutes:   10,
		FirstHardDays:  1,
		FirstGoodDays:  3,
		FirstEasyDays:  7,
		HardMultiplier: 1.2,
		GoodMultiplier: 2.5,
		EasyMultiplier: 4.0,
	}
}

// Bounds for a schedule a person could actually follow.
const (
	minAgainMinutes = 1
	maxAgainMinutes = 24 * 60
	maxFirstDays    = 365
	maxMultiplier   = 10
)

// Validate checks a schedule the user is trying to save. It is the write boundary —
// Settings rejects a bad schedule with an error rather than quietly correcting it,
// matching settings.Preferences.
//
// Multipliers below 1 are refused because they do not make a schedule at all: each
// review would return sooner than the last, so a card the user always gets right
// converges on being asked constantly.
func (i Intervals) Validate() error {
	for _, field := range []struct {
		name     string
		value    float64
		min, max float64
	}{
		{"again_minutes", i.AgainMinutes, minAgainMinutes, maxAgainMinutes},
		{"first_hard_days", i.FirstHardDays, 0, maxFirstDays},
		{"first_good_days", i.FirstGoodDays, 0, maxFirstDays},
		{"first_easy_days", i.FirstEasyDays, 0, maxFirstDays},
		{"hard_multiplier", i.HardMultiplier, 1, maxMultiplier},
		{"good_multiplier", i.GoodMultiplier, 1, maxMultiplier},
		{"easy_multiplier", i.EasyMultiplier, 1, maxMultiplier},
	} {
		if field.value <= 0 {
			return fmt.Errorf("%w: %s must be greater than 0, got %v", ErrInvalidInput, field.name, field.value)
		}
		if field.value < field.min || field.value > field.max {
			return fmt.Errorf("%w: %s must be between %v and %v, got %v", ErrInvalidInput, field.name, field.min, field.max, field.value)
		}
	}
	return nil
}

// withDefaults fills in anything non-positive from the default schedule.
//
// This is not a second validation — user input is refused by Validate, not repaired.
// It exists so the zero Intervals{} means "the default schedule" rather than "every
// interval is zero", which would make every card due the instant it was graded and
// would be a very quiet way for a wiring mistake to destroy someone's schedule.
func (i Intervals) withDefaults() Intervals {
	defaults := DefaultIntervals()
	for _, field := range []struct{ target, fallback *float64 }{
		{&i.AgainMinutes, &defaults.AgainMinutes},
		{&i.FirstHardDays, &defaults.FirstHardDays},
		{&i.FirstGoodDays, &defaults.FirstGoodDays},
		{&i.FirstEasyDays, &defaults.FirstEasyDays},
		{&i.HardMultiplier, &defaults.HardMultiplier},
		{&i.GoodMultiplier, &defaults.GoodMultiplier},
		{&i.EasyMultiplier, &defaults.EasyMultiplier},
	} {
		if *field.target <= 0 {
			*field.target = *field.fallback
		}
	}
	return i
}

// IntervalSource supplies the schedule to grade with.
//
// Grading asks for it per card rather than caching it at startup, so changing the
// schedule in Settings takes effect on the next answer instead of the next launch.
// This is one local SQLite read on a single-user desktop app, not a network call.
type IntervalSource interface {
	ReviewIntervals(ctx context.Context) (Intervals, error)
}

// FixedIntervals is an IntervalSource that always answers with the same schedule.
type FixedIntervals Intervals

func (f FixedIntervals) ReviewIntervals(context.Context) (Intervals, error) {
	return Intervals(f), nil
}
