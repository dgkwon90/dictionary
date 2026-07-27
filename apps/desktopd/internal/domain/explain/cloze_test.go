package explain

import (
	"strings"
	"testing"
)

func TestFindSpan(t *testing.T) {
	tests := []struct {
		name      string
		text      string
		surface   string
		wantStart int
		wantEnd   int
		wantOK    bool
	}{
		{"exact", "the cache is stale", "stale", 13, 18, true},
		{"case insensitive", "Stale cache", "stale", 0, 5, true},
		{"first occurrence wins", "stale is stale", "stale", 0, 5, true},
		{"surrounding whitespace ignored", "the cache is stale", "  stale ", 13, 18, true},
		// Offsets are counted in runes, not bytes: the frontend highlights by character
		// and the cloze builder slices by the same index.
		{"rune offsets", "캐시가 stale 상태다", "stale", 4, 9, true},
		{"absent", "the cache is fresh", "stale", 0, 0, false},
		{"longer than text", "run", "running", 0, 0, false},
		{"empty surface", "the cache is stale", "", 0, 0, false},
		{"empty text", "", "stale", 0, 0, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			start, end, ok := FindSpan(tt.text, tt.surface)
			if ok != tt.wantOK || start != tt.wantStart || end != tt.wantEnd {
				t.Fatalf("FindSpan(%q, %q) = (%d, %d, %t), want (%d, %d, %t)",
					tt.text, tt.surface, start, end, ok, tt.wantStart, tt.wantEnd, tt.wantOK)
			}
		})
	}
}

func TestBuildCloze(t *testing.T) {
	tests := []struct {
		name         string
		text         string
		surface      string
		wantQuestion string
		wantAnswer   string
		wantOK       bool
	}{
		{
			name:         "blanks the word out of the original sentence",
			text:         "The cache became stale after deploy.",
			surface:      "stale",
			wantQuestion: "The cache became ____ after deploy.",
			wantAnswer:   "stale",
			wantOK:       true,
		},
		{
			// The answer is taken from the text, not from the caller's surface form, so
			// the user is graded against what they actually read.
			name:         "answer keeps the text's own casing",
			text:         "Stale data was served.",
			surface:      "stale",
			wantQuestion: "____ data was served.",
			wantAnswer:   "Stale",
			wantOK:       true,
		},
		{
			name:         "inflected form",
			text:         "The job is running in the background.",
			surface:      "running",
			wantQuestion: "The job is ____ in the background.",
			wantAnswer:   "running",
			wantOK:       true,
		},
		{
			name:    "word not in the text produces no card",
			text:    "The cache became stale.",
			surface: "idempotent",
			wantOK:  false,
		},
		{
			// A blank covering everything asks nothing.
			name:    "whole text",
			text:    "stale",
			surface: "stale",
			wantOK:  false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cloze, ok := BuildCloze(tt.text, tt.surface)
			if ok != tt.wantOK {
				t.Fatalf("BuildCloze(%q, %q) ok = %t, want %t", tt.text, tt.surface, ok, tt.wantOK)
			}
			if !ok {
				return
			}
			if cloze.Question != tt.wantQuestion {
				t.Errorf("question = %q, want %q", cloze.Question, tt.wantQuestion)
			}
			if cloze.Answer != tt.wantAnswer {
				t.Errorf("answer = %q, want %q", cloze.Answer, tt.wantAnswer)
			}
			// The property the whole server-side design exists to guarantee: the card is
			// the captured text with one slice of it hidden. Putting the answer back into
			// the blank must reproduce the original exactly, which nothing invented for
			// the card can do.
			if restored := strings.Replace(cloze.Question, ClozeBlank, cloze.Answer, 1); restored != tt.text {
				t.Errorf("answer restored into the blank = %q, want the captured text %q", restored, tt.text)
			}
		})
	}
}

func TestSubItemLocatorForms(t *testing.T) {
	tests := []struct {
		name string
		item SubItem
		want []string
	}{
		{
			name: "in-text form is tried first",
			item: SubItem{SurfaceText: "run", SurfaceInText: "running"},
			want: []string{"running", "run"},
		},
		{
			name: "identical forms are not repeated",
			item: SubItem{SurfaceText: "stale", SurfaceInText: "stale"},
			want: []string{"stale"},
		},
		{
			name: "missing in-text form falls back to the dictionary form",
			item: SubItem{SurfaceText: "stale"},
			want: []string{"stale"},
		},
		{
			name: "nothing to look for",
			item: SubItem{},
			want: []string{},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.item.LocatorForms()
			if len(got) != len(tt.want) {
				t.Fatalf("LocatorForms() = %q, want %q", got, tt.want)
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Fatalf("LocatorForms() = %q, want %q", got, tt.want)
				}
			}
		})
	}
}
