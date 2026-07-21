package phonetic

import (
	"context"
	"sort"
)

const (
	defaultTopK           = 8
	defaultScoreThreshold = 0.45
	ctxCheckInterval      = 512
)

var vowelPhones = map[string]struct{}{
	"AA": {}, "AE": {}, "AH": {}, "AO": {}, "AW": {}, "AY": {}, "EH": {}, "ER": {},
	"EY": {}, "IH": {}, "IY": {}, "OW": {}, "OY": {}, "UH": {}, "UW": {},
}

var diphthongPairs = map[[2]string]string{
	{"EH", "IY"}: "EY",
}

type match struct {
	word  string
	score float64
}

func isVowel(phone string) bool {
	_, ok := vowelPhones[phone]
	return ok
}

func queryVariants(query []string) [][]string {
	collapsed := collapseDiphthongs(query)
	if equalPhones(query, collapsed) {
		return [][]string{query}
	}
	return [][]string{query, collapsed}
}

func collapseDiphthongs(query []string) []string {
	collapsed := make([]string, 0, len(query))
	for i := 0; i < len(query); i++ {
		if i+1 < len(query) {
			if phone, ok := diphthongPairs[[2]string{query[i], query[i+1]}]; ok {
				collapsed = append(collapsed, phone)
				i++
				continue
			}
		}
		collapsed = append(collapsed, query[i])
	}
	return collapsed
}

func matchEntries(ctx context.Context, queries [][]string, entries []entry, topK int, threshold float64) ([]match, error) {
	if topK <= 0 {
		topK = defaultTopK
	}
	if threshold <= 0 {
		threshold = defaultScoreThreshold
	}

	workspaceLen := maxQueryLen(queries) + 4
	previous := make([]float64, workspaceLen)
	current := make([]float64, workspaceLen)
	bestByWord := make(map[string]match, 128)
	for i, candidate := range entries {
		if i%ctxCheckInterval == 0 {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
		}
		score, ok := bestVariantScore(queries, candidate.phones, previous, current)
		if !ok {
			continue
		}
		if score < threshold {
			continue
		}

		next := match{word: candidate.word, score: score}
		if currentBest, ok := bestByWord[candidate.word]; !ok || betterMatch(next, currentBest) {
			bestByWord[candidate.word] = next
		}
	}

	matches := make([]match, 0, len(bestByWord))
	for _, candidate := range bestByWord {
		matches = append(matches, candidate)
	}
	sort.SliceStable(matches, func(i, j int) bool {
		return betterMatch(matches[i], matches[j])
	})
	if len(matches) > topK {
		matches = matches[:topK]
	}
	return matches, nil
}

func bestVariantScore(queries [][]string, phones []string, previous, current []float64) (float64, bool) {
	bestScore := 0.0
	ok := false
	for _, query := range queries {
		if absInt(len(query)-len(phones)) > 3 {
			continue
		}
		score := phonemeScoreWithWorkspace(query, phones, previous[:len(phones)+1], current[:len(phones)+1])
		if !ok || score > bestScore {
			bestScore = score
			ok = true
		}
	}
	return bestScore, ok
}

func weightedDistance(a, b []string) float64 {
	previous := make([]float64, len(b)+1)
	current := make([]float64, len(b)+1)
	return weightedDistanceWithWorkspace(a, b, previous, current)
}

func phonemeScore(a, b []string) float64 {
	previous := make([]float64, len(b)+1)
	current := make([]float64, len(b)+1)
	return phonemeScoreWithWorkspace(a, b, previous, current)
}

func phonemeScoreWithWorkspace(a, b []string, previous, current []float64) float64 {
	maxLen := maxInt(len(a), len(b))
	if maxLen == 0 {
		return 1
	}

	score := 1 - weightedDistanceWithWorkspace(a, b, previous, current)/float64(maxLen)
	return clampScore(score)
}

func weightedDistanceWithWorkspace(a, b []string, previous, current []float64) float64 {
	previous[0] = 0
	for j := 1; j <= len(b); j++ {
		previous[j] = previous[j-1] + indelCost(b[j-1])
	}

	for i := 1; i <= len(a); i++ {
		current[0] = previous[0] + indelCost(a[i-1])
		for j := 1; j <= len(b); j++ {
			deleteCost := previous[j] + indelCost(a[i-1])
			insertCost := current[j-1] + indelCost(b[j-1])
			substituteCost := previous[j-1] + substitutionCost(a[i-1], b[j-1])
			current[j] = minFloat(deleteCost, insertCost, substituteCost)
		}
		previous, current = current, previous
	}
	return previous[len(b)]
}

func substitutionCost(a, b string) float64 {
	if a == b {
		return 0
	}
	if isVowel(a) == isVowel(b) {
		return 0.5
	}
	return 1
}

func indelCost(phone string) float64 {
	if isVowel(phone) {
		return 0.5
	}
	return 1
}

func betterMatch(a, b match) bool {
	if a.score != b.score {
		return a.score > b.score
	}
	if len(a.word) != len(b.word) {
		return len(a.word) < len(b.word)
	}
	return a.word < b.word
}

func clampScore(score float64) float64 {
	if score < 0 {
		return 0
	}
	if score > 1 {
		return 1
	}
	return score
}

func absInt(value int) int {
	if value < 0 {
		return -value
	}
	return value
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func maxQueryLen(queries [][]string) int {
	maxLen := 0
	for _, query := range queries {
		if len(query) > maxLen {
			maxLen = len(query)
		}
	}
	return maxLen
}

func minFloat(a, b, c float64) float64 {
	if b < a {
		a = b
	}
	if c < a {
		a = c
	}
	return a
}

func equalPhones(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
