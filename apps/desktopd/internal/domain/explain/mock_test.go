package explain

import (
	"context"
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
			result, err := explainer.Explain(context.Background(), text)
			if err != nil {
				t.Fatalf("Explain() error = %v", err)
			}
			if err := result.Validate(); err != nil {
				t.Fatalf("Validate() error = %v", err)
			}
		})
	}
}
