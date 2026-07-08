package review

import (
	"fmt"
	"time"
)

// Initial intervals for a card's first successful review (PRD §13.1), in days.
const (
	intervalAgainDays = 10.0 / (24 * 60) // 10 minutes
	initialHardDays   = 1.0
	initialGoodDays   = 3.0
	initialEasyDays   = 7.0
)

// Multipliers applied to the previous interval on later reviews (PRD §13.1).
const (
	multHard = 1.2
	multGood = 2.5
	multEasy = 4.0
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
// successful reviews it already has (reps), its previous interval in days, and the
// rating. reps == 0 means this is a first/relearning review that uses the initial
// intervals; otherwise Hard/Good/Easy multiply the previous interval.
func NextSchedule(reps int, prevIntervalDays float64, rating string, now time.Time) (Schedule, error) {
	firstReview := reps <= 0

	var intervalDays float64
	switch rating {
	case RatingAgain:
		return Schedule{
			IntervalDays: intervalAgainDays,
			DueAt:        addDays(now, intervalAgainDays),
			State:        CardStateLearning,
			Reps:         0,
			Lapsed:       true,
		}, nil
	case RatingHard:
		if firstReview {
			intervalDays = initialHardDays
		} else {
			intervalDays = prevIntervalDays * multHard
		}
	case RatingGood:
		if firstReview {
			intervalDays = initialGoodDays
		} else {
			intervalDays = prevIntervalDays * multGood
		}
	case RatingEasy:
		if firstReview {
			intervalDays = initialEasyDays
		} else {
			intervalDays = prevIntervalDays * multEasy
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
