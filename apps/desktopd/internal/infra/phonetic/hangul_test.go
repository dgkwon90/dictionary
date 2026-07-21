package phonetic

import (
	"reflect"
	"testing"
)

func TestHangulToARPAbetStale(t *testing.T) {
	got := hangulToARPAbet("스테일")
	want := []string{"S", "T", "EH", "IY", "L"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("hangulToARPAbet() = %#v, want %#v", got, want)
	}
}

func TestHangulToARPAbetMutex(t *testing.T) {
	got := hangulToARPAbet("뮤텍스")
	wantCore := []string{"M", "Y", "UW", "T", "EH", "K", "S"}
	if !containsSubsequence(got, wantCore) {
		t.Fatalf("hangulToARPAbet() = %#v, want core subsequence %#v", got, wantCore)
	}
}

func TestHangulToARPAbetIgnoresNonHangul(t *testing.T) {
	got := hangulToARPAbet("abc 스테일 xyz")
	want := []string{"S", "T", "EH", "IY", "L"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("hangulToARPAbet() = %#v, want %#v", got, want)
	}
}

func containsSubsequence(got, want []string) bool {
	if len(want) == 0 {
		return true
	}
	next := 0
	for _, phone := range got {
		if phone == want[next] {
			next++
			if next == len(want) {
				return true
			}
		}
	}
	return false
}
