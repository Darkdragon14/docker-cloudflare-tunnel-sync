package config

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestLoadParsesDNSZones(t *testing.T) {
	withDockerSecretsDir(t, t.TempDir())
	t.Setenv("CF_API_TOKEN", "token")
	t.Setenv("CF_ACCOUNT_ID", "account")
	t.Setenv("CF_TUNNEL_ID", "tunnel")
	t.Setenv("SYNC_DNS_ZONES", "darkdragon.fr, cf.darkdragon.fr. ,darkdragon.fr,,CF.Darkdragon.FR")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	want := []string{"darkdragon.fr", "cf.darkdragon.fr"}
	if !reflect.DeepEqual(cfg.Controller.DNSZones, want) {
		t.Fatalf("unexpected DNS zones: got %+v want %+v", cfg.Controller.DNSZones, want)
	}
}

func TestLoadDefaultsEmptyDNSZones(t *testing.T) {
	withDockerSecretsDir(t, t.TempDir())
	t.Setenv("CF_API_TOKEN", "token")
	t.Setenv("CF_ACCOUNT_ID", "account")
	t.Setenv("CF_TUNNEL_ID", "tunnel")
	t.Setenv("SYNC_DNS_ZONES", "  , ,  ")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(cfg.Controller.DNSZones) != 0 {
		t.Fatalf("expected no DNS zones, got %+v", cfg.Controller.DNSZones)
	}
}

func TestLoadReadsSensitiveValuesFromDockerSecrets(t *testing.T) {
	secretDir := t.TempDir()
	withDockerSecretsDir(t, secretDir)
	writeDockerSecret(t, secretDir, "CF_API_TOKEN", " secret-token\n")
	writeDockerSecret(t, secretDir, "CF_ACCOUNT_ID", " secret-account\n")
	writeDockerSecret(t, secretDir, "CF_TUNNEL_ID", " secret-tunnel\n")
	t.Setenv("CF_API_TOKEN", "env-token")
	t.Setenv("CF_ACCOUNT_ID", "env-account")
	t.Setenv("CF_TUNNEL_ID", "env-tunnel")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.Cloudflare.APIToken != "secret-token" {
		t.Fatalf("unexpected API token: got %q", cfg.Cloudflare.APIToken)
	}
	if cfg.Cloudflare.AccountID != "secret-account" {
		t.Fatalf("unexpected account ID: got %q", cfg.Cloudflare.AccountID)
	}
	if cfg.Cloudflare.TunnelID != "secret-tunnel" {
		t.Fatalf("unexpected tunnel ID: got %q", cfg.Cloudflare.TunnelID)
	}
}

func TestLoadFallsBackToEnvWhenDockerSecretsAreMissing(t *testing.T) {
	withDockerSecretsDir(t, t.TempDir())
	t.Setenv("CF_API_TOKEN", "env-token")
	t.Setenv("CF_ACCOUNT_ID", "env-account")
	t.Setenv("CF_TUNNEL_ID", "env-tunnel")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.Cloudflare.APIToken != "env-token" {
		t.Fatalf("unexpected API token: got %q", cfg.Cloudflare.APIToken)
	}
	if cfg.Cloudflare.AccountID != "env-account" {
		t.Fatalf("unexpected account ID: got %q", cfg.Cloudflare.AccountID)
	}
	if cfg.Cloudflare.TunnelID != "env-tunnel" {
		t.Fatalf("unexpected tunnel ID: got %q", cfg.Cloudflare.TunnelID)
	}
}

func withDockerSecretsDir(t *testing.T, dir string) {
	t.Helper()
	previous := dockerSecretsDir
	dockerSecretsDir = dir
	t.Cleanup(func() {
		dockerSecretsDir = previous
	})
}

func writeDockerSecret(t *testing.T, dir, key, value string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, key), []byte(value), 0o600); err != nil {
		t.Fatalf("write Docker secret %s: %v", key, err)
	}
}
