package capture

import "fmt"

// Triage states describe what the *user* has decided about a search result. They are
// deliberately independent of how the AI lookup is going (lookup_jobs.status), which
// is derived at read time instead of being copied here.
//
//	unseen ──[word: 학습할래요]────────▶ learning
//	unseen ──[sentence: 결과 열람]─────▶ needs_selection
//	unseen ──[삭제]───────────────────▶ discarded
//	needs_selection ──[완료]──────────▶ learning
//	needs_selection ──[삭제]──────────▶ discarded
//	learning ──[삭제]─────────────────▶ discarded
//
// "미확인" (the default list) is unseen + needs_selection: a sentence whose unknown
// words the user has not picked yet is still unresolved, because we do not yet know
// *why* they did not understand it.
const (
	TriageUnseen         = "unseen"
	TriageNeedsSelection = "needs_selection"
	TriageLearning       = "learning"
	TriageDiscarded      = "discarded"
)

// LearnKind values. This is the server's own two-way classification, not the AI's
// finer-grained input_type — see LearnKind in the explain domain.
const (
	LearnKindWord     = "word"
	LearnKindSentence = "sentence"
)

func ValidTriageState(value string) bool {
	switch value {
	case TriageUnseen, TriageNeedsSelection, TriageLearning, TriageDiscarded:
		return true
	default:
		return false
	}
}

func ValidLearnKind(value string) bool {
	return value == LearnKindWord || value == LearnKindSentence
}

// Transition is a user action on a capture.
type Transition string

const (
	// TransitionOpen marks a sentence as awaiting word selection. Idempotent.
	TransitionOpen Transition = "open"
	// TransitionLearn commits the capture to the learning list.
	TransitionLearn Transition = "learn"
	// TransitionDiscard throws the search away.
	TransitionDiscard Transition = "discard"
)

// ErrSelectionRequired is returned when a sentence is asked to become a learning
// item before its unknown words have been picked. This is the rule that keeps the
// product honest: for a sentence, "I understood why I did not know it" *is* the act
// of choosing the words.
var ErrSelectionRequired = fmt.Errorf("%w: 모르는 단어를 먼저 선택하세요", ErrInvalidInput)

// NextTriageState applies a transition, returning the resulting state. It is a pure
// function so the rules can be tested exhaustively without a database.
func NextTriageState(current, learnKind string, transition Transition) (string, error) {
	if !ValidTriageState(current) {
		return "", fmt.Errorf("%w: unknown triage state %q", ErrInvalidInput, current)
	}
	switch transition {
	case TransitionDiscard:
		// Always allowed: throwing a search away must never be blocked by its state.
		return TriageDiscarded, nil

	case TransitionOpen:
		if learnKind != LearnKindSentence {
			return "", fmt.Errorf("%w: only sentences need word selection", ErrInvalidInput)
		}
		switch current {
		case TriageUnseen, TriageNeedsSelection:
			return TriageNeedsSelection, nil
		default:
			// Already resolved: opening it again is a no-op rather than an error, so a
			// double click or a stale UI cannot move a finished capture backwards.
			return current, nil
		}

	case TransitionLearn:
		switch current {
		case TriageLearning:
			return TriageLearning, nil
		case TriageUnseen:
			// No classification yet means the lookup has not produced a result, so the
			// user cannot have read one — refuse rather than guess a branch.
			if !ValidLearnKind(learnKind) {
				return "", fmt.Errorf("%w: capture has not been classified yet", ErrInvalidInput)
			}
			if learnKind == LearnKindSentence {
				return "", ErrSelectionRequired
			}
			return TriageLearning, nil
		case TriageNeedsSelection:
			// Reached only through the selection-complete path, which checks that at
			// least one word was picked before calling this.
			return TriageLearning, nil
		default:
			return "", fmt.Errorf("%w: cannot learn a %s capture", ErrInvalidInput, current)
		}
	}
	return "", fmt.Errorf("%w: unknown transition %q", ErrInvalidInput, transition)
}
