package config

import (
	"os"
	"testing"

	"github.com/zalando/go-keyring"
)

func TestLoadGeminiKeyFromKeychainLoadsWhenEnvEmpty(t *testing.T) {
	keyring.MockInit()
	t.Setenv("NEULSANG_GEMINI_API_KEY", "")
	if err := keyring.Set(keychainService, keychainAccountGeminiKey, "from-keychain"); err != nil {
		t.Fatalf("keyring.Set() error = %v", err)
	}

	if err := LoadGeminiKeyFromKeychain(); err != nil {
		t.Fatalf("LoadGeminiKeyFromKeychain() error = %v", err)
	}

	if got := os.Getenv("NEULSANG_GEMINI_API_KEY"); got != "from-keychain" {
		t.Errorf("NEULSANG_GEMINI_API_KEY = %q, want from-keychain", got)
	}
}

func TestLoadGeminiKeyFromKeychainKeepsExistingEnv(t *testing.T) {
	keyring.MockInit()
	t.Setenv("NEULSANG_GEMINI_API_KEY", "from-env")
	if err := keyring.Set(keychainService, keychainAccountGeminiKey, "from-keychain"); err != nil {
		t.Fatalf("keyring.Set() error = %v", err)
	}

	if err := LoadGeminiKeyFromKeychain(); err != nil {
		t.Fatalf("LoadGeminiKeyFromKeychain() error = %v", err)
	}

	if got := os.Getenv("NEULSANG_GEMINI_API_KEY"); got != "from-env" {
		t.Errorf("NEULSANG_GEMINI_API_KEY = %q, want from-env", got)
	}
}

func TestLoadGeminiKeyFromKeychainIgnoresMissingKey(t *testing.T) {
	keyring.MockInit()
	t.Setenv("NEULSANG_GEMINI_API_KEY", "")

	if err := LoadGeminiKeyFromKeychain(); err != nil {
		t.Fatalf("LoadGeminiKeyFromKeychain() error = %v", err)
	}

	if got := os.Getenv("NEULSANG_GEMINI_API_KEY"); got != "" {
		t.Errorf("NEULSANG_GEMINI_API_KEY = %q, want empty", got)
	}
}
