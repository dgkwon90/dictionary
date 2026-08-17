package config

import (
	"fmt"
	"log/slog"
	"net"
	"net/url"
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

	syncURL := envOrDefault("NEULSANG_SYNC_URL", "")
	if err := validateSyncURL(syncURL); err != nil {
		return Config{}, err
	}

	return Config{
		Addr:         addr,
		DBPath:       envOrDefault("NEULSANG_DB_PATH", filepath.Join(configDir, "neulsang", "neulsang.db")),
		LogLevel:     level,
		AIProvider:   strings.ToLower(envOrDefault("NEULSANG_AI_PROVIDER", "")),
		GeminiAPIKey: envOrDefault("NEULSANG_GEMINI_API_KEY", ""),
		GeminiModel:  envOrDefault("NEULSANG_GEMINI_MODEL", ""),
		SyncURL:      syncURL,
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
	if !isLoopbackHost(strings.Trim(host, "[]")) {
		return fmt.Errorf("NEULSANG_ADDR %q must bind to a loopback address (127.0.0.1, ::1, or localhost)", addr)
	}
	return nil
}

// validateSyncURL rejects a sync endpoint that would put the user's captures on
// the wire in the clear. An outbox event carries the capture whole — the text the
// user dragged out of whatever they were reading, and which app it came from
// (internal/domain/capture) — so work material, not just study metadata, is what
// travels. Plain http is allowed only against a loopback host, so that building
// the sync server locally stays possible without turning the escape hatch into
// something a user can point at the internet.
//
// Authentication is a separate, still-open question: syncpush sends no
// Authorization header at all (internal/infra/syncpush). That gets designed when
// there is an actual server to authenticate against; requiring https now is the
// part that costs nothing to get right early.
func validateSyncURL(raw string) error {
	if raw == "" {
		return nil // sync stays off, which is the default
	}
	parsed, err := url.Parse(raw)
	if err != nil {
		return fmt.Errorf("invalid NEULSANG_SYNC_URL %q: %w", raw, err)
	}
	if parsed.Host == "" {
		return fmt.Errorf("NEULSANG_SYNC_URL %q must be an absolute URL including a host", raw)
	}
	switch strings.ToLower(parsed.Scheme) {
	case "https":
		return nil
	case "http":
		if isLoopbackHost(parsed.Hostname()) {
			return nil
		}
		return fmt.Errorf("NEULSANG_SYNC_URL %q must use https — plain http is accepted only for a loopback host", raw)
	default:
		return fmt.Errorf("NEULSANG_SYNC_URL %q must use https (got scheme %q)", raw, parsed.Scheme)
	}
}

// isLoopbackHost reports whether host names this machine and nothing else.
func isLoopbackHost(host string) bool {
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
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
