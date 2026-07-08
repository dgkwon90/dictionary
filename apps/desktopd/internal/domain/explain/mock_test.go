package explain

import (
	"context"
	"encoding/json"
	"testing"
)

func TestMockExplainerAlwaysReturnsValidResult(t *testing.T) {
	tests := []string{
		"stale",
		"This is stale.",
		"",
		"stale data",
		"  nonspace  ",
	}
	explainer := NewMockExplainer()
	for _, text := range tests {
		t.Run(text, func(t *testing.T) {
			result, rawResponseJSON, err := explainer.Explain(context.Background(), text)
			if err != nil {
				t.Fatalf("Explain() error = %v", err)
			}
			if err := result.Validate(); err != nil {
				t.Fatalf("Validate() error = %v", err)
			}
			var decoded ExplainResult
			if err := json.Unmarshal([]byte(rawResponseJSON), &decoded); err != nil {
				t.Fatalf("rawResponseJSON is not ExplainResult JSON: %v", err)
			}
			if decoded.BriefKo != result.BriefKo {
				t.Fatalf("rawResponseJSON brief_ko = %q, want %q", decoded.BriefKo, result.BriefKo)
			}
		})
	}
}
