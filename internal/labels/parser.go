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
	LabelPrefix  = "cloudflare.tunnel."
	LabelEnable  = LabelPrefix + "enable"
	LabelHost    = LabelPrefix + "hostname"
	LabelPath    = LabelPrefix + "path"
	LabelService = LabelPrefix + "service"
)

// Parser converts Docker labels into desired Cloudflare ingress rules.
type Parser struct{}

func NewParser() *Parser {
	return &Parser{}
}

// ParseContainers returns desired routes and any validation errors.
func (parser *Parser) ParseContainers(containers []docker.ContainerInfo) ([]model.RouteSpec, []error) {
	errors := []error{}
	desired := make(map[model.RouteKey]model.RouteSpec)

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

		key := model.RouteKey{Hostname: hostname, Path: path}
		if _, exists := desired[key]; exists {
			errors = append(errors, fmt.Errorf("duplicate route definition for %s", key.String()))
			continue
		}

		source := model.SourceRef{ContainerID: container.ID, ContainerName: container.Name}
		desired[key] = model.RouteSpec{
			Key:     key,
			Service: service,
			Source:  source,
		}
	}

	result := make([]model.RouteSpec, 0, len(desired))
	for _, route := range desired {
		result = append(result, route)
	}

	sort.Slice(result, func(i, j int) bool {
		return result[i].Key.String() < result[j].Key.String()
	})

	return result, errors
}
