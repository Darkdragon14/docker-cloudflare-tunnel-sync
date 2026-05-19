---
title: Labels Reference
icon: fas fa-tags
order: 3
toc: true
---

# Labels Reference

Docker Cloudflare Tunnel Sync only manages containers that explicitly opt in with Docker labels.

A container is ignored unless this label is present:

```yaml
cloudflare.tunnel.enable: "true"
```

## Basic route labels

| Label | Required | Description |
| --- | --- | --- |
| `cloudflare.tunnel.enable` | yes | Enables Cloudflare Tunnel management for this container. |
| `cloudflare.tunnel.hostname` | yes | Public hostname for the route. |
| `cloudflare.tunnel.service` | yes | Origin service URL used by Cloudflare Tunnel. |
| `cloudflare.tunnel.path` | no | Optional path prefix. Must start with `/`. |

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

## DNS labels

DNS synchronization is only applied when `SYNC_MANAGED_DNS=true`.

| Label | Description |
| --- | --- |
| `cloudflare.tunnel.dns.zone` | Override automatic DNS zone selection for the base route hostname. |
| `cloudflare.tunnel.dns.zone.<suffix>` | Override automatic DNS zone selection for a suffix route. |

## Origin settings labels

| Label | Description |
| --- | --- |
| `cloudflare.tunnel.origin.server-name` | Sets `originRequest.originServerName` for the base route. |
| `cloudflare.tunnel.origin.no-tls-verify` | Sets `originRequest.noTLSVerify` for the base route. |
| `cloudflare.tunnel.origin.server-name.<suffix>` | Sets SNI override for a suffix route. |
| `cloudflare.tunnel.origin.no-tls-verify.<suffix>` | Sets TLS verification behavior for a suffix route. |

Example:

```yaml
labels:
  cloudflare.tunnel.enable: "true"
  cloudflare.tunnel.hostname: app.example.test
  cloudflare.tunnel.service: https://app:443
  cloudflare.tunnel.origin.server-name: app.internal
  cloudflare.tunnel.origin.no-tls-verify: "true"
```
