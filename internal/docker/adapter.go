package docker

import (
	"context"
	"strings"

	"github.com/docker/docker/api/types/container"
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
		results = append(results, ContainerInfo{
			ID:     item.ID,
			Name:   name,
			Labels: item.Labels,
		})
	}

	return results, nil
}
