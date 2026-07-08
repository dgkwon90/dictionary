package gemini

import "fmt"

func buildPrompt(text string) string {
	return fmt.Sprintf(`당신은 한국어 개발자를 위한 영어 학습 도우미입니다. 다음 규칙을 반드시 지키세요:
- 한국어로 설명한다
- 영어 철자는 유지한다
- 한글 발음을 제공한다
- 개발 문맥이면 개발 문맥을 우선한다
- 어려운 문법 설명보다 실무 이해를 우선한다
- JSON schema를 반드시 지킨다
- 모르면 모른다고 표시한다
- 과도한 설명을 피하고 짧고 명확하게 답한다
- difficulty와 각 sub_item의 importance는 반드시 0.0에서 1.0 사이의 소수로 답한다
- sub_items에는 학습할 핵심 영어 단어/용어만 넣는다. surface_text는 그 영어 철자만 쓰고(설명 문장 금지), normalized_key는 소문자 기본형, item_type은 word/term/phrase/sentence/error_message 중 하나로, meaning_ko와 pronunciation_ko(한글 발음)를 반드시 채운다. sub_items는 최소 1개 이상 만든다
- 각 sub_item마다 그 단어를 묻는 복습 카드 card_candidates를 1~3개 반드시 만든다(그 sub_item에 해당하는 카드만). card_type, question(한국어 질문), answer(한국어 정답)를 채운다

다음 표현을 설명하세요: %q`, text)
}

func responseSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"input_type": map[string]any{
				"type": "string",
				"enum": []string{"word", "term", "phrase", "sentence", "error_message"},
			},
			"detected_language": map[string]any{"type": "string"},
			"brief_ko":          map[string]any{"type": "string"},
			"detailed_ko":       map[string]any{"type": "string"},
			"pronunciation_ko":  map[string]any{"type": "string"},
			"domain_category": map[string]any{
				"type": "string",
				"enum": []string{"backend", "frontend", "infra", "database", "network", "general"},
			},
			"difficulty": map[string]any{"type": "number"},
			"examples": map[string]any{
				"type": "array",
				"items": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"english": map[string]any{"type": "string"},
						"korean":  map[string]any{"type": "string"},
						"note":    map[string]any{"type": "string"},
					},
					"required": []string{"english", "korean"},
				},
			},
			"sub_items": map[string]any{
				"type":     "array",
				"minItems": 1,
				"items": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"surface_text":   map[string]any{"type": "string"},
						"normalized_key": map[string]any{"type": "string"},
						"item_type": map[string]any{
							"type": "string",
							"enum": []string{"word", "term", "phrase", "sentence", "error_message"},
						},
						"meaning_ko":       map[string]any{"type": "string"},
						"pronunciation_ko": map[string]any{"type": "string"},
						"importance":       map[string]any{"type": "number"},
						// #22: card candidates nest here so each is tied to this term.
						"card_candidates": map[string]any{
							"type":     "array",
							"minItems": 1,
							"items": map[string]any{
								"type": "object",
								"properties": map[string]any{
									"card_type": map[string]any{
										"type": "string",
										"enum": []string{"meaning", "reverse", "cloze", "context", "sentence_translation"},
									},
									"question":    map[string]any{"type": "string"},
									"answer":      map[string]any{"type": "string"},
									"explanation": map[string]any{"type": "string"},
								},
								"required": []string{"card_type", "question", "answer"},
							},
						},
					},
					"required": []string{"surface_text", "normalized_key", "item_type", "meaning_ko", "card_candidates"},
				},
			},
		},
		"required": []string{"input_type", "detected_language", "brief_ko", "detailed_ko", "domain_category", "difficulty", "sub_items"},
	}
}
