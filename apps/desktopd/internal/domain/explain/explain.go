package explain

import (
	"context"
	"errors"
	"fmt"
)

var ErrInvalidResult = errors.New("invalid explain result")

type ExplainResult struct {
	InputType        string    `json:"input_type"`
	DetectedLanguage string    `json:"detected_language"`
	BriefKo          string    `json:"brief_ko"`
	DetailedKo       string    `json:"detailed_ko"`
	PronunciationKo  string    `json:"pronunciation_ko"`
	DomainCategory   string    `json:"domain_category"`
	Difficulty       float64   `json:"difficulty"`
	Examples         []Example `json:"examples"`
	SubItems         []SubItem `json:"sub_items"`
	// Sentence carries the whole-input reading, and is nil for a word lookup. It is
	// soft: a sentence explanation that arrives without it still saves, falling back
	// to BriefKo for the sentence's meaning.
	Sentence *Sentence `json:"sentence"`
}

// Sentence is the input read as one unit, for when the thing being learned is the
// sentence rather than a word in it.
//
// Before this existed, a sentence's "what did this mean?" card was answered with
// BriefKo — the one-line summary written for the result screen, which is a description
// of the sentence ("배포 후 캐시가 오래됐다는 뜻") rather than the translation a review
// card should be checking against.
type Sentence struct {
	// TranslationKo is the natural Korean translation of the whole input.
	TranslationKo string `json:"translation_ko"`
	// StructureKo explains the grammar or nuance that makes the sentence hard, and is
	// what the user reads when they open the sentence again later.
	StructureKo string `json:"structure_ko"`
}

type Example struct {
	English string `json:"english"`
	Korean  string `json:"korean"`
	Note    string `json:"note"`
}

// SubItem is one extracted word/term. Its review card candidates are nested here
// (#22) so each candidate is structurally tied to the exact term it tests — the AI
// schema cannot express a cross-array reference reliably, and nesting removes that
// class of dangling/ambiguous mapping entirely.
type SubItem struct {
	SurfaceText string `json:"surface_text"`
	// NormalizedKey is the AI's suggestion only. Identity is decided by
	// knowledge.NormalizeKey on the server — a generated key cannot be trusted to
	// distinguish two long strings that share a prefix.
	NormalizedKey string `json:"normalized_key"`
	// SurfaceInText is the word exactly as it appears in the captured text, which is
	// often not SurfaceText: the AI returns dictionary forms ("run"), sentences contain
	// inflections ("running"). It is what lets the server locate the word and cut a
	// cloze blank out of the original sentence. Empty when the AI omits it, in which
	// case SurfaceText is tried instead.
	SurfaceInText   string  `json:"surface_in_text"`
	ItemType        string  `json:"item_type"`
	MeaningKo       string  `json:"meaning_ko"`
	PronunciationKo string  `json:"pronunciation_ko"`
	Importance      float64 `json:"importance"`
	// DescriptionKo is the longer explanation shown when the user picks this word
	// out of a sentence and asks what it means in that context.
	DescriptionKo  string                `json:"description_ko"`
	CardCandidates []ReviewCardCandidate `json:"card_candidates"`
}

type ReviewCardCandidate struct {
	CardType    string `json:"card_type"`
	Question    string `json:"question"`
	Answer      string `json:"answer"`
	Explanation string `json:"explanation"`
}

// LocatorForms returns the strings to try, in order, when looking for this item inside
// the captured text: the form the AI says appears there first, its dictionary form as a
// fallback.
func (s SubItem) LocatorForms() []string {
	forms := make([]string, 0, 2)
	if s.SurfaceInText != "" && s.SurfaceInText != s.SurfaceText {
		forms = append(forms, s.SurfaceInText)
	}
	if s.SurfaceText != "" {
		forms = append(forms, s.SurfaceText)
	}
	return forms
}

type Explainer interface {
	// Explain returns the parsed result and the provider's raw response body
	// (for mock, its own marshaled JSON) so callers can preserve raw_response_json.
	// format carries the user's output preferences; pass DefaultFormat() for none.
	Explain(ctx context.Context, text string, format Format) (result ExplainResult, rawResponseJSON string, err error)
}

func (r ExplainResult) Validate() error {
	if !validInputType(r.InputType) {
		return fmt.Errorf("%w: unsupported input_type %q", ErrInvalidResult, r.InputType)
	}
	if r.DetectedLanguage == "" {
		return fmt.Errorf("%w: detected_language is required", ErrInvalidResult)
	}
	if r.BriefKo == "" {
		return fmt.Errorf("%w: brief_ko is required", ErrInvalidResult)
	}
	if r.DetailedKo == "" {
		return fmt.Errorf("%w: detailed_ko is required", ErrInvalidResult)
	}
	if !validDomainCategory(r.DomainCategory) {
		return fmt.Errorf("%w: unsupported domain_category %q", ErrInvalidResult, r.DomainCategory)
	}
	if r.Difficulty < 0 || r.Difficulty > 1 {
		return fmt.Errorf("%w: difficulty must be between 0.0 and 1.0", ErrInvalidResult)
	}
	return nil
}

func validInputType(value string) bool {
	switch value {
	case "word", "term", "phrase", "sentence", "error_message":
		return true
	default:
		return false
	}
}

func validDomainCategory(value string) bool {
	switch value {
	case "backend", "frontend", "infra", "database", "network", "general":
		return true
	default:
		return false
	}
}
