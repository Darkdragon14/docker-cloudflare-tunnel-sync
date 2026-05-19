---
layout: page
title: Labels Reference
permalink: /labels/
---

# Labels Reference

Docker Cloudflare Tunnel Sync only manages containers that explicitly opt in with Docker labels.

A container is ignored unless this label is present:

```yaml
cloudflare.tunnel.enable: "true"
```

## Basic route labels

| Label | Required | Example | Description |
| --- | --- | --- | --- |
| `cloudflare.tunnel.enable` | yes | `true` | Enables Cloudflare Tunnel management for this container. |
| `cloudflare.tunnel.hostname` | yes | `app.example.test` | Public hostname for the route. |
| `cloudflare.tunnel.service` | yes | `http://app:8080` | Origin service URL used by Cloudflare Tunnel. |
| `cloudflare.tunnel.path` | no | `/api` | Optional path prefix. Must start with `/`. |

Example:

```yaml
services:
  app:
    image: nginx
    labels:
      cloudflare.tunnel.enable: "true"
      cloudflare.tunnel.hostname: app.example.test
      cloudflare.tunnel.service: http://app:80
```

This creates a route equivalent to:

```text
https://app.example.test -> http://app:80
```

## Multiple routes per container

The base route labels remain required for every managed container.

Additional routes can be declared with suffix-based labels:

| Label pattern | Description |
| --- | --- |
| `cloudflare.tunnel.hostname.<suffix>` | Additional route hostname. |
| `cloudflare.tunnel.service.<suffix>` | Additional route origin service URL. |
| `cloudflare.tunnel.path.<suffix>` | Optional path prefix for the additional route. |
| `cloudflare.tunnel.dns.zone.<suffix>` | Optional DNS zone override for the additional route. |
| `cloudflare.tunnel.origin.server-name.<suffix>` | Optional SNI override for the additional route. |
| `cloudflare.tunnel.origin.no-tls-verify.<suffix>` | Optional TLS verification override for the additional route. |

A suffix route is created only when both `hostname.<suffix>` and `service.<suffix>` are set.

If one is missing, the controller logs a warning and skips that suffix. Empty suffix labels, such as `cloudflare.tunnel.hostname.`, are ignored.

Example:

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

This declares two routes from the same container:

```text
https://app.example.test   -> http://app:80
https://admin.example.test -> http://app:8080
```

## DNS labels

DNS synchronization is only applied when `SYNC_MANAGED_DNS=true`.

| Label | Required | Example | Description |
| --- | --- | --- | --- |
| `cloudflare.tunnel.dns.zone` | no | `dev.example.test` | Override automatic DNS zone selection for the base route hostname. |
| `cloudflare.tunnel.dns.zone.<suffix>` | no | `dev.example.test` | Override automatic DNS zone selection for a suffix route. |

By default, DNS sync derives the target Cloudflare zone from the hostname using the effective registrable domain.

If Cloudflare manages a delegated sub-zone, set the zone explicitly:

```yaml
labels:
  cloudflare.tunnel.enable: "true"
  cloudflare.tunnel.hostname: app.dev.example.test
  cloudflare.tunnel.service: http://app:80
  cloudflare.tunnel.dns.zone: dev.example.test
```

## Origin settings labels

Origin settings allow route-level Cloudflare Tunnel origin configuration.

| Label | Required | Example | Description |
| --- | --- | --- | --- |
| `cloudflare.tunnel.origin.server-name` | no | `app.internal` | Sets `originRequest.originServerName` for the base route. |
| `cloudflare.tunnel.origin.no-tls-verify` | no | `true` | Sets `originRequest.noTLSVerify` for the base route. |
| `cloudflare.tunnel.origin.server-name.<suffix>` | no | `app.internal` | Sets SNI override for a suffix route. |
| `cloudflare.tunnel.origin.no-tls-verify.<suffix>` | no | `true` | Sets TLS verification behavior for a suffix route. |

Example:

```yaml
labels:
  cloudflare.tunnel.enable: "true"
  cloudflare.tunnel.hostname: app.example.test
  cloudflare.tunnel.service: https://app:443
  cloudflare.tunnel.origin.server-name: app.internal
  cloudflare.tunnel.origin.no-tls-verify: "true"
```

When an origin label is omitted for a managed route, the corresponding `originRequest` key is removed during reconciliation. Unmanaged `originRequest` keys are preserved.
