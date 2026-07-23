package config

import (
	"log/slog"
	"os"
	"path/filepath"
	"testing"
)

func TestLoadDefaults(t *testing.T) {
	t.Setenv("NEULSANG_ADDR", "")
	t.Setenv("NEULSANG_DB_PATH", "")
	t.Setenv("NEULSANG_LOG_LEVEL", "")
	t.Setenv("NEULSANG_AI_PROVIDER", "")
	t.Setenv("NEULSANG_GEMINI_API_KEY", "")
	t.Setenv("NEULSANG_GEMINI_MODEL", "")
	t.Setenv("NEULSANG_SYNC_URL", "")
	t.Setenv("NEULSANG_API_TOKEN", "")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if cfg.Addr != defaultAddr {
		t.Errorf("Addr = %q, want %q", cfg.Addr, defaultAddr)
	}
	configDir, err := os.UserConfigDir()
	if err != nil {
		t.Fatalf("UserConfigDir() error = %v", err)
	}
	wantDBPath := filepath.Join(configDir, "neulsang", "neulsang.db")
	if cfg.DBPath != wantDBPath {
		t.Errorf("DBPath = %q, want %q", cfg.DBPath, wantDBPath)
	}
	if cfg.LogLevel != slog.LevelInfo {
		t.Errorf("LogLevel = %v, want %v", cfg.LogLevel, slog.LevelInfo)
	}
	if cfg.AIProvider != "" {
		t.Errorf("AIProvider = %q, want empty", cfg.AIProvider)
	}
	if cfg.GeminiAPIKey != "" {
		t.Errorf("GeminiAPIKey = %q, want empty", cfg.GeminiAPIKey)
	}
	if cfg.GeminiModel != "" {
		t.Errorf("GeminiModel = %q, want empty", cfg.GeminiModel)
	}
	if cfg.SyncURL != "" {
		t.Errorf("SyncURL = %q, want empty", cfg.SyncURL)
	}
	if cfg.APIToken != "" {
		t.Errorf("APIToken = %q, want empty", cfg.APIToken)
	}
}

func TestLoadEnvironmentOverrides(t *testing.T) {
	t.Setenv("NEULSANG_ADDR", "localhost:12345")
	t.Setenv("NEULSANG_DB_PATH", "/tmp/neulsang-test.db")
	t.Setenv("NEULSANG_LOG_LEVEL", "debug")
	t.Setenv("NEULSANG_AI_PROVIDER", "Gemini")
	t.Setenv("NEULSANG_GEMINI_API_KEY", "test-gemini-key")
	t.Setenv("NEULSANG_GEMINI_MODEL", "gemini-test-model")
	t.Setenv("NEULSANG_SYNC_URL", "https://sync.example.test/events")
	t.Setenv("NEULSANG_API_TOKEN", "test-api-token")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if cfg.Addr != "localhost:12345" {
		t.Errorf("Addr = %q, want %q", cfg.Addr, "localhost:12345")
	}
	if cfg.DBPath != "/tmp/neulsang-test.db" {
		t.Errorf("DBPath = %q, want %q", cfg.DBPath, "/tmp/neulsang-test.db")
	}
	if cfg.LogLevel != slog.LevelDebug {
		t.Errorf("LogLevel = %v, want %v", cfg.LogLevel, slog.LevelDebug)
	}
	if cfg.AIProvider != "gemini" {
		t.Errorf("AIProvider = %q, want gemini (lowercased)", cfg.AIProvider)
	}
	if cfg.GeminiAPIKey != "test-gemini-key" {
		t.Errorf("GeminiAPIKey = %q, want test-gemini-key", cfg.GeminiAPIKey)
	}
	if cfg.GeminiModel != "gemini-test-model" {
		t.Errorf("GeminiModel = %q, want gemini-test-model", cfg.GeminiModel)
	}
	if cfg.SyncURL != "https://sync.example.test/events" {
		t.Errorf("SyncURL = %q, want sync URL", cfg.SyncURL)
	}
	if cfg.APIToken != "test-api-token" {
		t.Errorf("APIToken = %q, want test-api-token", cfg.APIToken)
	}
}

func TestLoadInvalidLogLevel(t *testing.T) {
	t.Setenv("NEULSANG_LOG_LEVEL", "verbose")

	if _, err := Load(); err == nil {
		t.Fatal("Load() error = nil, want invalid log level error")
	}
}

func TestLoadAcceptsLoopbackAddr(t *testing.T) {
	tests := []string{"127.0.0.1:48989", "localhost:48989", "[::1]:48989", "LOCALHOST:48989"}
	for _, addr := range tests {
		t.Run(addr, func(t *testing.T) {
			t.Setenv("NEULSANG_ADDR", addr)
			cfg, err := Load()
			if err != nil {
				t.Fatalf("Load() error = %v", err)
			}
			if cfg.Addr != addr {
				t.Errorf("Addr = %q, want %q", cfg.Addr, addr)
			}
		})
	}
}

func TestLoadRejectsNonLoopbackAddr(t *testing.T) {
	tests := []string{"0.0.0.0:48989", "192.168.1.5:48989", ":48989", "example.com:48989", "not-an-addr"}
	for _, addr := range tests {
		t.Run(addr, func(t *testing.T) {
			t.Setenv("NEULSANG_ADDR", addr)
			if _, err := Load(); err == nil {
				t.Fatalf("Load() error = nil, want rejection for non-loopback addr %q", addr)
			}
		})
	}
}
