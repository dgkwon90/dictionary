package phonetic

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"neulsang/desktopd/internal/domain/suggest"
)

func TestMatcherKnownCases(t *testing.T) {
	matcher := NewMatcher()
	cases := []struct {
		query string
		want  string
	}{
		{query: "스테일", want: "stale"},
		{query: "뮤텍스", want: "mutex"},
		{query: "카디널리티", want: "cardinality"},
		{query: "이디엠포턴트", want: "idempotent"},
	}

	entries, err := loadEntries()
	if err != nil {
		t.Fatalf("loadEntries() error = %v", err)
	}
	words := entryWordSet(entries)

	present := 0
	passed := 0
	for _, tc := range cases {
		got, err := matcher.Suggest(context.Background(), tc.query)
		if err != nil {
			t.Fatalf("Suggest(%q) error = %v", tc.query, err)
		}
		t.Logf("%s -> %s", tc.query, formatCandidates(got))

		if !words[tc.want] {
			t.Logf("%q is not present in embedded CMU dictionary; it cannot appear as a local candidate", tc.want)
			continue
		}

		present++
		if containsCandidate(got, tc.want) {
			passed++
			continue
		}
		t.Logf("%s -> %q not in top %d", tc.query, tc.want, len(got))
	}

	wantPasses := 2
	if present < wantPasses {
		wantPasses = present
	}
	if passed < wantPasses {
		t.Fatalf("known-case hits = %d, want at least %d among %d present targets", passed, wantPasses, present)
	}
}

func TestMatcherLatinOnlyReturnsEmpty(t *testing.T) {
	got, err := NewMatcher().Suggest(context.Background(), "stale")
	if err != nil {
		t.Fatalf("Suggest() error = %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("Suggest() = %#v, want empty", got)
	}
}

func entryWordSet(entries []entry) map[string]bool {
	words := make(map[string]bool, len(entries))
	for _, candidate := range entries {
		words[candidate.word] = true
	}
	return words
}

func containsCandidate(candidates []suggest.Candidate, english string) bool {
	for _, candidate := range candidates {
		if candidate.English == english {
			return true
		}
	}
	return false
}

func formatCandidates(candidates []suggest.Candidate) string {
	if len(candidates) == 0 {
		return "(none)"
	}
	parts := make([]string, 0, len(candidates))
	for _, candidate := range candidates {
		parts = append(parts, fmt.Sprintf("%s=%.3f", candidate.English, candidate.Confidence))
	}
	return strings.Join(parts, ", ")
}
