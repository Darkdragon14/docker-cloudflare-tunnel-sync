---
title: Examples
icon: fas fa-code
order: 7
toc: true
---

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
