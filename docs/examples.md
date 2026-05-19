---
layout: page
title: Examples
permalink: /examples/
---

# Examples

This page contains common Docker Compose examples for Docker Cloudflare Tunnel Sync.

## Basic HTTP service

```yaml
services:
  whoami:
    image: traefik/whoami
    labels:
      cloudflare.tunnel.enable: "true"
      cloudflare.tunnel.hostname: whoami.example.test
      cloudflare.tunnel.service: http://whoami:80
```

## Service with a path prefix

```yaml
services:
  api:
    image: nginx
    labels:
      cloudflare.tunnel.enable: "true"
      cloudflare.tunnel.hostname: app.example.test
      cloudflare.tunnel.path: /api
      cloudflare.tunnel.service: http://api:80
```

## Multiple routes on one container

```yaml
services:
  app:
    image: nginx
    labels:
      cloudflare.tunnel.enable: "true"
      cloudflare.tunnel.hostname: app.example.test
      cloudflare.tunnel.service: http://app:80
      cloudflare.tunnel.hostname.admin: admin.example.test
      cloudflare.tunnel.service.admin: http://app:8080
```

## HTTPS origin with custom SNI

```yaml
services:
  app:
    image: nginx
    labels:
      cloudflare.tunnel.enable: "true"
      cloudflare.tunnel.hostname: app.example.test
      cloudflare.tunnel.service: https://app:443
      cloudflare.tunnel.origin.server-name: app.internal
      cloudflare.tunnel.origin.no-tls-verify: "true"
```

## DNS zone override

```yaml
services:
  app:
    image: nginx
    labels:
      cloudflare.tunnel.enable: "true"
      cloudflare.tunnel.hostname: app.dev.example.test
      cloudflare.tunnel.service: http://app:80
      cloudflare.tunnel.dns.zone: dev.example.test
```

## Controller with Docker secrets

```yaml
services:
  docker-cloudflare-tunnel-sync:
    image: ghcr.io/darkdragon14/docker-cloudflare-tunnel-sync
    secrets:
      - CF_API_TOKEN
      - CF_ACCOUNT_ID
      - CF_TUNNEL_ID
    environment:
      SYNC_MANAGED_TUNNEL: "true"
      SYNC_MANAGED_DNS: "true"
      SYNC_MANAGED_ACCESS: "false"
      SYNC_POLL_INTERVAL: 30s
    volumes:
      - /var/run/docker.sock:/var/run/docker.sock:ro

secrets:
  CF_API_TOKEN:
    file: ./secrets/cf_api_token
  CF_ACCOUNT_ID:
    file: ./secrets/cf_account_id
  CF_TUNNEL_ID:
    file: ./secrets/cf_tunnel_id
```
