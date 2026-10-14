package docker

import (
	"context"
	"strings"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/events"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/client"

	"github.com/darkdragon/docker-cloudflare-tunnel-sync/internal/config"
)

// Adapter provides read-only access to the Docker API.
type Adapter struct {
	client *client.Client
}

// NewAdapter creates a Docker adapter configured from environment variables.
func NewAdapter(cfg config.DockerConfig) (*Adapter, error) {
	opts := []client.Opt{client.FromEnv, client.WithAPIVersionNegotiation()}
	if cfg.Host != "" {
		opts = append(opts, client.WithHost(cfg.Host))
	}
	if cfg.APIVersion != "" {
		opts = append(opts, client.WithVersion(cfg.APIVersion))
	}

	dockerClient, err := client.NewClientWithOpts(opts...)
	if err != nil {
		return nil, err
	}

	return &Adapter{client: dockerClient}, nil
}

// ListRunningContainers returns all running containers with their labels.
func (adapter *Adapter) ListRunningContainers(ctx context.Context) ([]ContainerInfo, error) {
	containers, err := adapter.client.ContainerList(ctx, container.ListOptions{All: false})
	if err != nil {
		return nil, err
	}

	results := make([]ContainerInfo, 0, len(containers))
	for _, item := range containers {
		name := ""
		if len(item.Names) > 0 {
			name = strings.TrimPrefix(item.Names[0], "/")
		}
		ports := make([]PortInfo, 0, len(item.Ports))
		for _, port := range item.Ports {
			ports = append(ports, PortInfo{
				PrivatePort: port.PrivatePort,
				PublicPort:  port.PublicPort,
				Type:        port.Type,
			})
		}
		networks := map[string]string{}
		if item.NetworkSettings != nil {
			for networkName, endpoint := range item.NetworkSettings.Networks {
				if endpoint == nil {
					continue
				}
				networks[networkName] = endpoint.IPAddress
			}
		}
		results = append(results, ContainerInfo{
			ID:       item.ID,
			Name:     name,
			Labels:   item.Labels,
			Ports:    ports,
			Networks: networks,
		})
	}

	return results, nil
}

// WatchContainerEvents streams container lifecycle events that can change the set of running
// containers (start, die, destroy). The events channel is closed when the stream ends; the
// error channel receives the reason (including context cancellation).
func (adapter *Adapter) WatchContainerEvents(ctx context.Context) (<-chan ContainerEvent, <-chan error) {
	filter := filters.NewArgs(
		filters.Arg("type", string(events.ContainerEventType)),
		filters.Arg("event", string(events.ActionStart)),
		filters.Arg("event", string(events.ActionDie)),
		filters.Arg("event", string(events.ActionDestroy)),
	)
	messages, errs := adapter.client.Events(ctx, types.EventsOptions{Filters: filter})

	out := make(chan ContainerEvent)
	outErrs := make(chan error, 1)
	go func() {
		defer close(out)
		for {
			select {
			case err := <-errs:
				outErrs <- err
				return
			case message := <-messages:
				event := ContainerEvent{
					ID:     message.Actor.ID,
					Name:   message.Actor.Attributes["name"],
					Action: string(message.Action),
				}
				select {
				case out <- event:
				case <-ctx.Done():
					outErrs <- ctx.Err()
					return
				}
			}
		}
	}()
	return out, outErrs
}
