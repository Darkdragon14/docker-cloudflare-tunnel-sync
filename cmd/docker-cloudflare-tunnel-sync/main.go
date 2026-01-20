package main

import (
	"context"
	"errors"
	"os"
	"os/signal"
	"syscall"

	"log/slog"

	"github.com/darkdragon/docker-cloudflare-tunnel-sync/internal/access"
	"github.com/darkdragon/docker-cloudflare-tunnel-sync/internal/cloudflare"
	"github.com/darkdragon/docker-cloudflare-tunnel-sync/internal/config"
	"github.com/darkdragon/docker-cloudflare-tunnel-sync/internal/controller"
	"github.com/darkdragon/docker-cloudflare-tunnel-sync/internal/docker"
	"github.com/darkdragon/docker-cloudflare-tunnel-sync/internal/labels"
	"github.com/darkdragon/docker-cloudflare-tunnel-sync/internal/reconcile"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log := slog.New(slog.NewTextHandler(os.Stdout, nil))
		log.Error("failed to load configuration", "error", err)
		os.Exit(1)
	}

	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: cfg.LogLevel}))

	dockerAdapter, err := docker.NewAdapter(cfg.Docker)
	if err != nil {
		logger.Error("failed to initialize Docker adapter", "error", err)
		os.Exit(1)
	}

	cloudflareClient, err := cloudflare.NewClient(cfg.Cloudflare)
	if err != nil {
		logger.Error("failed to initialize Cloudflare client", "error", err)
		os.Exit(1)
	}

	parser := labels.NewParser()
	reconciler := reconcile.NewEngine(cloudflareClient, logger, cfg.Controller.DryRun, cfg.Controller.ManageTunnel)
	accessEngine := access.NewEngine(cloudflareClient, logger, cfg.Controller.DryRun, cfg.Controller.ManageAccess)
	controller := controller.NewController(dockerAdapter, parser, reconciler, accessEngine, cfg.Controller.PollInterval, logger)

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	if err := controller.Run(ctx, cfg.Controller.RunOnce); err != nil && !errors.Is(err, context.Canceled) {
		logger.Error("controller stopped with error", "error", err)
		os.Exit(1)
	}
}
