package review

import (
	"context"
	"errors"
	"testing"
	"time"
)

// A user who wants to see everything more often sets short first intervals and low
// multipliers; one cramming for something next week does the opposite. Both have to
// come out of NextSchedule unchanged, or the setting is decoration.
func TestNextScheduleUsesCustomIntervals(t *testing.T) {
	now := time.Date(2026, 7, 27, 12, 0, 0, 0, time.UTC)
	intense := Intervals{
		AgainMinutes:   2,
		FirstHardDays:  0.5,
		FirstGoodDays:  1,
		FirstEasyDays:  2,
		HardMultiplier: 1,
		GoodMultiplier: 1.5,
		EasyMultiplier: 2,
	}

	cases := []struct {
		name     string
		reps     int
		prevDays float64
		rating   string
		wantDays float64
		wantReps int
		wantSt   string
	}{
		{"again uses the custom minutes", 0, 0, RatingAgain, 2.0 / (24 * 60), 0, CardStateLearning},
		{"first hard", 0, 0, RatingHard, 0.5, 1, CardStateReview},
		{"first good", 0, 0, RatingGood, 1, 1, CardStateReview},
		{"first easy", 0, 0, RatingEasy, 2, 1, CardStateReview},
		// A multiplier of exactly 1 is allowed: "keep asking me at this interval".
		{"hard holds the interval", 3, 4, RatingHard, 4, 4, CardStateReview},
		{"good multiplies", 3, 4, RatingGood, 6, 4, CardStateReview},
		{"easy multiplies", 3, 4, RatingEasy, 8, 4, CardStateReview},
		// A lapse still resets to relearning, whatever the schedule says.
		{"again after many reps resets", 9, 60, RatingAgain, 2.0 / (24 * 60), 0, CardStateLearning},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			got, err := NextSchedule(test.reps, test.prevDays, test.rating, now, intense)
			if err != nil {
				t.Fatalf("NextSchedule() error = %v", err)
			}
			if !approx(got.IntervalDays, test.wantDays) {
				t.Errorf("interval = %v, want %v", got.IntervalDays, test.wantDays)
			}
			if got.Reps != test.wantReps || got.State != test.wantSt {
				t.Errorf("reps=%d state=%q, want %d/%q", got.Reps, got.State, test.wantReps, test.wantSt)
			}
			if !got.DueAt.Equal(addDays(now, test.wantDays)) {
				t.Errorf("dueAt = %v, want %v", got.DueAt, addDays(now, test.wantDays))
			}
		})
	}
}

// A zero Intervals reaching NextSchedule would otherwise make every interval 0, so
// every card would be due the moment it was graded — a wiring mistake that silently
// flattens someone's whole schedule instead of failing.
func TestNextScheduleZeroIntervalsFallsBackToDefaults(t *testing.T) {
	now := time.Date(2026, 7, 27, 12, 0, 0, 0, time.UTC)
	for _, rating := range []string{RatingAgain, RatingHard, RatingGood, RatingEasy} {
		zero, err := NextSchedule(0, 0, rating, now, Intervals{})
		if err != nil {
			t.Fatalf("%s: NextSchedule() error = %v", rating, err)
		}
		fallback, err := NextSchedule(0, 0, rating, now, DefaultIntervals())
		if err != nil {
			t.Fatalf("%s: NextSchedule(default) error = %v", rating, err)
		}
		if zero != fallback {
			t.Errorf("%s: zero intervals gave %+v, want the default %+v", rating, zero, fallback)
		}
	}
}

// Partly-filled intervals fill only the gaps: what the user did set is honoured.
func TestNextSchedulePartialIntervalsKeepSetFields(t *testing.T) {
	now := time.Date(2026, 7, 27, 12, 0, 0, 0, time.UTC)
	got, err := NextSchedule(0, 0, RatingGood, now, Intervals{FirstGoodDays: 5})
	if err != nil {
		t.Fatalf("NextSchedule() error = %v", err)
	}
	if !approx(got.IntervalDays, 5) {
		t.Errorf("interval = %v, want the 5 that was set", got.IntervalDays)
	}
}

func TestIntervalsValidate(t *testing.T) {
	valid := DefaultIntervals()
	if err := valid.Validate(); err != nil {
		t.Fatalf("the default schedule must be valid: %v", err)
	}

	tests := []struct {
		name   string
		mutate func(*Intervals)
	}{
		{"zero again", func(i *Intervals) { i.AgainMinutes = 0 }},
		{"negative again", func(i *Intervals) { i.AgainMinutes = -5 }},
		{"again longer than a day", func(i *Intervals) { i.AgainMinutes = 24*60 + 1 }},
		{"zero first good", func(i *Intervals) { i.FirstGoodDays = 0 }},
		{"first easy over a year", func(i *Intervals) { i.FirstEasyDays = 366 }},
		// Below 1 the interval shrinks on every success, so a card the user always
		// gets right ends up asked more and more often — the opposite of a schedule.
		{"shrinking good multiplier", func(i *Intervals) { i.GoodMultiplier = 0.9 }},
		{"absurd easy multiplier", func(i *Intervals) { i.EasyMultiplier = 11 }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			intervals := DefaultIntervals()
			test.mutate(&intervals)
			if err := intervals.Validate(); !errors.Is(err, ErrInvalidInput) {
				t.Errorf("Validate() = %v, want ErrInvalidInput", err)
			}
		})
	}
}

// A multiplier of exactly 1 is a legitimate choice, not a mistake.
func TestIntervalsValidateAcceptsHoldingMultiplier(t *testing.T) {
	intervals := DefaultIntervals()
	intervals.HardMultiplier = 1
	if err := intervals.Validate(); err != nil {
		t.Errorf("Validate() = %v, want a multiplier of 1 to be allowed", err)
	}
}

type stubIntervalSource struct {
	intervals Intervals
	err       error
	calls     int
}

func (s *stubIntervalSource) ReviewIntervals(context.Context) (Intervals, error) {
	s.calls++
	return s.intervals, s.err
}

func TestServiceGradeUsesLoadedIntervals(t *testing.T) {
	custom := DefaultIntervals()
	custom.FirstGoodDays = 42
	source := &stubIntervalSource{intervals: custom}
	repo := &fakeRepo{}
	svc := NewService(repo, source)

	if _, err := svc.Grade(context.Background(), GradeInput{CardID: "c1", Rating: RatingGood}); err != nil {
		t.Fatalf("Grade() error = %v", err)
	}
	if source.calls != 1 {
		t.Errorf("interval source called %d times, want 1", source.calls)
	}
	if repo.gradeIntervals != custom {
		t.Errorf("repo got intervals %+v, want the loaded %+v", repo.gradeIntervals, custom)
	}
}

// Reading the schedule is per-grade so that changing it in Settings applies to the
// next answer rather than the next launch.
func TestServiceGradeReloadsIntervalsEachTime(t *testing.T) {
	source := &stubIntervalSource{intervals: DefaultIntervals()}
	svc := NewService(&fakeRepo{}, source)

	for i := 0; i < 3; i++ {
		if _, err := svc.Grade(context.Background(), GradeInput{CardID: "c1", Rating: RatingGood}); err != nil {
			t.Fatalf("Grade() error = %v", err)
		}
	}
	if source.calls != 3 {
		t.Errorf("interval source called %d times, want 3", source.calls)
	}
}

func TestServiceGradeFailsWhenIntervalsCannotBeLoaded(t *testing.T) {
	repo := &fakeRepo{}
	svc := NewService(repo, &stubIntervalSource{err: errors.New("settings unavailable")})

	if _, err := svc.Grade(context.Background(), GradeInput{CardID: "c1", Rating: RatingGood}); err == nil {
		t.Fatal("Grade() error = nil, want the load failure surfaced")
	}
	if repo.gradeCardID != "" {
		t.Error("a card must not be rescheduled on a schedule nobody could read")
	}
}

func TestNewServiceDefaultsToDefaultIntervals(t *testing.T) {
	repo := &fakeRepo{}
	svc := NewService(repo, nil)

	if _, err := svc.Grade(context.Background(), GradeInput{CardID: "c1", Rating: RatingGood}); err != nil {
		t.Fatalf("Grade() error = %v", err)
	}
	if repo.gradeIntervals != DefaultIntervals() {
		t.Errorf("repo got %+v, want the default schedule", repo.gradeIntervals)
	}
}

// Practice never reschedules, so it has no business reading the schedule at all.
func TestServiceGradePracticeDoesNotLoadIntervals(t *testing.T) {
	source := &stubIntervalSource{intervals: DefaultIntervals()}
	svc := NewService(&fakeRepo{}, source)

	if _, err := svc.GradePractice(context.Background(), GradeInput{CardID: "c1", Rating: RatingGood}); err != nil {
		t.Fatalf("GradePractice() error = %v", err)
	}
	if source.calls != 0 {
		t.Errorf("interval source called %d times, want 0", source.calls)
	}
}
