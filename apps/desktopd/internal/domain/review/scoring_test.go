package review

import "testing"

func TestMasteryScore(t *testing.T) {
	cases := []struct {
		name   string
		counts GradeCounts
		want   float64
	}{
		{"single good", GradeCounts{Good: 1}, 0.2},
		{"good+easy", GradeCounts{Good: 1, Easy: 1}, 0.5},
		{"again pulls down and clamps to 0", GradeCounts{Again: 1}, 0.0},
		{"mixed clamps to 0", GradeCounts{Good: 1, Again: 2, Hard: 1}, 0.0}, // 0.2-0.8-0.1<0
		{"many easy clamps to 1", GradeCounts{Easy: 5}, 1.0},                // 1.5 -> 1.0
		{"empty", GradeCounts{}, 0.0},
	}
	for _, c := range cases {
		if got := MasteryScore(c.counts); !approx(got, c.want) {
			t.Errorf("%s: MasteryScore(%#v) = %v, want %v", c.name, c.counts, got, c.want)
		}
	}
}

func TestWeaknessScore(t *testing.T) {
	// 3*0.2 + 2*0.5 + 0 - 0.5*0.7 = 0.6 + 1.0 - 0.35 = 1.25
	if got := WeaknessScore(3, 2, 0.5, 0); !approx(got, 1.25) {
		t.Errorf("WeaknessScore = %v, want 1.25", got)
	}
	// recent_repeat_bonus is added
	if got := WeaknessScore(0, 0, 0, 0.4); !approx(got, 0.4) {
		t.Errorf("WeaknessScore with bonus = %v, want 0.4", got)
	}
	// high mastery floors weakness at 0 (never negative)
	if got := WeaknessScore(0, 0, 1.0, 0); got != 0 {
		t.Errorf("WeaknessScore floored = %v, want 0", got)
	}
}
