---
title: Configuration
icon: fas fa-sliders
order: 2
toc: true
---

# Configuration

Docker Cloudflare Tunnel Sync is configured with environment variables or Docker secrets for sensitive Cloudflare values.

## Environment variables

| Variable | Required | Default | Description |
| --- | --- | --- | --- |
| `CF_API_TOKEN` | yes | - | Cloudflare API token. Can also be provided as a Docker secret. |
| `CF_ACCOUNT_ID` | yes | - | Cloudflare account identifier. Can also be provided as a Docker secret. |
| `CF_TUNNEL_ID` | yes | - | Cloudflare Tunnel identifier. Can also be provided as a Docker secret. |
| `CF_API_BASE_URL` | no | `https://api.cloudflare.com/client/v4` | Override Cloudflare API base URL. |
| `DOCKER_HOST` | no | - | Docker daemon host. Standard Docker environment variable. |
| `DOCKER_API_VERSION` | no | - | Docker API version override. |
| `SYNC_POLL_INTERVAL` | no | `30s` | Controller poll interval. |
| `SYNC_RUN_ONCE` | no | `false` | Run a single reconciliation and exit. |
| `SYNC_DRY_RUN` | no | `false` | Log planned changes without applying them. |
| `SYNC_MANAGED_TUNNEL` | no | `false` | Allow the controller to overwrite the tunnel ingress configuration. |
| `SYNC_MANAGED_ACCESS` | no | `false` | Allow the controller to create or update Cloudflare Access apps and policies. |
| `SYNC_MANAGED_DNS` | no | `false` | Allow the controller to create or update DNS CNAME records for tunnel hostnames. |
| `SYNC_DNS_ZONES` | no | - | Comma-separated DNS zones kept in the orphan cleanup scan when `SYNC_DELETE_DNS=true`. |
| `SYNC_DELETE_DNS` | no | `false` | Delete managed DNS records in selected zones when they are no longer declared by labels. |
| `SYNC_MANAGED_BY` | no | `docker-cf-tunnel-sync` | Managed-by marker used for DNS comments and Access tags. |
| `LOG_LEVEL` | no | `info` | Log level: `debug`, `info`, `warn`, or `error`. |

## Managed modes

The controller is conservative by default. It only applies a category of changes when the matching managed flag is enabled.

| Flag | Cloudflare resources managed |
| --- | --- |
| `SYNC_MANAGED_TUNNEL=true` | Cloudflare Tunnel ingress rules |
| `SYNC_MANAGED_DNS=true` | DNS CNAME records for managed hostnames |
| `SYNC_MANAGED_ACCESS=true` | Cloudflare Access applications and policies |

## Dry-run mode

Dry-run mode logs planned changes without applying them:

```bash
-e SYNC_DRY_RUN=true
```

Use dry-run mode for first deployments, label validation, audits, and cleanup checks.

## Run once mode

Run once mode performs a single reconciliation and exits:

```bash
-e SYNC_RUN_ONCE=true
```

## DNS cleanup scope

When `SYNC_DELETE_DNS=true`, the controller deletes managed DNS records that are no longer declared by Docker labels.

The cleanup scan is limited to zones selected from current labels plus any zones listed in `SYNC_DNS_ZONES`.

## Logging

Set the log level with `LOG_LEVEL`:

```bash
-e LOG_LEVEL=debug
```
