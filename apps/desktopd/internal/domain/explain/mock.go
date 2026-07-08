package explain

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

type MockExplainer struct{}

func NewMockExplainer() *MockExplainer {
	return &MockExplainer{}
}

func (m *MockExplainer) Explain(_ context.Context, text string) (ExplainResult, string, error) {
	trimmed := strings.TrimSpace(text)
	inputType := mockInputType(trimmed)
	itemType := inputType
	if itemType == "sentence" {
		itemType = "phrase"
	}
	result := ExplainResult{
		InputType:        inputType,
		DetectedLanguage: "en",
		BriefKo:          fmt.Sprintf("%q에 대한 목업 해석입니다.", trimmed),
		DetailedKo:       fmt.Sprintf("%q를 한국어 학습용으로 설명하는 목업 상세 해석입니다.", trimmed),
		PronunciationKo:  "목업 발음",
		DomainCategory:   "general",
		Difficulty:       0.5,
		Examples: []Example{{
			English: trimmed,
			Korean:  "목업 예문 번역입니다.",
			Note:    "목업 예문 메모입니다.",
		}},
		SubItems: []SubItem{{
			SurfaceText:     trimmed,
			NormalizedKey:   strings.ToLower(trimmed),
			ItemType:        itemType,
			MeaningKo:       "목업 의미입니다.",
			PronunciationKo: "목업 발음",
			Importance:      0.5,
		}},
		ReviewCardCandidates: []ReviewCardCandidate{{
			CardType:    "meaning",
			Question:    fmt.Sprintf("%q의 의미는 무엇인가요?", trimmed),
			Answer:      "목업 답변입니다.",
			Explanation: "목업 카드 설명입니다.",
		}},
	}
	if err := result.Validate(); err != nil {
		return ExplainResult{}, "", err
	}
	rawResponseJSON, err := json.Marshal(result)
	if err != nil {
		return ExplainResult{}, "", err
	}
	return result, string(rawResponseJSON), nil
}

func mockInputType(text string) string {
	fields := strings.Fields(text)
	if len(fields) <= 1 {
		return "word"
	}
	if strings.HasSuffix(text, ".") || strings.HasSuffix(text, "?") || strings.HasSuffix(text, "!") {
		return "sentence"
	}
	return "phrase"
}
