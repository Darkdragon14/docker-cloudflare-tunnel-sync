package labels

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/darkdragon/docker-cloudflare-tunnel-sync/internal/docker"
	"github.com/darkdragon/docker-cloudflare-tunnel-sync/internal/model"
)

const (
	LabelPrefix            = "cloudflare.tunnel."
	LabelEnable            = LabelPrefix + "enable"
	LabelHost              = LabelPrefix + "hostname"
	LabelPath              = LabelPrefix + "path"
	LabelService           = LabelPrefix + "service"
	LabelOriginServerName  = LabelPrefix + "origin.server-name"
	LabelOriginNoTLSVerify = LabelPrefix + "origin.no-tls-verify"

	AccessLabelPrefix       = "cloudflare.access."
	AccessLabelEnable       = AccessLabelPrefix + "enable"
	AccessLabelAppName      = AccessLabelPrefix + "app.name"
	AccessLabelAppDomain    = AccessLabelPrefix + "app.domain"
	AccessLabelAppID        = AccessLabelPrefix + "app.id"
	AccessLabelAppTags      = AccessLabelPrefix + "app.tags"
	AccessLabelPolicyPrefix = AccessLabelPrefix + "policy."
)

// Parser converts Docker labels into desired Cloudflare ingress rules.
type Parser struct{}

func NewParser() *Parser {
	return &Parser{}
}

// ParseContainers returns desired tunnel ingress rules and any validation errors.
func (parser *Parser) ParseContainers(containers []docker.ContainerInfo) ([]model.RouteSpec, []error) {
	errors := []error{}
	desired := []model.RouteSpec{}
	desiredKeys := map[model.RouteKey]struct{}{}

	sorted := make([]docker.ContainerInfo, len(containers))
	copy(sorted, containers)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].ID < sorted[j].ID
	})

	for _, container := range sorted {
		enabled, hasEnable := container.Labels[LabelEnable]
		if !hasEnable {
			continue
		}
		flag, err := strconv.ParseBool(enabled)
		if err != nil || !flag {
			if err != nil {
				errors = append(errors, fmt.Errorf("container %s: invalid %s label: %w", container.Name, LabelEnable, err))
			}
			continue
		}

		hostname := strings.TrimSpace(container.Labels[LabelHost])
		service := strings.TrimSpace(container.Labels[LabelService])
		path := strings.TrimSpace(container.Labels[LabelPath])

		if hostname == "" {
			errors = append(errors, fmt.Errorf("container %s: missing required %s label", container.Name, LabelHost))
			continue
		}
		if service == "" {
			errors = append(errors, fmt.Errorf("container %s: missing required %s label", container.Name, LabelService))
			continue
		}
		if path != "" && !strings.HasPrefix(path, "/") {
			errors = append(errors, fmt.Errorf("container %s: %s must start with '/'", container.Name, LabelPath))
			continue
		}

		originServerName, originNoTLSVerify, err := parseOriginLabels(container.Name, container.Labels, LabelOriginServerName, LabelOriginNoTLSVerify)
		if err != nil {
			errors = append(errors, err)
			continue
		}

		key := model.RouteKey{Hostname: hostname, Path: path}
		source := model.SourceRef{ContainerID: container.ID, ContainerName: container.Name}
		if err := appendRouteSpec(&desired, desiredKeys, model.RouteSpec{
			Key:              key,
			Service:          service,
			OriginServerName: originServerName,
			NoTLSVerify:      originNoTLSVerify,
			Source:           source,
		}); err != nil {
			errors = append(errors, err)
		}

		hostSuffixes := collectSuffixes(container.Labels, LabelHost)
		serviceSuffixes := collectSuffixes(container.Labels, LabelService)

		hostSuffixList := sortedSuffixes(hostSuffixes)
		for _, suffix := range hostSuffixList {
			if _, ok := serviceSuffixes[suffix]; ok {
				continue
			}
			errors = append(errors, fmt.Errorf("container %s: %s.%s is set without matching %s.%s; skipping", container.Name, LabelHost, suffix, LabelService, suffix))
		}

		serviceSuffixList := sortedSuffixes(serviceSuffixes)
		for _, suffix := range serviceSuffixList {
			if _, ok := hostSuffixes[suffix]; ok {
				continue
			}
			errors = append(errors, fmt.Errorf("container %s: %s.%s is set without matching %s.%s; skipping", container.Name, LabelService, suffix, LabelHost, suffix))
		}

		for _, suffix := range hostSuffixList {
			if _, ok := serviceSuffixes[suffix]; !ok {
				continue
			}

			hostnameKey := LabelHost + "." + suffix
			serviceKey := LabelService + "." + suffix
			pathKey := LabelPath + "." + suffix
			originServerNameKey := LabelOriginServerName + "." + suffix
			originNoTLSVerifyKey := LabelOriginNoTLSVerify + "." + suffix

			hostname := strings.TrimSpace(container.Labels[hostnameKey])
			service := strings.TrimSpace(container.Labels[serviceKey])
			path := strings.TrimSpace(container.Labels[pathKey])
			if hostname == "" {
				errors = append(errors, fmt.Errorf("container %s: %s cannot be empty; skipping", container.Name, hostnameKey))
				continue
			}
			if service == "" {
				errors = append(errors, fmt.Errorf("container %s: %s cannot be empty; skipping", container.Name, serviceKey))
				continue
			}
			if path != "" && !strings.HasPrefix(path, "/") {
				errors = append(errors, fmt.Errorf("container %s: %s must start with '/'; skipping", container.Name, pathKey))
				continue
			}

			originServerName, originNoTLSVerify, err := parseOriginLabels(container.Name, container.Labels, originServerNameKey, originNoTLSVerifyKey)
			if err != nil {
				errors = append(errors, fmt.Errorf("%w; skipping", err))
				continue
			}

			key := model.RouteKey{Hostname: hostname, Path: path}
			if err := appendRouteSpec(&desired, desiredKeys, model.RouteSpec{
				Key:              key,
				Service:          service,
				OriginServerName: originServerName,
				NoTLSVerify:      originNoTLSVerify,
				Source:           source,
			}); err != nil {
				errors = append(errors, err)
			}
		}
	}

	return desired, errors
}

func appendRouteSpec(desired *[]model.RouteSpec, desiredKeys map[model.RouteKey]struct{}, route model.RouteSpec) error {
	if _, exists := desiredKeys[route.Key]; exists {
		return fmt.Errorf("duplicate route definition for %s", route.Key.String())
	}
	desiredKeys[route.Key] = struct{}{}
	*desired = append(*desired, route)
	return nil
}

func collectSuffixes(labels map[string]string, baseLabel string) map[string]struct{} {
	set := map[string]struct{}{}
	prefix := baseLabel + "."
	for labelKey := range labels {
		if !strings.HasPrefix(labelKey, prefix) {
			continue
		}
		suffix := strings.TrimPrefix(labelKey, prefix)
		if suffix == "" {
			continue
		}
		set[suffix] = struct{}{}
	}
	return set
}

func sortedSuffixes(set map[string]struct{}) []string {
	items := make([]string, 0, len(set))
	for suffix := range set {
		items = append(items, suffix)
	}
	sort.Strings(items)
	return items
}

func parseOriginLabels(containerName string, labels map[string]string, serverNameLabel string, noTLSVerifyLabel string) (*string, *bool, error) {
	var originServerName *string
	if originServerNameValue, hasOriginServerName := labels[serverNameLabel]; hasOriginServerName {
		trimmedServerName := strings.TrimSpace(originServerNameValue)
		if trimmedServerName == "" {
			return nil, nil, fmt.Errorf("container %s: %s cannot be empty", containerName, serverNameLabel)
		}
		originServerName = &trimmedServerName
	}

	var originNoTLSVerify *bool
	if originNoTLSVerifyValue, hasOriginNoTLSVerify := labels[noTLSVerifyLabel]; hasOriginNoTLSVerify {
		parsedNoTLSVerify, err := strconv.ParseBool(strings.TrimSpace(originNoTLSVerifyValue))
		if err != nil {
			return nil, nil, fmt.Errorf("container %s: invalid %s label: %w", containerName, noTLSVerifyLabel, err)
		}
		originNoTLSVerify = &parsedNoTLSVerify
	}

	return originServerName, originNoTLSVerify, nil
}

// ParseAccessContainers returns desired Access apps and any validation errors.
func (parser *Parser) ParseAccessContainers(containers []docker.ContainerInfo) ([]model.AccessAppSpec, []error) {
	errors := []error{}
	desired := make(map[accessAppKey]model.AccessAppSpec)

	sorted := make([]docker.ContainerInfo, len(containers))
	copy(sorted, containers)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].ID < sorted[j].ID
	})

	for _, container := range sorted {
		enabledValue, hasEnable := container.Labels[AccessLabelEnable]
		if !hasEnable {
			continue
		}
		enabled, err := strconv.ParseBool(enabledValue)
		if err != nil || !enabled {
			if err != nil {
				errors = append(errors, fmt.Errorf("container %s: invalid %s label: %w", container.Name, AccessLabelEnable, err))
			}
			continue
		}

		appName := strings.TrimSpace(container.Labels[AccessLabelAppName])
		appDomain := strings.TrimSpace(container.Labels[AccessLabelAppDomain])
		appID := strings.TrimSpace(container.Labels[AccessLabelAppID])
		appTagsValue, hasAppTags := container.Labels[AccessLabelAppTags]
		appTags := []string(nil)
		if hasAppTags {
			appTags = splitCommaList(appTagsValue)
		}

		if appName == "" {
			errors = append(errors, fmt.Errorf("container %s: missing required %s label", container.Name, AccessLabelAppName))
			continue
		}
		if appDomain == "" {
			tunnelDomain := strings.TrimSpace(container.Labels[LabelHost])
			if tunnelDomain == "" {
				errors = append(errors, fmt.Errorf("container %s: missing %s; set %s or %s", container.Name, AccessLabelAppDomain, AccessLabelAppDomain, LabelHost))
				continue
			}
			appDomain = tunnelDomain
		}

		policies, policyErrors := parseAccessPolicies(container)
		errors = append(errors, policyErrors...)
		if len(policies) == 0 {
			errors = append(errors, fmt.Errorf("container %s: no access policies configured", container.Name))
			continue
		}

		key := accessAppKey{Name: appName, Domain: appDomain}
		if _, exists := desired[key]; exists {
			errors = append(errors, fmt.Errorf("duplicate access app definition for %s", key.String()))
			continue
		}

		source := model.SourceRef{ContainerID: container.ID, ContainerName: container.Name}
		desired[key] = model.AccessAppSpec{
			ID:       appID,
			Name:     appName,
			Domain:   appDomain,
			Policies: policies,
			Tags:     appTags,
			TagsSet:  hasAppTags,
			Source:   source,
		}
	}

	result := make([]model.AccessAppSpec, 0, len(desired))
	for _, app := range desired {
		result = append(result, app)
	}

	sort.Slice(result, func(i, j int) bool {
		return accessAppKey{Name: result[i].Name, Domain: result[i].Domain}.String() < accessAppKey{Name: result[j].Name, Domain: result[j].Domain}.String()
	})

	return result, errors
}

type accessAppKey struct {
	Name   string
	Domain string
}

func (key accessAppKey) String() string {
	return fmt.Sprintf("%s@%s", key.Name, key.Domain)
}

type accessPolicyBuilder struct {
	ID            string
	Name          string
	Action        string
	IncludeEmails []string
	IncludeIPs    []string
}

func parseAccessPolicies(container docker.ContainerInfo) ([]model.AccessPolicySpec, []error) {
	policies := map[int]*accessPolicyBuilder{}
	errors := []error{}

	for labelKey, value := range container.Labels {
		if !strings.HasPrefix(labelKey, AccessLabelPolicyPrefix) {
			continue
		}
		remainder := strings.TrimPrefix(labelKey, AccessLabelPolicyPrefix)
		parts := strings.Split(remainder, ".")
		if len(parts) < 2 {
			errors = append(errors, fmt.Errorf("container %s: invalid access policy label %s", container.Name, labelKey))
			continue
		}

		index, err := strconv.Atoi(parts[0])
		if err != nil || index < 1 {
			errors = append(errors, fmt.Errorf("container %s: invalid access policy index in %s", container.Name, labelKey))
			continue
		}
		field := strings.Join(parts[1:], ".")
		builder := policies[index]
		if builder == nil {
			builder = &accessPolicyBuilder{}
			policies[index] = builder
		}

		trimmed := strings.TrimSpace(value)
		switch field {
		case "name":
			builder.Name = trimmed
		case "action":
			builder.Action = strings.ToLower(trimmed)
		case "id":
			builder.ID = trimmed
		case "include.emails":
			builder.IncludeEmails = splitCommaList(trimmed)
		case "include.ips":
			builder.IncludeIPs = splitCommaList(trimmed)
		default:
			errors = append(errors, fmt.Errorf("container %s: unknown access policy label %s", container.Name, labelKey))
		}
	}

	indexes := make([]int, 0, len(policies))
	for index := range policies {
		indexes = append(indexes, index)
	}
	sort.Ints(indexes)

	result := make([]model.AccessPolicySpec, 0, len(indexes))
	for _, index := range indexes {
		policy := policies[index]
		referenceOnly := policy.Action == "" && len(policy.IncludeEmails) == 0 && len(policy.IncludeIPs) == 0
		managed := !referenceOnly
		if referenceOnly {
			if policy.ID == "" && policy.Name == "" {
				errors = append(errors, fmt.Errorf("container %s: access policy %d missing id or name", container.Name, index))
				continue
			}
		}
		if managed {
			if policy.Name == "" {
				errors = append(errors, fmt.Errorf("container %s: access policy %d missing name", container.Name, index))
				continue
			}
			switch policy.Action {
			case "allow", "deny":
				// valid
			case "":
				errors = append(errors, fmt.Errorf("container %s: access policy %d missing action", container.Name, index))
				continue
			default:
				errors = append(errors, fmt.Errorf("container %s: access policy %d has invalid action %q", container.Name, index, policy.Action))
				continue
			}
			if len(policy.IncludeEmails) == 0 && len(policy.IncludeIPs) == 0 {
				errors = append(errors, fmt.Errorf("container %s: access policy %d has no include rules", container.Name, index))
				continue
			}
		}

		result = append(result, model.AccessPolicySpec{
			ID:            policy.ID,
			Name:          policy.Name,
			Action:        policy.Action,
			IncludeEmails: policy.IncludeEmails,
			IncludeIPs:    policy.IncludeIPs,
			Managed:       managed,
		})
	}

	return result, errors
}

func splitCommaList(value string) []string {
	if value == "" {
		return nil
	}
	parts := strings.Split(value, ",")
	items := make([]string, 0, len(parts))
	for _, part := range parts {
		item := strings.TrimSpace(part)
		if item == "" {
			continue
		}
		items = append(items, item)
	}
	return items
}
