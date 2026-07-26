package review

import (
	"math"
	"testing"
)

func TestIsCorrect(t *testing.T) {
	tests := []struct {
		rating string
		want   bool
	}{
		{RatingAgain, false},
		// "어려움"은 떠올리긴 한 것이라 정답으로 센다. 오답으로 치면 정직하게 고른
		// 사용자가 손해를 봐서, 결국 다들 "보통"을 누르게 되고 지표가 무의미해진다.
		{RatingHard, true},
		{RatingGood, true},
		{RatingEasy, true},
	}
	for _, test := range tests {
		t.Run(test.rating, func(t *testing.T) {
			if got := IsCorrect(test.rating); got != test.want {
				t.Errorf("IsCorrect(%q) = %v, want %v", test.rating, got, test.want)
			}
		})
	}
}

func TestAccuracy(t *testing.T) {
	tests := []struct {
		name     string
		attempts int
		correct  int
		want     float64
	}{
		{"never attempted", 0, 0, 0},
		{"all correct", 4, 4, 1},
		{"all wrong", 4, 0, 0},
		{"three of four", 4, 3, 0.75},
		{"one of three", 3, 1, 1.0 / 3.0},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := Accuracy(test.attempts, test.correct); math.Abs(got-test.want) > 1e-9 {
				t.Errorf("Accuracy(%d, %d) = %v, want %v", test.attempts, test.correct, got, test.want)
			}
		})
	}
}

func TestWeaknessScore(t *testing.T) {
	tests := []struct {
		name      string
		ask       float64
		unknown   float64
		accuracy  float64
		attempted bool
		want      float64
	}{
		{"asked and missed often, rarely right", 2, 1.5, 0.25, true, 0.975},
		{"perfect recall pulls the score down", 2, 1.5, 1, true, 0.45},
		// 아직 안 풀어본 항목은 정답률 항 자체가 없다. 0%로 치면 갓 등록한 단어가
		// 곧바로 "가장 약한 항목"으로 올라와 복습 순서를 지배한다.
		{"never attempted has no accuracy term", 1, 0, 0, false, 0.2},
		{"floored at zero", 0, 0, 1, true, 0},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := WeaknessScore(test.ask, test.unknown, test.accuracy, test.attempted)
			if math.Abs(got-test.want) > 1e-9 {
				t.Errorf("WeaknessScore(%v, %v, %v, %v) = %v, want %v",
					test.ask, test.unknown, test.accuracy, test.attempted, got, test.want)
			}
		})
	}
}

// An item the user has never been tested on must rank above one they always get
// right — otherwise brand-new words sink below mastered ones and never come up.
func TestWeaknessScoreRanksUntestedAboveMastered(t *testing.T) {
	untested := WeaknessScore(1, 1, 0, false)
	mastered := WeaknessScore(1, 1, 1, true)
	if untested <= mastered {
		t.Errorf("untested = %v, mastered = %v; want untested to rank higher", untested, mastered)
	}
}
