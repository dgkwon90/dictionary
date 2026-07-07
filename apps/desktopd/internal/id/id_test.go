package id

import (
	"regexp"
	"testing"
)

func TestNew(t *testing.T) {
	uuidPattern := regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)
	seen := make(map[string]struct{}, 1000)
	for range 1000 {
		got := New()
		if !uuidPattern.MatchString(got) {
			t.Fatalf("New() = %q, want UUIDv4", got)
		}
		if got[14] != '4' {
			t.Fatalf("version nibble = %q, want 4", got[14])
		}
		switch got[19] {
		case '8', '9', 'a', 'b':
		default:
			t.Fatalf("variant nibble = %q, want 8, 9, a, or b", got[19])
		}
		if _, ok := seen[got]; ok {
			t.Fatalf("duplicate UUID generated: %s", got)
		}
		seen[got] = struct{}{}
	}
}
