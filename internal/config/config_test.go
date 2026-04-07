package config

import (
	"reflect"
	"testing"
)

func TestLoadParsesDNSZones(t *testing.T) {
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
