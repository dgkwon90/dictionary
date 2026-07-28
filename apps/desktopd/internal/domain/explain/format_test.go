package explain

import (
	"errors"
	"strings"
	"testing"
)

func TestFormatNormalized(t *testing.T) {
	tests := []struct {
		name              string
		input             Format
		wantStyle         string
		wantExamplesCount int
	}{
		{
			name:              "defaults pass through",
			input:             DefaultFormat(),
			wantStyle:         "",
			wantExamplesCount: DefaultExamplesCount,
		},
		{
			name:              "style is trimmed",
			input:             Format{PromptStyle: "  짧게 설명해줘\n ", ExamplesCount: 1},
			wantStyle:         "짧게 설명해줘",
			wantExamplesCount: 1,
		},
		{
			// Zero is a real answer ("예문 없음"), not an unset value to fill in.
			name:              "zero examples is honored",
			input:             Format{ExamplesCount: 0},
			wantStyle:         "",
			wantExamplesCount: 0,
		},
		{
			name:              "examples clamped up",
			input:             Format{ExamplesCount: -3},
			wantStyle:         "",
			wantExamplesCount: 0,
		},
		{
			name:              "examples clamped down",
			input:             Format{ExamplesCount: 99},
			wantStyle:         "",
			wantExamplesCount: MaxExamplesCount,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.input.Normalized()
			if got.PromptStyle != tt.wantStyle {
				t.Errorf("PromptStyle = %q, want %q", got.PromptStyle, tt.wantStyle)
			}
			if got.ExamplesCount != tt.wantExamplesCount {
				t.Errorf("ExamplesCount = %d, want %d", got.ExamplesCount, tt.wantExamplesCount)
			}
		})
	}
}

// The style limit is counted in runes because the text is Korean. A byte limit would
// silently cut a 2000-character request to about a third of that.
func TestFormatNormalizedTruncatesStyleByRunes(t *testing.T) {
	format := Format{PromptStyle: strings.Repeat("한", MaxPromptStyleRunes+50)}.Normalized()
	if got := len([]rune(format.PromptStyle)); got != MaxPromptStyleRunes {
		t.Fatalf("PromptStyle rune length = %d, want %d", got, MaxPromptStyleRunes)
	}
}

// Validate is the write boundary: what the user typed is refused, not repaired.
func TestFormatValidate(t *testing.T) {
	cases := []struct {
		name    string
		format  Format
		wantErr bool
	}{
		{"default", DefaultFormat(), false},
		{"no style, no examples", Format{}, false},
		{"max examples", Format{ExamplesCount: MaxExamplesCount}, false},
		{"style at the limit", Format{PromptStyle: strings.Repeat("가", MaxPromptStyleRunes)}, false},
		{"too many examples", Format{ExamplesCount: MaxExamplesCount + 1}, true},
		{"negative examples", Format{ExamplesCount: -1}, true},
		{"style over the limit", Format{PromptStyle: strings.Repeat("가", MaxPromptStyleRunes+1)}, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.format.Validate()
			if tc.wantErr {
				if !errors.Is(err, ErrInvalidFormat) {
					t.Fatalf("Validate() = %v, want ErrInvalidFormat", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("Validate() = %v, want nil", err)
			}
		})
	}
}

// The limit is in runes, so Korean guidance is measured the way the user counts it —
// a byte limit would cut a 2000-character request to about 660.
func TestFormatValidateCountsRunesNotBytes(t *testing.T) {
	style := strings.Repeat("한", MaxPromptStyleRunes)
	if len(style) <= MaxPromptStyleRunes {
		t.Fatalf("test text is not multi-byte: %d bytes for %d runes", len(style), MaxPromptStyleRunes)
	}
	if err := (Format{PromptStyle: style}).Validate(); err != nil {
		t.Fatalf("Validate() = %v, want a %d-rune style to be accepted", err, MaxPromptStyleRunes)
	}
}
