package explain

import (
	"context"
	"errors"
	"fmt"
)

var ErrInvalidResult = errors.New("invalid explain result")

type ExplainResult struct {
	InputType            string                `json:"input_type"`
	DetectedLanguage     string                `json:"detected_language"`
	BriefKo              string                `json:"brief_ko"`
	DetailedKo           string                `json:"detailed_ko"`
	PronunciationKo      string                `json:"pronunciation_ko"`
	DomainCategory       string                `json:"domain_category"`
	Difficulty           float64               `json:"difficulty"`
	Examples             []Example             `json:"examples"`
	SubItems             []SubItem             `json:"sub_items"`
	ReviewCardCandidates []ReviewCardCandidate `json:"review_card_candidates"`
}

type Example struct {
	English string `json:"english"`
	Korean  string `json:"korean"`
	Note    string `json:"note"`
}

type SubItem struct {
	SurfaceText     string  `json:"surface_text"`
	NormalizedKey   string  `json:"normalized_key"`
	ItemType        string  `json:"item_type"`
	MeaningKo       string  `json:"meaning_ko"`
	PronunciationKo string  `json:"pronunciation_ko"`
	Importance      float64 `json:"importance"`
}

type ReviewCardCandidate struct {
	CardType    string `json:"card_type"`
	Question    string `json:"question"`
	Answer      string `json:"answer"`
	Explanation string `json:"explanation"`
}

type Explainer interface {
	Explain(ctx context.Context, text string) (ExplainResult, error)
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
