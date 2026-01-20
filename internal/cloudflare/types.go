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

// API defines the Cloudflare operations used by the tunnel reconciler.
type API interface {
	GetConfig(ctx context.Context) (TunnelConfig, error)
	UpdateConfig(ctx context.Context, config TunnelConfig) error
}

// AccessRule represents an Access policy include rule.
type AccessRule struct {
	Email string
	IP    string
}

// AccessPolicyInput describes the payload to create or update a policy.
type AccessPolicyInput struct {
	Name    string
	Action  string
	Include []AccessRule
}

// AccessPolicyRecord represents an Access policy returned by the API.
type AccessPolicyRecord struct {
	ID                  string
	Name                string
	Action              string
	Include             []AccessRule
	HasUnsupportedRules bool
}

// AccessPolicyRef links a policy to an Access application.
type AccessPolicyRef struct {
	ID         string
	Precedence int
}

// AccessAppInput describes the payload to create or update an Access application.
type AccessAppInput struct {
	Name     string
	Domain   string
	Type     string
	Policies []AccessPolicyRef
	Tags     []string
}

// AccessAppRecord represents an Access application returned by the API.
type AccessAppRecord struct {
	ID       string
	Name     string
	Domain   string
	Type     string
	Policies []AccessPolicyRef
	Tags     []string
}

// AccessAPI defines the Cloudflare operations used for Access reconciliation.
type AccessAPI interface {
	ListAccessApps(ctx context.Context) ([]AccessAppRecord, error)
	CreateAccessApp(ctx context.Context, input AccessAppInput) (AccessAppRecord, error)
	UpdateAccessApp(ctx context.Context, id string, input AccessAppInput) (AccessAppRecord, error)
	DeleteAccessApp(ctx context.Context, id string) error
	ListAccessPolicies(ctx context.Context) ([]AccessPolicyRecord, error)
	CreateAccessPolicy(ctx context.Context, input AccessPolicyInput) (AccessPolicyRecord, error)
	UpdateAccessPolicy(ctx context.Context, id string, input AccessPolicyInput) (AccessPolicyRecord, error)
	EnsureAccessTag(ctx context.Context, name string) error
}
