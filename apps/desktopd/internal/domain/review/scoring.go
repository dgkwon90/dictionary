package review

// IsCorrect decides whether a self-graded rating counts as a hit.
//
// Only "again" — I could not recall it — is wrong. "hard" means the answer did come
// back, just with effort, and counting that as a miss would punish honest reporting:
// the user learns to press "good" for everything and the number stops meaning
// anything. The three-way alternative (excluding hard from the denominator) was
// rejected because it makes two items' accuracy incomparable.
func IsCorrect(rating string) bool {
	return rating != RatingAgain
}

// Accuracy is the share of attempts the user got right, in [0, 1]. It returns 0 for
// an item that has never been attempted; callers that need to tell "never tried"
// apart from "always wrong" should look at the attempt count, which is why the
// counts travel alongside it rather than being collapsed into this number.
func Accuracy(attemptCount, correctCount int) float64 {
	if attemptCount <= 0 {
		return 0
	}
	return float64(correctCount) / float64(attemptCount)
}

// Weakness weights (PRD §13.3, restated in terms of accuracy).
const (
	weaknessUnknownWeight  = 0.5
	weaknessAskWeight      = 0.2
	weaknessAccuracyWeight = 0.7
)

// WeaknessScore ranks how much an item needs attention. It is not persisted — it is
// derived on demand for ordering and dashboards.
//
// Inputs are float64 so callers can pass either one item's counts or averages across
// a group (e.g. per-category means). The result is floored at 0 so it is usable as a
// sort key.
//
// Note the asymmetry with accuracy: an item with no attempts contributes no accuracy
// term at all, so a freshly registered word ranks by how often it was not known
// rather than being treated as 0% correct.
func WeaknessScore(askCount, unknownCount, accuracy float64, attempted bool) float64 {
	score := askCount*weaknessAskWeight + unknownCount*weaknessUnknownWeight
	if attempted {
		score -= accuracy * weaknessAccuracyWeight
	}
	if score < 0 {
		return 0
	}
	return score
}
