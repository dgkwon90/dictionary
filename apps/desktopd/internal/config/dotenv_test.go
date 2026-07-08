package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParseDotenvLine(t *testing.T) {
	cases := []struct {
		line    string
		wantKey string
		wantVal string
		wantOK  bool
	}{
		{`NEULSANG_GEMINI_API_KEY=abc123`, "NEULSANG_GEMINI_API_KEY", "abc123", true},
		{`export NEULSANG_ADDR=127.0.0.1:1`, "NEULSANG_ADDR", "127.0.0.1:1", true},
		{`  KEY = "quoted value" `, "KEY", "quoted value", true},
		{`KEY='single'`, "KEY", "single", true},
		{`# comment`, "", "", false},
		{``, "", "", false},
		{`novalueline`, "", "", false},
		{`=noname`, "", "", false},
	}
	for _, c := range cases {
		key, val, ok := parseDotenvLine(c.line)
		if key != c.wantKey || val != c.wantVal || ok != c.wantOK {
			t.Errorf("parseDotenvLine(%q) = (%q,%q,%v), want (%q,%q,%v)", c.line, key, val, ok, c.wantKey, c.wantVal, c.wantOK)
		}
	}
}

func TestApplyDotenvFileRealEnvWins(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".env")
	if err := os.WriteFile(path, []byte("NEULSANG_DOTENV_PRESET=fromfile\nNEULSANG_DOTENV_FRESH=fromfile\n"), 0o600); err != nil {
		t.Fatalf("write .env: %v", err)
	}
	// A variable already in the environment must not be overwritten.
	t.Setenv("NEULSANG_DOTENV_PRESET", "fromenv")
	// A variable only in the file must be applied. t.Setenv registers cleanup so the
	// process env is restored after the test even though we overwrite it below.
	t.Setenv("NEULSANG_DOTENV_FRESH", "")
	if err := os.Unsetenv("NEULSANG_DOTENV_FRESH"); err != nil {
		t.Fatalf("unset: %v", err)
	}

	if err := applyDotenvFile(path); err != nil {
		t.Fatalf("applyDotenvFile() error = %v", err)
	}

	if got := os.Getenv("NEULSANG_DOTENV_PRESET"); got != "fromenv" {
		t.Errorf("preset var = %q, want fromenv (real env must win)", got)
	}
	if got := os.Getenv("NEULSANG_DOTENV_FRESH"); got != "fromfile" {
		t.Errorf("fresh var = %q, want fromfile", got)
	}
}
