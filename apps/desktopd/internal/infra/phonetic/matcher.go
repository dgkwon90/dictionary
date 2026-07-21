package phonetic

import (
	"context"

	"neulsang/desktopd/internal/domain/suggest"
)

type Matcher struct {
	topK      int
	threshold float64
}

var _ suggest.Suggester = (*Matcher)(nil)

func NewMatcher() *Matcher {
	return &Matcher{topK: defaultTopK, threshold: defaultScoreThreshold}
}

func (m *Matcher) Suggest(ctx context.Context, query string) ([]suggest.Candidate, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	queryPhones := hangulToARPAbet(query)
	if len(queryPhones) == 0 {
		return []suggest.Candidate{}, nil
	}

	entries, err := loadEntries()
	if err != nil {
		return nil, err
	}

	matches, err := matchEntries(ctx, queryVariants(queryPhones), entries, m.topK, m.threshold)
	if err != nil {
		return nil, err
	}

	candidates := make([]suggest.Candidate, 0, len(matches))
	for _, matched := range matches {
		candidates = append(candidates, suggest.Candidate{
			English:    matched.word,
			Confidence: clampScore(matched.score),
			GlossKo:    "",
			Source:     suggest.SourceLocal,
		})
	}
	return candidates, nil
}
