package config

import (
	"fmt"
	"os"
	"strings"
	"time"

	"log/slog"
)

// Config captures all runtime configuration derived from environment variables.
type Config struct {
	Docker     DockerConfig
	Cloudflare CloudflareConfig
	Controller ControllerConfig
	LogLevel   slog.Level
}

type DockerConfig struct {
	Host       string
	APIVersion string
}

type CloudflareConfig struct {
	APIToken  string
	AccountID string
	TunnelID  string
	BaseURL   string
}

type ControllerConfig struct {
	PollInterval time.Duration
	RunOnce      bool
	DryRun       bool
	ManageTunnel bool
}

// Load parses configuration from environment variables.
func Load() (Config, error) {
	pollInterval := getEnvDefault("SYNC_POLL_INTERVAL", "30s")
	parsedInterval, err := time.ParseDuration(pollInterval)
	if err != nil {
		return Config{}, fmt.Errorf("invalid SYNC_POLL_INTERVAL: %w", err)
	}

	runOnce, err := parseBoolEnv("SYNC_RUN_ONCE", false)
	if err != nil {
		return Config{}, err
	}
	dryRun, err := parseBoolEnv("SYNC_DRY_RUN", false)
	if err != nil {
		return Config{}, err
	}
	manageTunnel, err := parseBoolEnv("SYNC_MANAGED_TUNNEL", false)
	if err != nil {
		return Config{}, err
	}

	logLevel, err := parseLogLevel(getEnvDefault("LOG_LEVEL", "info"))
	if err != nil {
		return Config{}, err
	}

	apiToken, err := requiredEnv("CF_API_TOKEN")
	if err != nil {
		return Config{}, err
	}
	accountID, err := requiredEnv("CF_ACCOUNT_ID")
	if err != nil {
		return Config{}, err
	}
	tunnelID, err := requiredEnv("CF_TUNNEL_ID")
	if err != nil {
		return Config{}, err
	}

	return Config{
		Docker: DockerConfig{
			Host:       os.Getenv("DOCKER_HOST"),
			APIVersion: os.Getenv("DOCKER_API_VERSION"),
		},
		Cloudflare: CloudflareConfig{
			APIToken:  apiToken,
			AccountID: accountID,
			TunnelID:  tunnelID,
			BaseURL:   os.Getenv("CF_API_BASE_URL"),
		},
		Controller: ControllerConfig{
			PollInterval: parsedInterval,
			RunOnce:      runOnce,
			DryRun:       dryRun,
			ManageTunnel: manageTunnel,
		},
		LogLevel: logLevel,
	}, nil
}

func requiredEnv(key string) (string, error) {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return "", fmt.Errorf("missing required %s", key)
	}
	return value, nil
}

func getEnvDefault(key, fallback string) string {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	return value
}

func parseBoolEnv(key string, fallback bool) (bool, error) {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback, nil
	}
	parsed, err := parseBool(value)
	if err != nil {
		return false, fmt.Errorf("invalid %s: %w", key, err)
	}
	return parsed, nil
}

func parseBool(value string) (bool, error) {
	switch strings.ToLower(value) {
	case "true", "1", "yes":
		return true, nil
	case "false", "0", "no":
		return false, nil
	default:
		return false, fmt.Errorf("invalid boolean %q", value)
	}
}

func parseLogLevel(value string) (slog.Level, error) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "debug":
		return slog.LevelDebug, nil
	case "info":
		return slog.LevelInfo, nil
	case "warn", "warning":
		return slog.LevelWarn, nil
	case "error":
		return slog.LevelError, nil
	default:
		return slog.LevelInfo, fmt.Errorf("invalid LOG_LEVEL: %s", value)
	}
}
