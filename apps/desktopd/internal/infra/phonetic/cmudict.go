package phonetic

import (
	"bufio"
	"bytes"
	"compress/gzip"
	_ "embed"
	"fmt"
	"strings"
	"sync"
)

//go:embed data/cmudict.dict.gz
var cmuDictGzip []byte

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
		cmuEntries, cmuErr = parseCMUDict()
	})
	if cmuErr != nil {
		return nil, cmuErr
	}
	return cmuEntries, nil
}

func parseCMUDict() ([]entry, error) {
	reader, err := gzip.NewReader(bytes.NewReader(cmuDictGzip))
	if err != nil {
		return nil, fmt.Errorf("phonetic: open cmudict: %w", err)
	}

	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, 1024), 1024*1024)

	entries := make([]entry, 0, 135_000)
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) < 2 {
			continue
		}

		word := stripVariantSuffix(strings.ToLower(fields[0]))
		if word == "" {
			continue
		}

		phones := make([]string, 0, len(fields)-1)
		for _, phone := range fields[1:] {
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

	scanErr := scanner.Err()
	closeErr := reader.Close()
	if scanErr != nil {
		return nil, fmt.Errorf("phonetic: scan cmudict: %w", scanErr)
	}
	if closeErr != nil {
		return nil, fmt.Errorf("phonetic: close cmudict: %w", closeErr)
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
