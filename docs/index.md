---
layout: page
title: Docker Cloudflare Tunnel Sync
permalink: /
toc: false
---

# Traefik-like Labels for Cloudflare Tunnel

Docker Cloudflare Tunnel Sync exposes Docker services through Cloudflare Tunnel using Docker labels.

Instead of manually editing Cloudflare Tunnel ingress rules every time you add, rename, move, or remove a container, you declare the desired public route directly on the container itself.

Your Docker labels become the source of truth for Cloudflare Tunnel routes, with optional DNS and Cloudflare Access synchronization.

```yaml
services:
  app:
    image: nginx
    labels:
      cloudflare.tunnel.enable: "true"
      cloudflare.tunnel.hostname: app.example.com
      cloudflare.tunnel.service: http://app:80
```

## Highlights

- Define Cloudflare Tunnel ingress rules from Docker labels.
- Keep containers as the source of truth for exposed services.
- Synchronize Cloudflare Tunnel routes automatically.
- Optionally manage DNS CNAME records for tunnel hostnames.
- Optionally manage Cloudflare Access applications and policies.
- Support multiple routes per container with suffix-based labels.
- Configure origin settings such as SNI override and TLS verification.
- Test changes safely with dry-run mode.
- Scope DNS cleanup to avoid touching unrelated Cloudflare zones.
- Keep sensitive Cloudflare values in Docker secrets.

## Quick Start

Run the controller with a scoped Cloudflare API token, account ID, tunnel ID, and a read-only Docker socket mount:

```bash
docker run --rm \
  -e CF_API_TOKEN=your-token \
  -e CF_ACCOUNT_ID=your-account-id \
  -e CF_TUNNEL_ID=your-tunnel-id \
  -e SYNC_MANAGED_TUNNEL=true \
  -v /var/run/docker.sock:/var/run/docker.sock:ro \
  ghcr.io/darkdragon14/docker-cloudflare-tunnel-sync
```

Then label a container:

```yaml
services:
  whoami:
    image: traefik/whoami
    labels:
      cloudflare.tunnel.enable: "true"
      cloudflare.tunnel.hostname: whoami.example.com
      cloudflare.tunnel.service: http://whoami:80
```

The controller reconciles the Cloudflare Tunnel configuration from the labels.

## Safety Warning

Use a dedicated Cloudflare Tunnel for this controller.

When managed tunnel synchronization is enabled, routes that are no longer declared by Docker labels may be removed from the managed tunnel. Keeping a dedicated tunnel makes cleanup predictable and avoids deleting manually configured application routes.

## Documentation

- [Installation]({{ '/installation/' | relative_url }}): Cloudflare prerequisites, API token, controller deployment, and Docker secrets.
- [Configuration]({{ '/configuration/' | relative_url }}): environment variables, sync modes, dry-run, polling, and managed behavior.
- [Labels Reference]({{ '/labels/' | relative_url }}): base route labels, multiple routes, DNS labels, Access labels, and origin settings.
- [DNS Synchronization]({{ '/dns/' | relative_url }}): managed DNS records, zone selection, delegated zones, and cleanup scope.
- [Cloudflare Access]({{ '/access/' | relative_url }}): Access applications, policies, tags, references, and limitations.
- [Safety Model]({{ '/safety/' | relative_url }}): dedicated tunnel recommendation, cleanup behavior, Docker socket risk, and dry-run usage.
- [Examples]({{ '/examples/' | relative_url }}): common Compose examples for self-hosted services.
- [Migration to v1.0]({{ '/migration-v1/' | relative_url }}): stable label format, recommended setup, and migration notes.

## When should I use it?

Use Docker Cloudflare Tunnel Sync if:

- you expose Docker services through Cloudflare Tunnel;
- you want routing rules to live next to your Compose services;
- you prefer labels over manually editing tunnel ingress rules;
- you want a Traefik-like label experience for Cloudflare Tunnel;
- you want optional DNS and Access management from the same source of truth.

It is especially useful for homelabs, self-hosted dashboards, internal tools, and services that are frequently added, removed, or moved between Compose stacks.

## When should I not use it?

This project may not be the right fit if:

- you only have one or two static routes;
- you prefer managing everything manually from the Cloudflare dashboard;
- you do not want a controller to modify Cloudflare Tunnel configuration;
- you need a full reverse proxy replacement.

Docker Cloudflare Tunnel Sync does not replace Traefik, Caddy, Nginx, or Cloudflare Tunnel itself. It only synchronizes Cloudflare Tunnel configuration from Docker labels.

## Project Status

Docker Cloudflare Tunnel Sync is being prepared for a stable v1.0 release.

The goal of v1.0 is to provide a stable label format, clearer documentation, safer defaults, better migration guidance, and a cleaner onboarding experience.

## License

MIT
