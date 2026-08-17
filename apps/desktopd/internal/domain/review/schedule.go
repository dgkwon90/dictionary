package review

import (
	"fmt"
	"time"
)

// Schedule is the outcome of grading a card: the new interval, the next due time,
// the resulting state, and the new repetition count. Again is treated as a lapse
// that resets reps to 0, so the next successful grade uses the initial intervals
// again (relearning), which is how we read §13.1's "Again: 간격 초기화".
type Schedule struct {
	IntervalDays float64
	DueAt        time.Time
	State        string
	Reps         int
	Lapsed       bool
}

// NextSchedule computes the next schedule for a card given how many consecutive
// successful reviews it already has (reps), its previous interval in days, the
// rating, and the schedule to apply. reps == 0 means this is a first/relearning
// review that uses the initial intervals; otherwise Hard/Good/Easy multiply the
// previous interval.
//
// A zero-value intervals argument means the default schedule (see withDefaults).
func NextSchedule(reps int, prevIntervalDays float64, rating string, now time.Time, intervals Intervals) (Schedule, error) {
	intervals = intervals.withDefaults()
	firstReview := reps <= 0

	var intervalDays float64
	switch rating {
	case RatingAgain:
		againDays := intervals.AgainMinutes / (24 * 60)
		return Schedule{
			IntervalDays: againDays,
			DueAt:        addDays(now, againDays),
			State:        CardStateLearning,
			Reps:         0,
			Lapsed:       true,
		}, nil
	case RatingHard:
		if firstReview {
			intervalDays = intervals.FirstHardDays
		} else {
			intervalDays = prevIntervalDays * intervals.HardMultiplier
		}
	case RatingGood:
		if firstReview {
			intervalDays = intervals.FirstGoodDays
		} else {
			intervalDays = prevIntervalDays * intervals.GoodMultiplier
		}
	case RatingEasy:
		if firstReview {
			intervalDays = intervals.FirstEasyDays
		} else {
			intervalDays = prevIntervalDays * intervals.EasyMultiplier
		}
	default:
		return Schedule{}, fmt.Errorf("%w: unknown rating %q", ErrInvalidInput, rating)
	}

	return Schedule{
		IntervalDays: intervalDays,
		DueAt:        addDays(now, intervalDays),
		State:        CardStateReview,
		Reps:         reps + 1,
		Lapsed:       false,
	}, nil
}

func addDays(now time.Time, days float64) time.Time {
	return now.Add(time.Duration(days * float64(24*time.Hour)))
}

// ValidRating reports whether rating is one of the four accepted grades.
func ValidRating(rating string) bool {
	switch rating {
	case RatingAgain, RatingHard, RatingGood, RatingEasy:
		return true
	default:
		return false
	}
}
