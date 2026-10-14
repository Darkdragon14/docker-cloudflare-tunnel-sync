---
title: Labels Reference
icon: fas fa-tags
order: 3
toc: true
---
{% raw %}

Docker Cloudflare Tunnel Sync only manages containers that explicitly opt in with Docker labels.

A container is ignored unless this label is present:

```yaml
cloudflare.tunnel.enable: "true"
```

## Basic route labels

| Label | Required | Description |
| --- | --- | --- |
| `cloudflare.tunnel.enable` | yes | Enables Cloudflare Tunnel management for this container. |
| `cloudflare.tunnel.hostname` | yes* | Public hostname for the route, or a comma-separated list of hostnames that share the service. *Optional when `SYNC_DEFAULT_HOSTNAME` is set. |
| `cloudflare.tunnel.service` | yes* | Origin service URL used by Cloudflare Tunnel. *Optional when `SYNC_AUTO_SERVICE=true` (derived from the exposed port). |
| `cloudflare.tunnel.port` | no | Port used by the default service template instead of the auto-detected exposed port. |
| `cloudflare.tunnel.network` | no | Network used for `{{ .IP }}` in the default service template. |
| `cloudflare.tunnel.path` | no | Optional path prefix. Must start with `/`. |

Multiple hostnames for the same service can be given in one label as a comma/space separated list or as Traefik-style ``Host(`...`)`` rules. Each hostname becomes its own route (and DNS record). Repeating the same label key does **not** work: Docker labels are a map, so the last value wins.

```yaml
labels:
  - "cloudflare.tunnel.enable=true"
  - "cloudflare.tunnel.hostname=git.ywfl.dev,git.yawaflua.tech"
  - "cloudflare.tunnel.service=http://gitea:3000"
```

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

## Default rules

Like Traefik's `providers.docker.defaultRule`, the base hostname and service can be derived from templates when the labels are omitted. Explicit labels always win. Containers still need `cloudflare.tunnel.enable=true`.

```yaml
services:
  docker-cloudflare-tunnel-sync:
    environment:
      SYNC_DEFAULT_HOSTNAME: 'Host(`{{ index .Labels "com.docker.compose.service" }}.dev.example.com`) || Host(`{{ index .Labels "com.docker.compose.service" }}.example.com`)'
      SYNC_AUTO_SERVICE: "true"
      # optional, this is the built-in default:
      SYNC_DEFAULT_SERVICE: 'http://{{ .Name }}:{{ .Port }}'

  app:
    image: nginx            # EXPOSE 80 in the Dockerfile
    labels:
      cloudflare.tunnel.enable: "true"
    # -> app.dev.example.com and app.example.com -> http://<project>-app-1:80
```

`SYNC_DEFAULT_HOSTNAME` may render a single hostname, a comma/space separated list, or Traefik-style ``Host(`...`)`` expressions joined with `||`. Every hostname becomes its own route with the same service.

Template fields:

| Field | Description |
| --- | --- |
| `.Name` | Container name (resolvable through Docker DNS on shared networks). |
| `.Service` | `com.docker.compose.service` label, or the container name. |
| `.Project` | `com.docker.compose.project` label. |
| `.ID` | Container ID. |
| `.Labels` | All container labels (`{{ index .Labels "key" }}`). |
| `.Port` | Service port (service template only): `cloudflare.tunnel.port`, otherwise the lowest exposed TCP port (Dockerfile `EXPOSE`, compose `expose`/`ports`). |
| `.IP` / `.Network` | Container IP on `cloudflare.tunnel.network`, `SYNC_DOCKER_NETWORK` (compose `<project>_` prefix is tried too), or the first attached network by name. |

Template functions: `lower`, `upper`, `normalize`, `replace OLD NEW S`, `trimPrefix P S`, `trimSuffix X S`.

Service auto-detection is opt-in: without `SYNC_AUTO_SERVICE=true` the `cloudflare.tunnel.service` label stays required.

Use `SYNC_DEFAULT_SERVICE='http://{{ .IP }}:{{ .Port }}'` together with `SYNC_DOCKER_NETWORK` to route by network IP instead of container name. Make sure `cloudflared` is attached to the same network.

## Multiple routes per container

The base route hostname and service remain required for every managed container, either as labels or through default rules.

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
{% endraw %}
