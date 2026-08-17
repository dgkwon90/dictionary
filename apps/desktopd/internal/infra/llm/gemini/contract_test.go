package gemini

import (
	"encoding/json"
	"strings"
	"testing"

	"neulsang/desktopd/internal/domain/explain"
)

// goldenSentenceResponse is a hand-written v2 response for a sentence lookup, kept as
// literal JSON rather than a marshaled Go struct on purpose: it is the wire format the
// provider is asked for, so a renamed json tag has to fail here instead of quietly
// agreeing with itself on both sides of the boundary.
const goldenSentenceResponse = `{
  "input_type": "sentence",
  "detected_language": "en",
  "brief_ko": "배포 후 캐시가 오래된 상태가 되었다는 뜻입니다.",
  "detailed_ko": "become + 형용사 구문으로 상태 변화를 나타냅니다.",
  "pronunciation_ko": "더 캐시 비케임 스테일 애프터 디플로이",
  "domain_category": "backend",
  "difficulty": 0.45,
  "sentence": {
    "translation_ko": "배포 후에 캐시가 오래된 상태가 되었다.",
    "structure_ko": "become의 과거형 became가 '~한 상태가 되다'라는 변화를 나타냅니다."
  },
  "examples": [
    {"english": "The token became stale.", "korean": "토큰이 만료되었다.", "note": "개발 문맥"}
  ],
  "sub_items": [
    {
      "surface_text": "stale",
      "surface_in_text": "stale",
      "normalized_key": "stale",
      "item_type": "word",
      "meaning_ko": "오래되어 최신이 아닌",
      "pronunciation_ko": "스테일",
      "description_ko": "캐시가 원본과 어긋난 상태를 가리킵니다.",
      "importance": 0.9,
      "card_candidates": [
        {"card_type": "meaning", "question": "stale의 의미는?", "answer": "오래되어 최신이 아닌 상태", "explanation": "stale cache처럼 쓴다."}
      ]
    },
    {
      "surface_text": "deploy",
      "surface_in_text": "deploy",
      "normalized_key": "deploy",
      "item_type": "term",
      "meaning_ko": "배포",
      "pronunciation_ko": "디플로이",
      "description_ko": "코드를 실행 환경에 올리는 일입니다.",
      "importance": 0.6,
      "card_candidates": [
        {"card_type": "meaning", "question": "deploy의 의미는?", "answer": "배포"}
      ]
    }
  ]
}`

func TestParseResponseGoldenSentence(t *testing.T) {
	result, err := parseResponse(geminiResponse([]byte(goldenSentenceResponse)))
	if err != nil {
		t.Fatalf("parseResponse() error = %v", err)
	}
	if err := result.Validate(); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}

	if result.Sentence == nil {
		t.Fatal("sentence is nil, want the sentence object parsed")
	}
	if got, want := result.Sentence.TranslationKo, "배포 후에 캐시가 오래된 상태가 되었다."; got != want {
		t.Errorf("translation_ko = %q, want %q", got, want)
	}
	if !strings.Contains(result.Sentence.StructureKo, "became") {
		t.Errorf("structure_ko = %q, want the grammar note", result.Sentence.StructureKo)
	}

	if len(result.SubItems) != 2 {
		t.Fatalf("sub_items = %d, want 2", len(result.SubItems))
	}
	first := result.SubItems[0]
	if first.SurfaceInText != "stale" {
		t.Errorf("surface_in_text = %q, want %q", first.SurfaceInText, "stale")
	}
	if first.DescriptionKo == "" {
		t.Error("description_ko is empty, want the contextual explanation")
	}
	if len(first.CardCandidates) != 1 || first.CardCandidates[0].CardType != "meaning" {
		t.Errorf("card_candidates = %#v", first.CardCandidates)
	}

	// The whole point of the sentence object: the sentence's own card is answered with
	// a translation, not with brief_ko, which describes the sentence instead.
	if result.Sentence.TranslationKo == result.BriefKo {
		t.Error("translation_ko equals brief_ko; the fixture no longer distinguishes them")
	}
}

func TestParseResponseWordLookupHasNoSentence(t *testing.T) {
	const wordResponse = `{
  "input_type": "word",
  "detected_language": "en",
  "brief_ko": "짧은 설명",
  "detailed_ko": "자세한 설명",
  "domain_category": "general",
  "difficulty": 0.3,
  "sub_items": [
    {"surface_text": "stale", "normalized_key": "stale", "item_type": "word", "meaning_ko": "오래된",
     "card_candidates": [{"card_type": "meaning", "question": "q", "answer": "a"}]}
  ]
}`
	result, err := parseResponse(geminiResponse([]byte(wordResponse)))
	if err != nil {
		t.Fatalf("parseResponse() error = %v", err)
	}
	if result.Sentence != nil {
		t.Fatalf("sentence = %#v, want nil for a word lookup", result.Sentence)
	}
	if result.SubItems[0].SurfaceInText != "" {
		t.Errorf("surface_in_text = %q, want empty when the model omits it", result.SubItems[0].SurfaceInText)
	}
}

// An object present but empty is the same as no sentence at all. Left as a non-nil
// pointer it would make every caller check the pointer and then the string inside it.
func TestParseResponseDropsEmptySentence(t *testing.T) {
	const blankSentence = `{
  "input_type": "sentence",
  "detected_language": "en",
  "brief_ko": "짧은 설명",
  "detailed_ko": "자세한 설명",
  "domain_category": "general",
  "difficulty": 0.3,
  "sentence": {"translation_ko": "   ", "structure_ko": ""},
  "sub_items": [
    {"surface_text": "stale", "surface_in_text": "  ", "normalized_key": "stale", "item_type": "word", "meaning_ko": "오래된",
     "card_candidates": [{"card_type": "meaning", "question": "q", "answer": "a"}]}
  ]
}`
	result, err := parseResponse(geminiResponse([]byte(blankSentence)))
	if err != nil {
		t.Fatalf("parseResponse() error = %v", err)
	}
	if result.Sentence != nil {
		t.Errorf("sentence = %#v, want nil when it carries nothing", result.Sentence)
	}
	if result.SubItems[0].SurfaceInText != "" {
		t.Errorf("surface_in_text = %q, want empty after trimming whitespace", result.SubItems[0].SurfaceInText)
	}
}

func TestResponseSchemaExamplesCount(t *testing.T) {
	tests := []struct {
		name          string
		format        explain.Format
		wantExamples  bool
		wantMinAndMax int
	}{
		// Asking for none removes the field rather than capping it at zero: a model that
		// can see an examples array in the schema tends to fill it.
		{"none", explain.Format{ExamplesCount: 0}, false, 0},
		{"default", explain.DefaultFormat(), true, explain.DefaultExamplesCount},
		{"clamped to the maximum", explain.Format{ExamplesCount: 99}, true, explain.MaxExamplesCount},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			properties := schemaProperties(t, responseSchema(tt.format))
			examples, ok := properties["examples"]
			if ok != tt.wantExamples {
				t.Fatalf("examples present = %t, want %t", ok, tt.wantExamples)
			}
			if !ok {
				return
			}
			spec, isMap := examples.(map[string]any)
			if !isMap {
				t.Fatalf("examples = %#v, want an object", examples)
			}
			if spec["minItems"] != tt.wantMinAndMax || spec["maxItems"] != tt.wantMinAndMax {
				t.Fatalf("examples min/max = %v/%v, want %d", spec["minItems"], spec["maxItems"], tt.wantMinAndMax)
			}
		})
	}
}

// The server cuts cloze blanks out of the captured text, so the model must not be
// offered cloze as a card type: a model-written one would be a second, unverifiable
// source for the same card.
func TestResponseSchemaCardTypesExcludeServerBuiltCards(t *testing.T) {
	properties := schemaProperties(t, responseSchema(explain.DefaultFormat()))
	subItems, ok := properties["sub_items"].(map[string]any)
	if !ok {
		t.Fatalf("sub_items = %#v", properties["sub_items"])
	}
	items, ok := subItems["items"].(map[string]any)
	if !ok {
		t.Fatalf("sub_items.items = %#v", subItems["items"])
	}
	itemProperties, ok := items["properties"].(map[string]any)
	if !ok {
		t.Fatalf("sub_items.items.properties = %#v", items["properties"])
	}
	if _, ok := itemProperties["surface_in_text"]; !ok {
		t.Error("sub_items has no surface_in_text; the server cannot locate inflected words without it")
	}

	candidates, ok := itemProperties["card_candidates"].(map[string]any)
	if !ok {
		t.Fatalf("card_candidates = %#v", itemProperties["card_candidates"])
	}
	candidateItems, ok := candidates["items"].(map[string]any)
	if !ok {
		t.Fatalf("card_candidates.items = %#v", candidates["items"])
	}
	candidateProperties, ok := candidateItems["properties"].(map[string]any)
	if !ok {
		t.Fatalf("card_candidates.items.properties = %#v", candidateItems["properties"])
	}
	cardType, ok := candidateProperties["card_type"].(map[string]any)
	if !ok {
		t.Fatalf("card_type = %#v", candidateProperties["card_type"])
	}
	enum, ok := cardType["enum"].([]string)
	if !ok {
		t.Fatalf("card_type.enum = %#v, want []string", cardType["enum"])
	}
	for _, value := range enum {
		if value == "cloze" || value == "sentence_translation" {
			t.Errorf("card_type enum offers %q, which the server builds itself", value)
		}
	}
}

// sentence must stay out of the required list: a word lookup has no sentence to read,
// and requiring the field is an invitation to invent one.
func TestResponseSchemaSentenceIsOptional(t *testing.T) {
	schema := responseSchema(explain.DefaultFormat())
	if _, ok := schemaProperties(t, schema)["sentence"]; !ok {
		t.Fatal("schema has no sentence object")
	}
	required, ok := schema["required"].([]string)
	if !ok {
		t.Fatalf("required = %#v, want []string", schema["required"])
	}
	for _, field := range required {
		if field == "sentence" {
			t.Error("sentence is required; a word lookup would have to invent one")
		}
	}
}

func TestBuildPromptAppliesFormat(t *testing.T) {
	t.Run("style is included and fenced", func(t *testing.T) {
		prompt := buildPrompt("stale", explain.Format{PromptStyle: "예문은 백엔드 문맥으로", ExamplesCount: 3})
		if !strings.Contains(prompt, "예문은 백엔드 문맥으로") {
			t.Error("prompt does not carry the user's style guidance")
		}
		if !strings.Contains(prompt, `"""`) {
			t.Error("style guidance is not fenced off from the rules")
		}
		if !strings.Contains(prompt, "examples는 3개") {
			t.Errorf("prompt does not ask for 3 examples:\n%s", prompt)
		}
	})

	t.Run("no style leaves no empty fence", func(t *testing.T) {
		prompt := buildPrompt("stale", explain.DefaultFormat())
		if strings.Contains(prompt, `"""`) {
			t.Error("prompt has an empty style fence")
		}
	})

	t.Run("zero examples", func(t *testing.T) {
		prompt := buildPrompt("stale", explain.Format{ExamplesCount: 0})
		if !strings.Contains(prompt, "examples는 만들지 않는다") {
			t.Errorf("prompt does not suppress examples:\n%s", prompt)
		}
	})

	t.Run("out-of-range style is clamped before it reaches the model", func(t *testing.T) {
		long := strings.Repeat("한", explain.MaxPromptStyleRunes+500)
		prompt := buildPrompt("stale", explain.Format{PromptStyle: long})
		if strings.Contains(prompt, long) {
			t.Error("prompt carried the unclamped style text")
		}
	})
}

func schemaProperties(t *testing.T, schema map[string]any) map[string]any {
	t.Helper()
	properties, ok := schema["properties"].(map[string]any)
	if !ok {
		t.Fatalf("schema properties = %#v", schema["properties"])
	}
	return properties
}

// The schema is sent as JSON, so anything in it has to survive marshaling.
func TestResponseSchemaIsMarshalable(t *testing.T) {
	for _, count := range []int{0, 2, 5} {
		data, err := json.Marshal(responseSchema(explain.Format{ExamplesCount: count}))
		if err != nil {
			t.Fatalf("marshal schema (examples=%d): %v", count, err)
		}
		if !strings.Contains(string(data), "surface_in_text") {
			t.Fatalf("marshaled schema (examples=%d) lost surface_in_text: %s", count, data)
		}
	}
}
