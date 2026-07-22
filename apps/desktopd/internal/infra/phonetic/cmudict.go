package phonetic

import (
	"bufio"
	"bytes"
	"compress/gzip"
	_ "embed"
	"fmt"
	"io"
	"strings"
	"sync"
)

//go:embed data/cmudict.dict.gz
var cmuDictGzip []byte

// 개발/CS 용어 보충 발음사전(#30) — CMUdict에 없는 현대 기술용어(mutex·cardinality·
// idempotent 등)를 오프라인 로컬 매칭에 포함시킨다. CMU 뒤에 append돼 매칭 후보에 합류.
//
//go:embed data/devterms.dict
var devTermsData []byte

type entry struct {
	word   string
	phones []string
}

var (
	cmuOnce    sync.Once
	cmuEntries []entry
	cmuErr     error
)

func loadEntries() ([]entry, error) {
	cmuOnce.Do(func() {
		cmuEntries, cmuErr = parseAll()
	})
	if cmuErr != nil {
		return nil, cmuErr
	}
	return cmuEntries, nil
}

// parseAll은 gzip된 CMUdict를 읽고 그 뒤에 보충 devterms.dict를 이어붙인다.
func parseAll() ([]entry, error) {
	reader, err := gzip.NewReader(bytes.NewReader(cmuDictGzip))
	if err != nil {
		return nil, fmt.Errorf("phonetic: open cmudict: %w", err)
	}

	entries, scanErr := scanEntries(reader, 135_000)
	closeErr := reader.Close()
	if scanErr != nil {
		return nil, fmt.Errorf("phonetic: scan cmudict: %w", scanErr)
	}
	if closeErr != nil {
		return nil, fmt.Errorf("phonetic: close cmudict: %w", closeErr)
	}

	devEntries, err := scanEntries(bytes.NewReader(devTermsData), 128)
	if err != nil {
		return nil, fmt.Errorf("phonetic: scan devterms: %w", err)
	}
	return append(entries, devEntries...), nil
}

// scanEntries는 CMUdict 포맷(`word PH PH ...`)을 파싱한다. '#'로 시작하는 주석 줄과
// 필드가 2개 미만인 줄은 건너뛴다(devterms.dict의 주석/빈 줄 처리).
func scanEntries(r io.Reader, capacity int) ([]entry, error) {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 1024), 1024*1024)

	entries := make([]entry, 0, capacity)
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) < 2 || strings.HasPrefix(fields[0], "#") {
			continue
		}

		word := stripVariantSuffix(strings.ToLower(fields[0]))
		if word == "" {
			continue
		}

		phones := make([]string, 0, len(fields)-1)
		for _, phone := range fields[1:] {
			// 인라인 주석(`word PH PH  # 메모`)은 여기서부터 무시한다.
			if strings.HasPrefix(phone, "#") {
				break
			}
			stripped := stripStressDigit(phone)
			if stripped != "" {
				phones = append(phones, stripped)
			}
		}
		if len(phones) == 0 {
			continue
		}

		entries = append(entries, entry{word: word, phones: phones})
	}

	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return entries, nil
}

func stripVariantSuffix(word string) string {
	if !strings.HasSuffix(word, ")") {
		return word
	}

	open := strings.LastIndexByte(word, '(')
	if open < 0 || open == len(word)-2 {
		return word
	}
	for _, r := range word[open+1 : len(word)-1] {
		if r < '0' || r > '9' {
			return word
		}
	}
	return word[:open]
}

func stripStressDigit(phone string) string {
	if phone == "" {
		return ""
	}
	last := phone[len(phone)-1]
	if last >= '0' && last <= '9' {
		return phone[:len(phone)-1]
	}
	return phone
}
