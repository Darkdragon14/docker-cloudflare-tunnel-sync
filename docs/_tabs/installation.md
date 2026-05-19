---
layout: page
title: Installation
permalink: /installation/
icon: fas fa-download
order: 1
---

# Installation

This guide explains how to deploy Docker Cloudflare Tunnel Sync and connect it to a dedicated Cloudflare Tunnel.

## Prerequisites

You need:

- a Cloudflare account;
- an existing Cloudflare Tunnel;
- a Cloudflare API token;
- Docker or Docker Compose;
- access to the Docker socket on the host running the controller.

## Recommended Cloudflare setup

Use a dedicated Cloudflare Tunnel for this controller.

This keeps label-managed routes isolated from manually configured routes and makes cleanup behavior predictable.

## Cloudflare API token

Create a scoped Cloudflare API token instead of using a Global API Key.

Recommended permissions:

| Scope | Resource | Access | Required for |
| --- | --- | --- | --- |
| Account | Cloudflare Tunnel | Edit | Tunnel ingress synchronization |
| Account | Access: Apps and Policies | Edit | Cloudflare Access synchronization |
| Zone | Zone | Read | DNS zone lookup |
| Zone | DNS | Edit | DNS record synchronization |

If you do not use DNS synchronization, the DNS permissions are not required.

If you do not use Cloudflare Access synchronization, the Access permission is not required.

## Run with Docker

```bash
docker run --rm \
  -e CF_API_TOKEN=your-token \
  -e CF_ACCOUNT_ID=your-account-id \
  -e CF_TUNNEL_ID=your-tunnel-id \
  -e SYNC_MANAGED_TUNNEL=true \
  -e SYNC_POLL_INTERVAL=30s \
  -v /var/run/docker.sock:/var/run/docker.sock:ro \
  ghcr.io/darkdragon14/docker-cloudflare-tunnel-sync
```

The Docker socket is mounted read-only because the controller only needs to read container metadata and labels.

## Run with Docker Compose

```yaml
services:
  docker-cloudflare-tunnel-sync:
    image: ghcr.io/darkdragon14/docker-cloudflare-tunnel-sync
    environment:
      CF_API_TOKEN: your-token
      CF_ACCOUNT_ID: your-account-id
      CF_TUNNEL_ID: your-tunnel-id
      SYNC_MANAGED_TUNNEL: "true"
      SYNC_MANAGED_ACCESS: "false"
      SYNC_MANAGED_DNS: "false"
      SYNC_DELETE_DNS: "false"
      SYNC_POLL_INTERVAL: 30s
    volumes:
      - /var/run/docker.sock:/var/run/docker.sock:ro
    restart: unless-stopped
```

## Docker secrets

Sensitive Cloudflare values can also be provided with Docker secrets. The controller checks `/run/secrets/<VARIABLE>` before falling back to the matching environment variable.

Supported secrets:

| Secret | Required | Description |
| --- | --- | --- |
| `CF_API_TOKEN` | yes | Cloudflare API token. |
| `CF_ACCOUNT_ID` | yes | Cloudflare account identifier. |
| `CF_TUNNEL_ID` | yes | Cloudflare Tunnel identifier. |

## First managed service

Add labels to a container you want to expose:

```yaml
services:
  app:
    image: nginx
    labels:
      cloudflare.tunnel.enable: "true"
      cloudflare.tunnel.hostname: app.example.com
      cloudflare.tunnel.service: http://app:80
```

Start or recreate the container, then the controller will discover the labels and reconcile the Cloudflare Tunnel configuration.

## Safer first run

For a first deployment, use dry-run mode:

```yaml
environment:
  SYNC_DRY_RUN: "true"
```

Dry-run mode logs the planned changes without applying them to Cloudflare.
