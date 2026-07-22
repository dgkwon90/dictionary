package config

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"
)

// LoadDotenv finds .env config and applies its KEY=VALUE lines to the process
// environment. It searches two locations, applying each that exists:
//
//  1. cwd walk-up to the filesystem root — the gitignored repo .env, for local dev.
//  2. the OS user-config directory (<UserConfigDir>/neulsang/.env) — for the
//     installed/bundled app, whose cwd is "/" and so can never reach the repo .env
//     (backlog #25: running the bundle via open/Finder otherwise fell back to mock).
//
// Precedence: real environment variables always win (an existing var is never
// overwritten), then the repo .env, then the user-config .env — because
// applyDotenvFile never overwrites an already-set var and the repo path is applied
// first. It is a no-op when neither file exists. Secrets stay in these gitignored,
// user-owned files and are never committed, stored in the DB, or logged.
func LoadDotenv() error {
	for _, path := range dotenvCandidatePaths() {
		if path == "" {
			continue
		}
		if info, err := os.Stat(path); err != nil || info.IsDir() {
			continue
		}
		if err := applyDotenvFile(path); err != nil {
			return err
		}
	}
	return nil
}

// dotenvCandidatePaths returns the .env locations to load, in precedence order
// (earlier wins for overlapping keys). An empty string means "no candidate".
func dotenvCandidatePaths() []string {
	return []string{repoDotenvPath(), userConfigDotenvPath()}
}

// repoDotenvPath walks up from the cwd looking for a .env (local-dev convenience).
// Returns "" if none is found or the cwd is unavailable.
func repoDotenvPath() string {
	dir, err := os.Getwd()
	if err != nil {
		return ""
	}
	for {
		path := filepath.Join(dir, ".env")
		if info, err := os.Stat(path); err == nil && !info.IsDir() {
			return path
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "" // reached filesystem root without finding a .env
		}
		dir = parent
	}
}

// userConfigDotenvPath returns the installed app's cwd-independent config file,
// alongside the default SQLite DB under <UserConfigDir>/neulsang/. Returns "" if
// the OS user-config directory can't be resolved.
func userConfigDotenvPath() string {
	configDir, err := os.UserConfigDir()
	if err != nil {
		return ""
	}
	return filepath.Join(configDir, "neulsang", ".env")
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
