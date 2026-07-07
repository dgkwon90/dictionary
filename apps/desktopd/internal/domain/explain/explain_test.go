package explain

import (
	"errors"
	"testing"
)

func TestExplainResultValidateValidCases(t *testing.T) {
	for _, inputType := range []string{"word", "term", "phrase", "sentence", "error_message"} {
		t.Run(inputType, func(t *testing.T) {
			result := validExplainResult()
			result.InputType = inputType
			if err := result.Validate(); err != nil {
				t.Fatalf("Validate() error = %v", err)
			}
		})
	}
}

func TestExplainResultValidateInvalidCases(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*ExplainResult)
	}{
		{name: "empty input_type", mutate: func(r *ExplainResult) { r.InputType = "" }},
		{name: "bad input_type", mutate: func(r *ExplainResult) { r.InputType = "bogus" }},
		{name: "empty detected_language", mutate: func(r *ExplainResult) { r.DetectedLanguage = "" }},
		{name: "empty brief_ko", mutate: func(r *ExplainResult) { r.BriefKo = "" }},
		{name: "empty detailed_ko", mutate: func(r *ExplainResult) { r.DetailedKo = "" }},
		{name: "bad domain_category", mutate: func(r *ExplainResult) { r.DomainCategory = "bogus" }},
		{name: "difficulty low", mutate: func(r *ExplainResult) { r.Difficulty = -0.01 }},
		{name: "difficulty high", mutate: func(r *ExplainResult) { r.Difficulty = 1.01 }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := validExplainResult()
			tt.mutate(&result)
			err := result.Validate()
			if !errors.Is(err, ErrInvalidResult) {
				t.Fatalf("Validate() error = %v, want ErrInvalidResult", err)
			}
		})
	}
}

func validExplainResult() ExplainResult {
	return ExplainResult{
		InputType:        "word",
		DetectedLanguage: "en",
		BriefKo:          "brief",
		DetailedKo:       "detailed",
		DomainCategory:   "general",
		Difficulty:       0.5,
	}
}
