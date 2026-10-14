package controller

import (
	"context"
	"time"

	"log/slog"

	"github.com/darkdragon/docker-cloudflare-tunnel-sync/internal/access"
	"github.com/darkdragon/docker-cloudflare-tunnel-sync/internal/dns"
	"github.com/darkdragon/docker-cloudflare-tunnel-sync/internal/docker"
	"github.com/darkdragon/docker-cloudflare-tunnel-sync/internal/labels"
	"github.com/darkdragon/docker-cloudflare-tunnel-sync/internal/reconcile"
)

// Controller polls Docker and reconciles ingress, DNS, and Access resources.
type Controller struct {
	docker       *docker.Adapter
	parser       *labels.Parser
	reconciler   *reconcile.Engine
	dnsEngine    *dns.Engine
	accessEngine *access.Engine
	interval     time.Duration
	watchEvents  bool
	debounce     time.Duration
	log          *slog.Logger
}

// eventRetryDelay is the wait before reconnecting to the Docker event stream after it fails.
const eventRetryDelay = 5 * time.Second

func NewController(dockerAdapter *docker.Adapter, parser *labels.Parser, reconciler *reconcile.Engine, dnsEngine *dns.Engine, accessEngine *access.Engine, interval time.Duration, watchEvents bool, debounce time.Duration, logger *slog.Logger) *Controller {
	return &Controller{
		docker:       dockerAdapter,
		parser:       parser,
		reconciler:   reconciler,
		dnsEngine:    dnsEngine,
		accessEngine: accessEngine,
		interval:     interval,
		watchEvents:  watchEvents,
		debounce:     debounce,
		log:          logger,
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

	// Events only signal that a sync is needed; syncs always run on this goroutine so they never overlap.
	triggers := make(chan struct{}, 1)
	if controller.watchEvents {
		go controller.watchDockerEvents(ctx, triggers)
	}

	debounce := time.NewTimer(controller.debounce)
	if !debounce.Stop() {
		<-debounce.C
	}
	defer debounce.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			if err := controller.syncOnce(ctx); err != nil {
				controller.log.Error("sync failed", "error", err)
			}
		case <-triggers:
			// Coalesce bursts (e.g. docker compose up recreating several containers) into one sync.
			debounce.Reset(controller.debounce)
		case <-debounce.C:
			controller.log.Info("syncing after Docker container events")
			if err := controller.syncOnce(ctx); err != nil {
				controller.log.Error("sync failed", "error", err)
			}
			ticker.Reset(controller.interval)
		}
	}
}

// watchDockerEvents forwards container lifecycle events as sync triggers and reconnects on failure.
func (controller *Controller) watchDockerEvents(ctx context.Context, triggers chan<- struct{}) {
	for {
		events, errs := controller.docker.WatchContainerEvents(ctx)
		controller.log.Info("watching Docker container events")
		for event := range events {
			controller.log.Debug("Docker container event", "action", event.Action, "container", event.Name, "id", event.ID)
			select {
			case triggers <- struct{}{}:
			default:
			}
		}

		err := <-errs
		if ctx.Err() != nil {
			return
		}
		controller.log.Warn("Docker event stream stopped; relying on polling until reconnect", "error", err, "retry_in", eventRetryDelay)
		select {
		case <-ctx.Done():
			return
		case <-time.After(eventRetryDelay):
		}
		// Events may have been missed while disconnected.
		select {
		case triggers <- struct{}{}:
		default:
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

	if err := controller.reconciler.Reconcile(ctx, desiredRoutes); err != nil {
		return err
	}

	if controller.dnsEngine != nil {
		if err := controller.dnsEngine.Reconcile(ctx, desiredRoutes); err != nil {
			controller.log.Error("DNS sync failed", "error", err)
		}
	}

	if controller.accessEngine == nil {
		return nil
	}

	accessApps, accessErrors := controller.parser.ParseAccessContainers(containers)
	for _, parseErr := range accessErrors {
		controller.log.Warn("access label parsing error", "error", parseErr)
	}

	return controller.accessEngine.Reconcile(ctx, accessApps)
}
