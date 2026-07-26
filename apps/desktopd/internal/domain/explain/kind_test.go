package explain

import "testing"

func TestLearnKind(t *testing.T) {
	tests := []struct {
		name      string
		inputType string
		text      string
		want      string
	}{
		{"single word", "word", "stale", LearnKindWord},
		{"technical term", "term", "connection pool", LearnKindWord},
		{"three-word term stays a word", "term", "eventual consistency guarantee", LearnKindWord},
		{"short phrase", "phrase", "opt in", LearnKindWord},
		{"explicit sentence", "sentence", "The cache entry went stale after 5 minutes.", LearnKindSentence},
		{"error message", "error_message", "panic: runtime error: index out of range", LearnKindSentence},

		// The override that matters: the provider labels a full sentence as a phrase.
		// Without this, the sentence never reaches word selection and can never become
		// a learning item — the failure is silent, which is why it is guarded here.
		{"long text mislabeled as phrase", "phrase", "The cache entry went stale after 5 minutes.", LearnKindSentence},
		{"long text mislabeled as word", "word", "we should probably rate limit this endpoint", LearnKindSentence},

		{"unknown classification falls back to length", "", "stale", LearnKindWord},
		{"unknown classification on long text", "garbage", "the quick brown fox jumps", LearnKindSentence},
		{"empty text", "word", "", LearnKindWord},
		{"extra whitespace does not inflate the count", "term", "  connection   pool  ", LearnKindWord},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := LearnKind(test.inputType, test.text); got != test.want {
				t.Errorf("LearnKind(%q, %q) = %q, want %q", test.inputType, test.text, got, test.want)
			}
		})
	}
}
