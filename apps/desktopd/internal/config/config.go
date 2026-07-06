package config

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
)

const defaultAddr = "127.0.0.1:48989"

type Config struct {
	Addr     string
	DBPath   string
	LogLevel slog.Level
}

func Load() (Config, error) {
	configDir, err := os.UserConfigDir()
	if err != nil {
		return Config{}, fmt.Errorf("get user config directory: %w", err)
	}

	level, err := parseLogLevel(envOrDefault("NEULSANG_LOG_LEVEL", "info"))
	if err != nil {
		return Config{}, err
	}

	return Config{
		Addr:     envOrDefault("NEULSANG_ADDR", defaultAddr),
		DBPath:   envOrDefault("NEULSANG_DB_PATH", filepath.Join(configDir, "neulsang", "neulsang.db")),
		LogLevel: level,
	}, nil
}

func envOrDefault(name, fallback string) string {
	if value := os.Getenv(name); value != "" {
		return value
	}
	return fallback
}

func parseLogLevel(value string) (slog.Level, error) {
	switch strings.ToLower(value) {
	case "debug":
		return slog.LevelDebug, nil
	case "info":
		return slog.LevelInfo, nil
	case "warn":
		return slog.LevelWarn, nil
	case "error":
		return slog.LevelError, nil
	default:
		return 0, fmt.Errorf("invalid log level %q", value)
	}
}
