package config

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"
)

// LoadDotenv finds a .env file by starting at the current working directory and
// walking up to the filesystem root, then applies its KEY=VALUE lines to the process
// environment. Real environment variables always win (an existing var is never
// overwritten). It is a no-op when no .env exists. This is a local-development
// convenience only — secrets stay in the gitignored .env and are never committed.
func LoadDotenv() error {
	dir, err := os.Getwd()
	if err != nil {
		return err
	}
	for {
		path := filepath.Join(dir, ".env")
		if info, err := os.Stat(path); err == nil && !info.IsDir() {
			return applyDotenvFile(path)
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return nil // reached filesystem root without finding a .env
		}
		dir = parent
	}
}

func applyDotenvFile(path string) (resultErr error) {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer func() {
		if err := file.Close(); err != nil && resultErr == nil {
			resultErr = err
		}
	}()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		key, value, ok := parseDotenvLine(scanner.Text())
		if !ok {
			continue
		}
		if _, exists := os.LookupEnv(key); exists {
			continue
		}
		if err := os.Setenv(key, value); err != nil {
			return err
		}
	}
	return scanner.Err()
}

// parseDotenvLine parses one `KEY=VALUE` line (optionally `export KEY=VALUE`),
// ignoring blanks and # comments and stripping matching surrounding quotes.
func parseDotenvLine(line string) (key, value string, ok bool) {
	line = strings.TrimSpace(line)
	if line == "" || strings.HasPrefix(line, "#") {
		return "", "", false
	}
	line = strings.TrimPrefix(line, "export ")
	eq := strings.IndexByte(line, '=')
	if eq <= 0 {
		return "", "", false
	}
	key = strings.TrimSpace(line[:eq])
	value = trimMatchingQuotes(strings.TrimSpace(line[eq+1:]))
	return key, value, key != ""
}

func trimMatchingQuotes(s string) string {
	if len(s) >= 2 {
		first, last := s[0], s[len(s)-1]
		if (first == '"' && last == '"') || (first == '\'' && last == '\'') {
			return s[1 : len(s)-1]
		}
	}
	return s
}
