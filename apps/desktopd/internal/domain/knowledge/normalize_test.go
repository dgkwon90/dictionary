package knowledge

import (
	"strings"
	"testing"
)

func TestNormalizeKeyCollapsesEquivalentText(t *testing.T) {
	tests := []struct {
		name string
		a    string
		b    string
	}{
		{"case", "Stale", "stale"},
		{"surrounding whitespace", "  stale  ", "stale"},
		{"internal whitespace runs", "connection    pool", "connection pool"},
		{"newlines and tabs", "connection\tpool", "connection pool"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if NormalizeKey(test.a) != NormalizeKey(test.b) {
				t.Errorf("NormalizeKey(%q) = %q, want same as NormalizeKey(%q) = %q",
					test.a, NormalizeKey(test.a), test.b, NormalizeKey(test.b))
			}
		})
	}
}

func TestNormalizeKeyKeepsDistinctTextDistinct(t *testing.T) {
	if NormalizeKey("stale") == NormalizeKey("stalls") {
		t.Error("different words produced the same key")
	}
}

// Long sentences must stay distinct even when they share a long prefix — that is the
// failure mode an AI-truncated key would have introduced.
func TestNormalizeKeyHashesLongTextWithoutMerging(t *testing.T) {
	prefix := strings.Repeat("the cache entry went stale ", 6)
	a := prefix + "after five minutes."
	b := prefix + "after ten minutes."

	keyA, keyB := NormalizeKey(a), NormalizeKey(b)
	if !strings.HasPrefix(keyA, hashedKeyPrefix) {
		t.Fatalf("long text key = %q, want it to be hashed", keyA)
	}
	if len(keyA) > maxInlineKeyLength {
		t.Errorf("hashed key length = %d, want <= %d", len(keyA), maxInlineKeyLength)
	}
	if keyA == keyB {
		t.Error("two sentences sharing a long prefix produced the same key")
	}
}

func TestNormalizeKeyEmpty(t *testing.T) {
	for _, text := range []string{"", "   ", "\t\n"} {
		if got := NormalizeKey(text); got != "" {
			t.Errorf("NormalizeKey(%q) = %q, want empty", text, got)
		}
	}
}
