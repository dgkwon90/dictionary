package review

import (
	"errors"
	"math"
	"testing"
	"time"
)

func TestNextScheduleFirstReview(t *testing.T) {
	now := time.Date(2026, 7, 8, 12, 0, 0, 0, time.UTC)
	cases := []struct {
		rating   string
		wantDays float64
		wantReps int
		wantSt   string
	}{
		{RatingAgain, 10.0 / (24 * 60), 0, CardStateLearning},
		{RatingHard, 1.0, 1, CardStateReview},
		{RatingGood, 3.0, 1, CardStateReview},
		{RatingEasy, 7.0, 1, CardStateReview},
	}
	for _, c := range cases {
		s, err := NextSchedule(0, 0, c.rating, now)
		if err != nil {
			t.Fatalf("%s: NextSchedule() error = %v", c.rating, err)
		}
		if !approx(s.IntervalDays, c.wantDays) {
			t.Errorf("%s: interval = %v, want %v", c.rating, s.IntervalDays, c.wantDays)
		}
		if s.Reps != c.wantReps || s.State != c.wantSt {
			t.Errorf("%s: reps=%d state=%q, want %d/%q", c.rating, s.Reps, s.State, c.wantReps, c.wantSt)
		}
		if !s.DueAt.Equal(addDays(now, c.wantDays)) {
			t.Errorf("%s: dueAt = %v", c.rating, s.DueAt)
		}
	}
}

func TestNextScheduleSubsequentMultipliesPrevInterval(t *testing.T) {
	now := time.Date(2026, 7, 8, 12, 0, 0, 0, time.UTC)
	// prev interval 3 days, reps 1 (already had a first review)
	for _, c := range []struct {
		rating   string
		wantDays float64
	}{
		{RatingHard, 3.0 * 1.2},
		{RatingGood, 3.0 * 2.5},
		{RatingEasy, 3.0 * 4.0},
	} {
		s, err := NextSchedule(1, 3.0, c.rating, now)
		if err != nil {
			t.Fatalf("%s: error = %v", c.rating, err)
		}
		if !approx(s.IntervalDays, c.wantDays) {
			t.Errorf("%s: interval = %v, want %v", c.rating, s.IntervalDays, c.wantDays)
		}
		if s.Reps != 2 {
			t.Errorf("%s: reps = %d, want 2", c.rating, s.Reps)
		}
	}
}

func TestNextScheduleAgainResetsToRelearning(t *testing.T) {
	now := time.Date(2026, 7, 8, 12, 0, 0, 0, time.UTC)
	// A mature card (reps 4, 30-day interval) graded Again resets.
	s, err := NextSchedule(4, 30, RatingAgain, now)
	if err != nil {
		t.Fatalf("error = %v", err)
	}
	if s.Reps != 0 || !s.Lapsed || s.State != CardStateLearning {
		t.Fatalf("again reset = %#v", s)
	}
	if !approx(s.IntervalDays, 10.0/(24*60)) {
		t.Fatalf("again interval = %v, want 10min", s.IntervalDays)
	}
	// The next Good after a lapse uses the initial interval again, not a multiple.
	next, err := NextSchedule(s.Reps, s.IntervalDays, RatingGood, now)
	if err != nil {
		t.Fatalf("error = %v", err)
	}
	if !approx(next.IntervalDays, 3.0) {
		t.Fatalf("post-lapse good interval = %v, want initial 3d", next.IntervalDays)
	}
}

func TestNextScheduleRejectsUnknownRating(t *testing.T) {
	_, err := NextSchedule(0, 0, "bogus", time.Now())
	if !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("error = %v, want ErrInvalidInput", err)
	}
}

func approx(a, b float64) bool {
	return math.Abs(a-b) < 1e-9
}
