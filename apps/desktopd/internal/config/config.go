package config

import (
	"fmt"
	"log/slog"
	"net"
	"os"
	"path/filepath"
	"strings"
)

const defaultAddr = "127.0.0.1:48989"

type Config struct {
	Addr         string
	DBPath       string
	LogLevel     slog.Level
	AIProvider   string
	GeminiAPIKey string
	GeminiModel  string
	SyncURL      string
	// APIToken authenticates every /v1/* request (review R-01). Empty means
	// unset — bootstrap generates and logs a session-only token in that case,
	// so a bare `go run`/curl dev workflow still works without a fixed value.
	APIToken string
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

	addr := envOrDefault("NEULSANG_ADDR", defaultAddr)
	if err := validateLoopbackAddr(addr); err != nil {
		return Config{}, err
	}

	return Config{
		Addr:         addr,
		DBPath:       envOrDefault("NEULSANG_DB_PATH", filepath.Join(configDir, "neulsang", "neulsang.db")),
		LogLevel:     level,
		AIProvider:   strings.ToLower(envOrDefault("NEULSANG_AI_PROVIDER", "")),
		GeminiAPIKey: envOrDefault("NEULSANG_GEMINI_API_KEY", ""),
		GeminiModel:  envOrDefault("NEULSANG_GEMINI_MODEL", ""),
		SyncURL:      envOrDefault("NEULSANG_SYNC_URL", ""),
		APIToken:     envOrDefault("NEULSANG_API_TOKEN", ""),
	}, nil
}

// validateLoopbackAddr rejects a bind address that isn't loopback-only (review
// R-01: desktopd is a single-user local sidecar with no legitimate reason to
// accept connections from other machines, and a non-loopback bind would expose
// the trust-boundary gap this review closes to the network instead of just this
// machine's browsers).
func validateLoopbackAddr(addr string) error {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		return fmt.Errorf("invalid NEULSANG_ADDR %q: %w", addr, err)
	}
	host = strings.Trim(host, "[]")
	if strings.EqualFold(host, "localhost") {
		return nil
	}
	ip := net.ParseIP(host)
	if ip == nil || !ip.IsLoopback() {
		return fmt.Errorf("NEULSANG_ADDR %q must bind to a loopback address (127.0.0.1, ::1, or localhost)", addr)
	}
	return nil
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
