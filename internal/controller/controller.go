package controller

import (
	"context"
	"time"

	"log/slog"

	"github.com/darkdragon/docker-cloudflare-tunnel-sync/internal/docker"
	"github.com/darkdragon/docker-cloudflare-tunnel-sync/internal/labels"
	"github.com/darkdragon/docker-cloudflare-tunnel-sync/internal/reconcile"
)

// Controller polls Docker and reconciles Cloudflare ingress rules.
type Controller struct {
	docker     *docker.Adapter
	parser     *labels.Parser
	reconciler *reconcile.Engine
	interval   time.Duration
	log        *slog.Logger
}

func NewController(dockerAdapter *docker.Adapter, parser *labels.Parser, reconciler *reconcile.Engine, interval time.Duration, logger *slog.Logger) *Controller {
	return &Controller{
		docker:     dockerAdapter,
		parser:     parser,
		reconciler: reconciler,
		interval:   interval,
		log:        logger,
	}
}

func (controller *Controller) Run(ctx context.Context, runOnce bool) error {
	if err := controller.syncOnce(ctx); err != nil {
		controller.log.Error("initial sync failed", "error", err)
	}
	if runOnce {
		return nil
	}

	ticker := time.NewTicker(controller.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			if err := controller.syncOnce(ctx); err != nil {
				controller.log.Error("sync failed", "error", err)
			}
		}
	}
}

func (controller *Controller) syncOnce(ctx context.Context) error {
	containers, err := controller.docker.ListRunningContainers(ctx)
	if err != nil {
		return err
	}

	desiredRoutes, errors := controller.parser.ParseContainers(containers)
	for _, parseErr := range errors {
		controller.log.Warn("label parsing error", "error", parseErr)
	}

	return controller.reconciler.Reconcile(ctx, desiredRoutes)
}
