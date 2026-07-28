package gemini

import (
	"fmt"
	"strings"

	"neulsang/desktopd/internal/domain/explain"
)

func buildPrompt(text string, format explain.Format) string {
	format = format.Normalized()

	var prompt strings.Builder
	prompt.WriteString(`당신은 한국어 개발자를 위한 영어 학습 도우미입니다. 다음 규칙을 반드시 지키세요:
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
- surface_in_text에는 그 단어가 입력 원문에 **실제로 적혀 있는 형태 그대로**를 옮겨 적는다(대소문자·굴절 포함). 예: 원문이 "The job is running"이면 surface_text는 "run", surface_in_text는 "running". 원문에 없는 단어라면 빈 문자열로 둔다
- description_ko에는 그 단어가 이 문맥에서 어떤 뜻으로 쓰였는지 두세 문장으로 설명한다
- 각 sub_item마다 그 단어의 뜻을 묻는 복습 카드 card_candidates를 1~3개 반드시 만든다(그 sub_item에 해당하는 카드만). card_type, question(한국어 질문), answer(한국어 정답)를 채운다. 빈칸 채우기 카드는 만들지 않는다 — 그건 앱이 원문에서 직접 만든다
- 입력이 문장이나 에러 메시지이면 sentence 객체를 채운다. translation_ko는 문장 전체의 자연스러운 한국어 번역이고(요약이나 설명이 아니라 번역), structure_ko는 이 문장을 어렵게 만드는 문법·뉘앙스를 짚는다. 입력이 단어나 용어이면 sentence는 비워 둔다`)

	switch format.ExamplesCount {
	case 0:
		prompt.WriteString("\n- examples는 만들지 않는다(빈 배열)")
	default:
		fmt.Fprintf(&prompt, "\n- examples는 %d개 만든다", format.ExamplesCount)
	}

	// The user's style guidance is fenced and labeled as a formatting request. It can
	// still steer tone and depth — that is the point — but it arrives clearly marked as
	// the user's words rather than as more rules from us, so an instruction hidden in
	// it ("ignore the schema") reads as text to weigh rather than a system directive.
	if format.PromptStyle != "" {
		fmt.Fprintf(&prompt, `

사용자가 요청한 설명 스타일입니다. 위 규칙과 JSON schema를 어기지 않는 선에서 최대한 반영하세요:
"""
%s
"""`, format.PromptStyle)
	}

	fmt.Fprintf(&prompt, "\n\n다음 표현을 설명하세요: %q", text)
	return prompt.String()
}

// buildSuggestPrompt asks the model to infer English dev-term candidates from a
// Korean phonetic spelling (backlog #21).
func buildSuggestPrompt(query string) string {
	return fmt.Sprintf(`당신은 한국어 개발자를 돕는 도구입니다. 사용자가 영어 철자를 몰라 한글 발음으로 입력한 개발/IT 용어를, 원래 영어 단어로 추론하세요. 규칙:
- 한글 발음에 해당할 법한 실제 영어 단어/용어 후보를 최대 3개, 가능성 높은 순으로 제시한다
- 반드시 실재하는 영어 단어만 제시한다(지어내지 않는다)
- english는 영어 철자, confidence는 0.0~1.0, gloss_ko는 짧은 한국어 뜻
- 개발 문맥을 우선한다(예: "뮤텍스"→mutex, "이디엠포턴트"→idempotent)
- 확신이 없으면 후보를 적게 제시한다

한글 발음: %q`, query)
}

func suggestResponseSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"candidates": map[string]any{
				"type": "array",
				"items": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"english":    map[string]any{"type": "string"},
						"confidence": map[string]any{"type": "number"},
						"gloss_ko":   map[string]any{"type": "string"},
					},
					"required": []string{"english"},
				},
			},
		},
		"required": []string{"candidates"},
	}
}

// ResponseSchema is the JSON schema a lookup asks the model to fill, exported so the
// Settings screen can show the user exactly what shape their explanations are requested
// in (D7). It is the same value the real request uses — a sample that could drift from
// the contract would be worse than none.
func ResponseSchema(format explain.Format) map[string]any {
	return responseSchema(format)
}

func responseSchema(format explain.Format) map[string]any {
	format = format.Normalized()

	properties := map[string]any{
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
		// sentence is not in the top-level required list: a word lookup has no sentence
		// to read, and forcing the field would make the model invent one.
		"sentence": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"translation_ko": map[string]any{"type": "string"},
				"structure_ko":   map[string]any{"type": "string"},
			},
			"required": []string{"translation_ko"},
		},
		"sub_items": map[string]any{
			"type":     "array",
			"minItems": 1,
			"items": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"surface_text":    map[string]any{"type": "string"},
					"surface_in_text": map[string]any{"type": "string"},
					"normalized_key":  map[string]any{"type": "string"},
					"item_type": map[string]any{
						"type": "string",
						"enum": []string{"word", "term", "phrase", "sentence", "error_message"},
					},
					"meaning_ko":       map[string]any{"type": "string"},
					"pronunciation_ko": map[string]any{"type": "string"},
					"description_ko":   map[string]any{"type": "string"},
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
									// cloze and sentence_translation are gone from the
									// enum: both are built by the server from the text
									// the user actually captured, so a model-written
									// version of either could only be an unverifiable
									// second source of truth.
									"enum": []string{"meaning", "reverse", "context"},
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
	}

	// Asking for zero examples by dropping the field entirely, rather than by saying
	// "maxItems: 0", keeps the model from filling an array it can see in the schema.
	if format.ExamplesCount > 0 {
		properties["examples"] = map[string]any{
			"type":     "array",
			"minItems": format.ExamplesCount,
			"maxItems": format.ExamplesCount,
			"items": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"english": map[string]any{"type": "string"},
					"korean":  map[string]any{"type": "string"},
					"note":    map[string]any{"type": "string"},
				},
				"required": []string{"english", "korean"},
			},
		}
	}

	return map[string]any{
		"type":       "object",
		"properties": properties,
		"required":   []string{"input_type", "detected_language", "brief_ko", "detailed_ko", "domain_category", "difficulty", "sub_items"},
	}
}
