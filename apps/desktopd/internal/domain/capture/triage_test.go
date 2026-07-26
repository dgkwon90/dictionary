package capture

import (
	"errors"
	"testing"
)

func TestNextTriageState(t *testing.T) {
	tests := []struct {
		name       string
		current    string
		learnKind  string
		transition Transition
		want       string
		wantErr    error
	}{
		// 단어: 결과를 보고 "학습할래요"를 누르면 바로 학습 목록으로.
		{"word learns directly", TriageUnseen, LearnKindWord, TransitionLearn, TriageLearning, nil},
		{"learning is idempotent", TriageLearning, LearnKindWord, TransitionLearn, TriageLearning, nil},

		// 문장: 모르는 단어를 고르기 전에는 학습 대상이 될 수 없다. 이게 제품의 핵심
		// 규칙이라 API 경계에서 막는다.
		{"sentence cannot skip selection", TriageUnseen, LearnKindSentence, TransitionLearn, "", ErrSelectionRequired},
		{"sentence opens for selection", TriageUnseen, LearnKindSentence, TransitionOpen, TriageNeedsSelection, nil},
		{"opening twice is a no-op", TriageNeedsSelection, LearnKindSentence, TransitionOpen, TriageNeedsSelection, nil},
		{"sentence learns after selection", TriageNeedsSelection, LearnKindSentence, TransitionLearn, TriageLearning, nil},

		// 단어는 고를 단어가 자기 자신뿐이라 선택 단계가 없다.
		{"word cannot open", TriageUnseen, LearnKindWord, TransitionOpen, "", ErrInvalidInput},

		// 이미 정리된 캡처를 뒤로 되돌리지 않는다(더블클릭·오래된 화면 방어).
		{"reopening a learning capture is a no-op", TriageLearning, LearnKindSentence, TransitionOpen, TriageLearning, nil},
		{"reopening a discarded capture is a no-op", TriageDiscarded, LearnKindSentence, TransitionOpen, TriageDiscarded, nil},
		{"discarded cannot become learning", TriageDiscarded, LearnKindWord, TransitionLearn, "", ErrInvalidInput},

		// 버리기는 어떤 상태에서도 막히면 안 된다.
		{"discard from unseen", TriageUnseen, LearnKindWord, TransitionDiscard, TriageDiscarded, nil},
		{"discard from needs_selection", TriageNeedsSelection, LearnKindSentence, TransitionDiscard, TriageDiscarded, nil},
		{"discard from learning", TriageLearning, LearnKindWord, TransitionDiscard, TriageDiscarded, nil},
		{"discard is idempotent", TriageDiscarded, LearnKindWord, TransitionDiscard, TriageDiscarded, nil},

		{"unknown current state", "bogus", LearnKindWord, TransitionLearn, "", ErrInvalidInput},
		{"unknown transition", TriageUnseen, LearnKindWord, Transition("explode"), "", ErrInvalidInput},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := NextTriageState(test.current, test.learnKind, test.transition)
			if test.wantErr != nil {
				if !errors.Is(err, test.wantErr) {
					t.Fatalf("error = %v, want %v", err, test.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error = %v", err)
			}
			if got != test.want {
				t.Errorf("state = %q, want %q", got, test.want)
			}
		})
	}
}

// A capture whose AI lookup has not finished yet has no learn_kind. Asking to learn
// it must fail cleanly rather than guessing a branch.
func TestNextTriageStateBeforeClassification(t *testing.T) {
	if _, err := NextTriageState(TriageUnseen, "", TransitionOpen); !errors.Is(err, ErrInvalidInput) {
		t.Errorf("open without learn_kind: error = %v, want ErrInvalidInput", err)
	}
	if _, err := NextTriageState(TriageUnseen, "", TransitionLearn); !errors.Is(err, ErrInvalidInput) {
		t.Errorf("learn without learn_kind: error = %v, want ErrInvalidInput", err)
	}
	// Discard must still work — a user can throw away a search that is still running.
	if got, err := NextTriageState(TriageUnseen, "", TransitionDiscard); err != nil || got != TriageDiscarded {
		t.Errorf("discard without learn_kind = %q, %v; want %q, nil", got, err, TriageDiscarded)
	}
}
