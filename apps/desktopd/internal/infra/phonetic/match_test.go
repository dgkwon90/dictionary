package phonetic

import "testing"

func TestPhonemeScoreIdentical(t *testing.T) {
	score := phonemeScore([]string{"S", "T", "EH", "L"}, []string{"S", "T", "EH", "L"})
	if score != 1 {
		t.Fatalf("phonemeScore() = %f, want 1", score)
	}
}

func TestWeightedDistanceCostClasses(t *testing.T) {
	vowelSubstitution := weightedDistance([]string{"AA"}, []string{"AE"})
	consonantSubstitution := weightedDistance([]string{"S"}, []string{"T"})
	crossClassSubstitution := weightedDistance([]string{"AA"}, []string{"T"})

	if vowelSubstitution != 0.5 {
		t.Fatalf("vowel substitution cost = %f, want 0.5", vowelSubstitution)
	}
	if consonantSubstitution != 0.5 {
		t.Fatalf("consonant substitution cost = %f, want 0.5", consonantSubstitution)
	}
	if vowelSubstitution >= crossClassSubstitution {
		t.Fatalf("vowel substitution cost = %f, want less than cross-class cost %f", vowelSubstitution, crossClassSubstitution)
	}
}

func TestWeightedDistanceCheapVowelIndel(t *testing.T) {
	vowelDeletion := weightedDistance([]string{"AH"}, nil)
	consonantDeletion := weightedDistance([]string{"T"}, nil)
	if vowelDeletion >= consonantDeletion {
		t.Fatalf("vowel deletion cost = %f, want less than consonant deletion cost %f", vowelDeletion, consonantDeletion)
	}
}
