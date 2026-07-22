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

// freshEnv registers a cleanup-restored env var and clears it, so a test starts
// from "unset". t.Setenv records the original for restore; unsetting after keeps
// the process env clean during the test itself.
func freshEnv(t *testing.T, key string) {
	t.Helper()
	t.Setenv(key, "")
	if err := os.Unsetenv(key); err != nil {
		t.Fatalf("unset %s: %v", key, err)
	}
}

// TestLoadDotenvUserConfigFallback covers backlog #25: when the cwd has no repo
// .env in its ancestry (the installed/bundled app runs with cwd "/"), config is
// still loaded from <UserConfigDir>/neulsang/.env.
func TestLoadDotenvUserConfigFallback(t *testing.T) {
	// A temp cwd with no .env anywhere above it stands in for the bundle's cwd.
	t.Chdir(t.TempDir())
	// Point os.UserConfigDir() at a temp home so we control the fallback file.
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", "") // ensure darwin/linux both resolve under HOME
	if err := os.Unsetenv("XDG_CONFIG_HOME"); err != nil {
		t.Fatalf("unset XDG_CONFIG_HOME: %v", err)
	}

	path := userConfigDotenvPath()
	if path == "" {
		t.Fatal("userConfigDotenvPath() empty")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatalf("mkdir config dir: %v", err)
	}
	if err := os.WriteFile(path, []byte("NEULSANG_TEST_FALLBACK=fromuserconfig\n"), 0o600); err != nil {
		t.Fatalf("write user-config .env: %v", err)
	}
	freshEnv(t, "NEULSANG_TEST_FALLBACK")

	if err := LoadDotenv(); err != nil {
		t.Fatalf("LoadDotenv() error = %v", err)
	}
	if got := os.Getenv("NEULSANG_TEST_FALLBACK"); got != "fromuserconfig" {
		t.Errorf("fallback var = %q, want fromuserconfig (user-config .env must load when cwd has none)", got)
	}
}

// TestLoadDotenvRepoWinsOverUserConfig verifies the precedence: when both a repo
// .env (cwd walk-up) and the user-config .env define the same key, the repo one
// wins (dev config overrides the installed default).
func TestLoadDotenvRepoWinsOverUserConfig(t *testing.T) {
	cwd := t.TempDir()
	t.Chdir(cwd)
	if err := os.WriteFile(filepath.Join(cwd, ".env"), []byte("NEULSANG_TEST_PREC=fromrepo\n"), 0o600); err != nil {
		t.Fatalf("write repo .env: %v", err)
	}

	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", "")
	if err := os.Unsetenv("XDG_CONFIG_HOME"); err != nil {
		t.Fatalf("unset XDG_CONFIG_HOME: %v", err)
	}
	ucPath := userConfigDotenvPath()
	if err := os.MkdirAll(filepath.Dir(ucPath), 0o700); err != nil {
		t.Fatalf("mkdir config dir: %v", err)
	}
	if err := os.WriteFile(ucPath, []byte("NEULSANG_TEST_PREC=fromuserconfig\n"), 0o600); err != nil {
		t.Fatalf("write user-config .env: %v", err)
	}
	freshEnv(t, "NEULSANG_TEST_PREC")

	if err := LoadDotenv(); err != nil {
		t.Fatalf("LoadDotenv() error = %v", err)
	}
	if got := os.Getenv("NEULSANG_TEST_PREC"); got != "fromrepo" {
		t.Errorf("precedence var = %q, want fromrepo (repo .env must win over user-config)", got)
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
