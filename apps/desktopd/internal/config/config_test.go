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
}

func TestLoadEnvironmentOverrides(t *testing.T) {
	t.Setenv("NEULSANG_ADDR", "localhost:12345")
	t.Setenv("NEULSANG_DB_PATH", "/tmp/neulsang-test.db")
	t.Setenv("NEULSANG_LOG_LEVEL", "debug")

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
}

func TestLoadInvalidLogLevel(t *testing.T) {
	t.Setenv("NEULSANG_LOG_LEVEL", "verbose")

	if _, err := Load(); err == nil {
		t.Fatal("Load() error = nil, want invalid log level error")
	}
}
