package explain

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

type MockExplainer struct{}

func NewMockExplainer() *MockExplainer {
	return &MockExplainer{}
}

func (m *MockExplainer) Explain(_ context.Context, text string, format Format) (ExplainResult, string, error) {
	format = format.Normalized()
	trimmed := strings.TrimSpace(text)
	inputType := mockInputType(trimmed)

	result := ExplainResult{
		InputType:        inputType,
		DetectedLanguage: "en",
		BriefKo:          fmt.Sprintf("%q에 대한 목업 해석입니다.", trimmed),
		DetailedKo:       fmt.Sprintf("%q를 한국어 학습용으로 설명하는 목업 상세 해석입니다.", trimmed),
		PronunciationKo:  "목업 발음",
		DomainCategory:   "general",
		Difficulty:       0.5,
		Examples:         mockExamples(trimmed, format.ExamplesCount),
		SubItems:         mockSubItems(trimmed, inputType),
	}
	// A sentence lookup gets the sentence object too, so the selection flow and its
	// cards can be exercised end to end without an API key. The mock used to return
	// one sub_item covering the whole input, which meant the sentence path was never
	// actually walked locally — that is how the previous contract's gaps stayed
	// invisible until a real provider ran.
	if inputType == "sentence" || inputType == "error_message" {
		result.Sentence = &Sentence{
			TranslationKo: fmt.Sprintf("%q의 목업 번역입니다.", trimmed),
			StructureKo:   "목업 문장 구조 설명입니다.",
		}
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

func mockExamples(text string, count int) []Example {
	examples := make([]Example, 0, count)
	for i := 0; i < count; i++ {
		examples = append(examples, Example{
			English: text,
			Korean:  fmt.Sprintf("목업 예문 번역 %d입니다.", i+1),
			Note:    "목업 예문 메모입니다.",
		})
	}
	return examples
}

// mockSubItems picks words out of a sentence instead of returning the whole input as a
// single item, so that offsets, cloze blanks, and multi-word selection all have
// something real to work on in mock mode.
func mockSubItems(text, inputType string) []SubItem {
	if inputType != "sentence" && inputType != "error_message" {
		return []SubItem{mockSubItem(text, text, itemTypeFor(inputType))}
	}
	items := []SubItem{}
	for _, word := range mockPickWords(text, 2) {
		items = append(items, mockSubItem(strings.ToLower(word), word, "word"))
	}
	if len(items) == 0 {
		// Nothing word-like in the input; fall back so sub_items is never empty.
		items = append(items, mockSubItem(text, text, "phrase"))
	}
	return items
}

func mockSubItem(surfaceText, surfaceInText, itemType string) SubItem {
	return SubItem{
		SurfaceText:     surfaceText,
		SurfaceInText:   surfaceInText,
		NormalizedKey:   strings.ToLower(surfaceText),
		ItemType:        itemType,
		MeaningKo:       fmt.Sprintf("%q의 목업 의미입니다.", surfaceText),
		PronunciationKo: "목업 발음",
		DescriptionKo:   fmt.Sprintf("%q를 문맥에서 다시 설명하는 목업 본문입니다.", surfaceText),
		Importance:      0.5,
		CardCandidates: []ReviewCardCandidate{{
			CardType:    "meaning",
			Question:    fmt.Sprintf("%q의 의미는 무엇인가요?", surfaceText),
			Answer:      "목업 답변입니다.",
			Explanation: "목업 카드 설명입니다.",
		}},
	}
}

// mockPickWords takes the longest words in the text, which are the ones a learner is
// most likely to be stuck on and, more usefully here, are unambiguous to locate.
func mockPickWords(text string, limit int) []string {
	seen := map[string]struct{}{}
	words := []string{}
	for _, field := range strings.Fields(text) {
		word := strings.Trim(field, ".,!?;:\"'()[]{}")
		if len([]rune(word)) < 3 {
			continue
		}
		key := strings.ToLower(word)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		words = append(words, word)
	}
	sort.SliceStable(words, func(i, j int) bool {
		return len([]rune(words[i])) > len([]rune(words[j]))
	})
	if len(words) > limit {
		words = words[:limit]
	}
	return words
}

func itemTypeFor(inputType string) string {
	if inputType == "sentence" || inputType == "error_message" {
		return "phrase"
	}
	return inputType
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
