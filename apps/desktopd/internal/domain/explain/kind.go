package explain

import "strings"

// LearnKindWord / LearnKindSentence mirror the capture domain's constants. They are
// repeated rather than imported so this package stays free of that dependency; the
// values are a storage contract (a CHECK constraint) and are covered by tests on
// both sides.
const (
	LearnKindWord     = "word"
	LearnKindSentence = "sentence"
)

// maxWordTokens is where "a term" stops and "a sentence" begins. Multi-word technical
// terms ("connection pool", "eventual consistency", "N+1 query") are common enough
// that treating any two- or three-word input as a sentence would push the user into
// the word-selection flow for what is really one thing to learn.
const maxWordTokens = 3

// LearnKind decides whether a capture is a word or a sentence — the branch the whole
// product hangs off. The AI's own input_type is the primary signal, but the server
// overrides it when the text is plainly too long to be a single term.
//
// The override only ever runs one way (word-ish → sentence) because the two errors
// are not symmetric. Misreading a sentence as a word skips word selection entirely,
// so the user can never record *why* they did not understand it and the sentence
// silently cannot become a learning item. Misreading a short phrase as a sentence
// just asks one extra question. We already know this provider mislabels fields under
// load (it returned out-of-range difficulty and empty item_type during the #6 rollout),
// so the cheap guard is worth having.
func LearnKind(inputType, text string) string {
	switch inputType {
	case "sentence", "error_message":
		return LearnKindSentence
	case "word", "term", "phrase":
		if tokenCount(text) > maxWordTokens {
			return LearnKindSentence
		}
		return LearnKindWord
	default:
		// Unknown or missing classification: fall back to length alone.
		if tokenCount(text) > maxWordTokens {
			return LearnKindSentence
		}
		return LearnKindWord
	}
}

func tokenCount(text string) int {
	return len(strings.Fields(text))
}
