package model

import "fmt"

// RouteKey identifies a unique Cloudflare Tunnel ingress rule.
type RouteKey struct {
	Hostname string
	Path     string
}

func (key RouteKey) String() string {
	if key.Path == "" {
		return key.Hostname
	}
	return fmt.Sprintf("%s%s", key.Hostname, key.Path)
}

// SourceRef captures where a desired route came from.
type SourceRef struct {
	ContainerID   string
	ContainerName string
}

// RouteSpec describes the desired ingress rule state derived from Docker labels.
type RouteSpec struct {
	Key              RouteKey
	Service          string
	OriginServerName *string
	NoTLSVerify      *bool
	Source           SourceRef
}
