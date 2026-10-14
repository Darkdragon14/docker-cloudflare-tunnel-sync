package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"log/slog"
)

var dockerSecretsDir = "/run/secrets"

// Config captures all runtime configuration derived from environment variables and Docker secrets.
type Config struct {
	Docker     DockerConfig
	Cloudflare CloudflareConfig
	Controller ControllerConfig
	Defaults   DefaultsConfig
	ManagedBy  string
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
	// WatchEvents triggers a sync when Docker reports container start/die/destroy events.
	WatchEvents bool
	// EventDebounce is how long to wait after the last Docker event before syncing.
	EventDebounce time.Duration
	RunOnce       bool
	DryRun        bool
	ManageTunnel  bool
	ManageAccess  bool
	ManageDNS     bool
	DNSZones      []string
	DeleteDNS     bool
}

// DefaultsConfig holds fallback templates for containers without hostname/service labels.
type DefaultsConfig struct {
	// HostnameTemplate is a Go template used when cloudflare.tunnel.hostname is missing (empty = disabled).
	HostnameTemplate string
	// AutoService enables deriving cloudflare.tunnel.service from the exposed port when the label is missing.
	AutoService bool
	// ServiceTemplate is a Go template used by AutoService (empty = built-in default).
	ServiceTemplate string
	// Network selects the Docker network whose IP is exposed to templates as .IP.
	Network string
}

// Load parses configuration from environment variables and Docker secrets.
func Load() (Config, error) {
	pollInterval := getEnvDefault("SYNC_POLL_INTERVAL", "30s")
	parsedInterval, err := time.ParseDuration(pollInterval)
	if err != nil {
		return Config{}, fmt.Errorf("invalid SYNC_POLL_INTERVAL: %w", err)
	}

	watchEvents, err := parseBoolEnv("SYNC_WATCH_EVENTS", true)
	if err != nil {
		return Config{}, err
	}
	parsedDebounce, err := time.ParseDuration(getEnvDefault("SYNC_EVENT_DEBOUNCE", "2s"))
	if err != nil {
		return Config{}, fmt.Errorf("invalid SYNC_EVENT_DEBOUNCE: %w", err)
	}
	if parsedDebounce <= 0 {
		return Config{}, fmt.Errorf("invalid SYNC_EVENT_DEBOUNCE: must be positive")
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
	manageAccess, err := parseBoolEnv("SYNC_MANAGED_ACCESS", false)
	if err != nil {
		return Config{}, err
	}
	manageDNS, err := parseBoolEnv("SYNC_MANAGED_DNS", false)
	if err != nil {
		return Config{}, err
	}
	deleteDNS, err := parseBoolEnv("SYNC_DELETE_DNS", false)
	if err != nil {
		return Config{}, err
	}
	autoService, err := parseBoolEnv("SYNC_AUTO_SERVICE", false)
	if err != nil {
		return Config{}, err
	}
	dnsZones := parseDNSZonesEnv("SYNC_DNS_ZONES")

	managedBy := strings.TrimSpace(os.Getenv("SYNC_MANAGED_BY"))

	logLevel, err := parseLogLevel(getEnvDefault("LOG_LEVEL", "info"))
	if err != nil {
		return Config{}, err
	}

	apiToken, err := requiredSecretOrEnv("CF_API_TOKEN")
	if err != nil {
		return Config{}, err
	}
	accountID, err := requiredSecretOrEnv("CF_ACCOUNT_ID")
	if err != nil {
		return Config{}, err
	}
	tunnelID, err := requiredSecretOrEnv("CF_TUNNEL_ID")
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
			PollInterval:  parsedInterval,
			WatchEvents:   watchEvents,
			EventDebounce: parsedDebounce,
			RunOnce:       runOnce,
			DryRun:        dryRun,
			ManageTunnel:  manageTunnel,
			ManageAccess:  manageAccess,
			ManageDNS:     manageDNS,
			DNSZones:      dnsZones,
			DeleteDNS:     deleteDNS,
		},
		Defaults: DefaultsConfig{
			HostnameTemplate: strings.TrimSpace(os.Getenv("SYNC_DEFAULT_HOSTNAME")),
			AutoService:      autoService,
			ServiceTemplate:  strings.TrimSpace(os.Getenv("SYNC_DEFAULT_SERVICE")),
			Network:          strings.TrimSpace(os.Getenv("SYNC_DOCKER_NETWORK")),
		},
		ManagedBy: managedBy,
		LogLevel:  logLevel,
	}, nil
}

func requiredSecretOrEnv(key string) (string, error) {
	if value, ok, err := dockerSecret(key); err != nil {
		return "", err
	} else if ok {
		return value, nil
	}

	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return "", fmt.Errorf("missing required %s (set %s or /run/secrets/%s)", key, key, key)
	}
	return value, nil
}

func dockerSecret(key string) (string, bool, error) {
	content, err := os.ReadFile(filepath.Join(dockerSecretsDir, key))
	if err != nil {
		if os.IsNotExist(err) {
			return "", false, nil
		}
		return "", false, fmt.Errorf("read Docker secret %s: %w", key, err)
	}

	value := strings.TrimSpace(string(content))
	if value == "" {
		return "", false, nil
	}
	return value, true, nil
}

func getEnvDefault(key, fallback string) string {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	return value
}

func parseDNSZonesEnv(key string) []string {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return nil
	}

	seen := map[string]struct{}{}
	zones := []string{}
	for _, part := range strings.Split(value, ",") {
		zone := strings.ToLower(strings.TrimSuffix(strings.TrimSpace(part), "."))
		if zone == "" {
			continue
		}
		if _, ok := seen[zone]; ok {
			continue
		}
		seen[zone] = struct{}{}
		zones = append(zones, zone)
	}

	return zones
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
