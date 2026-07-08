package review

// GradeCounts is how many times a knowledge item has been graded at each rating,
// aggregated across all of its review cards.
type GradeCounts struct {
	Again int
	Hard  int
	Good  int
	Easy  int
}

// Scoring weights (PRD §13.2, §13.3).
const (
	masteryGoodWeight  = 0.2
	masteryEasyWeight  = 0.3
	masteryAgainWeight = 0.4
	masteryHardWeight  = 0.1

	weaknessAskWeight     = 0.2
	weaknessWrongWeight   = 0.5
	weaknessMasteryWeight = 0.7
)

// MasteryScore implements PRD §13.2, clamped to [0.0, 1.0].
func MasteryScore(counts GradeCounts) float64 {
	score := float64(counts.Good)*masteryGoodWeight +
		float64(counts.Easy)*masteryEasyWeight -
		float64(counts.Again)*masteryAgainWeight -
		float64(counts.Hard)*masteryHardWeight
	return clamp01(score)
}

// WeaknessScore implements PRD §13.3. It is not persisted (learner_items has no
// column) — it is derived on demand for review ordering and dashboards (#12).
// recentRepeatBonus is supplied by the caller (0 for the MVP), and the result is
// floored at 0 so it is usable as a sort key.
func WeaknessScore(askCount, wrongCount int, masteryScore, recentRepeatBonus float64) float64 {
	score := float64(askCount)*weaknessAskWeight +
		float64(wrongCount)*weaknessWrongWeight +
		recentRepeatBonus -
		masteryScore*weaknessMasteryWeight
	if score < 0 {
		return 0
	}
	return score
}

func clamp01(value float64) float64 {
	if value < 0 {
		return 0
	}
	if value > 1 {
		return 1
	}
	return value
}
