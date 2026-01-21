package model

import "strings"

const DefaultManagedBy = "docker-cf-tunnel-sync"

func ManagedByValue(value string) string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return DefaultManagedBy
	}
	return trimmed
}

func AccessManagedTag(value string) string {
	return "managed-by=" + ManagedByValue(value)
}

func DNSManagedComment(value string) string {
	return "managed-by=" + ManagedByValue(value)
}
