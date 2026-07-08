package suggest

import "context"

// MockSuggester returns a deterministic stub candidate so the flow works without an
// AI provider (real inference needs the model — see gemini implementation).
type MockSuggester struct{}

func NewMockSuggester() *MockSuggester {
	return &MockSuggester{}
}

func (m *MockSuggester) Suggest(_ context.Context, query string) ([]Candidate, error) {
	return []Candidate{{
		English:    "mock",
		Confidence: 0.5,
		GlossKo:    "목업 후보입니다: " + query,
	}}, nil
}
