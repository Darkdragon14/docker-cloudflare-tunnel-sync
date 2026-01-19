package cloudflare

import (
	"context"
	"encoding/json"
)

// IngressRule represents a Cloudflare Tunnel ingress rule.
type IngressRule struct {
	Hostname      string          `json:"hostname,omitempty"`
	Path          string          `json:"path,omitempty"`
	Service       string          `json:"service"`
	OriginRequest json.RawMessage `json:"originRequest,omitempty"`
}

// TunnelConfig contains the tunnel configuration payload plus parsed ingress rules.
type TunnelConfig struct {
	Ingress []IngressRule
	Raw     map[string]json.RawMessage
}

// API defines the Cloudflare operations used by the reconciler.
type API interface {
	GetConfig(ctx context.Context) (TunnelConfig, error)
	UpdateConfig(ctx context.Context, config TunnelConfig) error
}
