package explain

import "strings"

// ClozeBlank is what replaces the hidden word in a cloze question.
const ClozeBlank = "____"

// Cloze is a fill-in-the-blank card built from a sentence the user actually captured.
//
// The server builds these rather than asking the AI for them. A cloze card's whole
// value is that it quizzes the sentence the user really met, and a model asked to
// "make a fill-in-the-blank question" will sometimes return a tidier sentence it made
// up instead — which is unverifiable in the question text and would be indistinguishable
// from the real thing once it is sitting in a review card. Cutting the blank out of the
// original string makes that class of error structurally impossible, and costs nothing:
// the AI only has to say which form of the word appears (SubItem.SurfaceInText), and
// even that is a fallback for when the dictionary form is not in the text verbatim.
type Cloze struct {
	Question string
	Answer   string
}

// BuildCloze blanks out surface inside text. It reports false when the word cannot be
// located, or when it covers the whole text — blanking everything leaves a card that
// asks nothing.
func BuildCloze(text, surface string) (Cloze, bool) {
	start, end, ok := FindSpan(text, surface)
	if !ok {
		return Cloze{}, false
	}
	runes := []rune(text)
	if strings.TrimSpace(string(runes[:start])) == "" && strings.TrimSpace(string(runes[end:])) == "" {
		return Cloze{}, false
	}
	return Cloze{
		Question: string(runes[:start]) + ClozeBlank + string(runes[end:]),
		// Taken from the text, not from the caller's surface: the text's own casing is
		// the answer the user is expected to produce.
		Answer: string(runes[start:end]),
	}, true
}

// FindSpan locates a term inside a captured text so the UI can highlight it and a cloze
// card can blank it out. Offsets are in runes, not bytes, because the frontend indexes
// strings by character.
//
// The match is case-insensitive because the AI returns terms in dictionary form
// ("stale") while the sentence may capitalize them. Only the first occurrence is
// recorded; a user who means a later occurrence selects it explicitly, which stores its
// own offsets.
func FindSpan(text, surface string) (start, end int, ok bool) {
	surface = strings.TrimSpace(surface)
	if text == "" || surface == "" {
		return 0, 0, false
	}
	haystack := []rune(strings.ToLower(text))
	needle := []rune(strings.ToLower(surface))
	if len(needle) > len(haystack) {
		return 0, 0, false
	}
	for index := 0; index+len(needle) <= len(haystack); index++ {
		if string(haystack[index:index+len(needle)]) == string(needle) {
			return index, index + len(needle), true
		}
	}
	return 0, 0, false
}
