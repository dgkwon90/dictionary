// Package knowledge holds the rule for identifying a knowledge item: what counts as
// "the same word" or "the same sentence" no matter how it was typed.
//
// It used to own much more. Learner state lived here ("모름"/"알아요" wrote
// learner_items), and it served a capture's extracted items over HTTP. Both are gone:
// an item now enters the learning list through triage (a word's "학습할래요", a
// sentence's selection-complete) and leaves it through the learning domain, and the
// capture's items come from the search domain along with the user's selections —
// which is what the screen actually needs (ADR-0010).
//
// What remains is the piece all of those paths still depend on: key normalization. It
// keeps its own package rather than moving into search or learning because knowledge
// extraction (explain), the selection flow (search) and dedup on import (backup) must
// all compute the identical key. A second copy would split one word into two rows the
// first time the two spellings drifted.
package knowledge
