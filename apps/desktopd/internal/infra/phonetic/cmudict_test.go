package phonetic

import (
	"strings"
	"testing"
)

// TestScanEntriesSkipsComments는 devterms.dict 포맷 파서가 빈 줄·전체 주석 줄·인라인 주석을
// 올바르게 처리하는지 검증한다(#30 보충 사전 편집 안전성).
func TestScanEntriesSkipsComments(t *testing.T) {
	input := strings.Join([]string{
		"# 전체 주석 줄",
		"",
		"   ",
		"mutex M Y UW T EH K S",
		"cache K AE SH  # 인라인 주석은 여기부터 무시",
		"# 또 다른 주석",
		"queue K Y UW",
	}, "\n")

	entries, err := scanEntries(strings.NewReader(input), 8)
	if err != nil {
		t.Fatalf("scanEntries() error = %v", err)
	}
	if len(entries) != 3 {
		t.Fatalf("scanEntries() got %d entries, want 3: %+v", len(entries), entries)
	}

	byWord := make(map[string][]string, len(entries))
	for _, e := range entries {
		byWord[e.word] = e.phones
	}

	if got := byWord["mutex"]; strings.Join(got, " ") != "M Y UW T EH K S" {
		t.Errorf("mutex phones = %v", got)
	}
	// 인라인 주석 토큰(#, 인라인, 주석은, ...)이 phone으로 새지 않아야 한다.
	if got := byWord["cache"]; strings.Join(got, " ") != "K AE SH" {
		t.Errorf("cache phones = %v, want just K AE SH (inline comment must be dropped)", got)
	}
	if got := byWord["queue"]; strings.Join(got, " ") != "K Y UW" {
		t.Errorf("queue phones = %v", got)
	}
}
