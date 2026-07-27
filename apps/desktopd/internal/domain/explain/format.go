package explain

import "strings"

// Format is how the user wants explanations written. It is passed into the prompt and
// the response schema rather than baked into either, so "설명을 내 취향대로" is a
// setting instead of a code change.
//
// What it deliberately does not carry is the shape of the JSON itself. The response
// schema drives struct parsing, which drives knowledge extraction; letting the user
// rename a field would fail every save and leave captures permanently failed with no
// way for them to know what they broke. Style is safe to hand over, structure is not.
type Format struct {
	// PromptStyle is free-text guidance appended to the prompt ("짧게", "예문은
	// 백엔드 문맥으로" 등). Empty means no extra guidance.
	PromptStyle string
	// ExamplesCount is how many example sentences to ask for. Zero is a real answer
	// — some users want none — so it is not treated as "unset".
	ExamplesCount int
}

const (
	// MaxPromptStyleRunes bounds the user's free text. Counted in runes because the
	// text is Korean; a byte limit would cut a 2000-character request to ~660.
	MaxPromptStyleRunes = 2000
	// MaxExamplesCount caps what the schema will ask for. Beyond a handful the model
	// starts padding with near-duplicates and the response gets slow for no gain.
	MaxExamplesCount = 5
	// DefaultExamplesCount matches what the fixed prompt produced before this was
	// configurable, so an existing user sees no change until they ask for one.
	DefaultExamplesCount = 2
)

// DefaultFormat is what a lookup uses when the user has not set anything.
func DefaultFormat() Format {
	return Format{ExamplesCount: DefaultExamplesCount}
}

// Normalized clamps a Format into the range the provider is asked to honor. It never
// returns an error: a preferences value that drifted out of range must not be able to
// fail a lookup, and clamping is what the user meant in every case.
func (f Format) Normalized() Format {
	f.PromptStyle = strings.TrimSpace(f.PromptStyle)
	if runes := []rune(f.PromptStyle); len(runes) > MaxPromptStyleRunes {
		f.PromptStyle = strings.TrimSpace(string(runes[:MaxPromptStyleRunes]))
	}
	if f.ExamplesCount < 0 {
		f.ExamplesCount = 0
	}
	if f.ExamplesCount > MaxExamplesCount {
		f.ExamplesCount = MaxExamplesCount
	}
	return f
}
