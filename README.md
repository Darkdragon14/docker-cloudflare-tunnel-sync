# docker-cloudflare-tunnel-sync

Automatically reconcile Cloudflare Tunnel ingress routes from Docker container labels. Containers opt in via explicit, namespaced labels and remain the single source of truth for route definitions.

## Why Go

Go provides a stable Docker SDK, strong concurrency primitives for controller loops, and a single static binary that is easy to distribute in homelab and production environments.

## Architecture

The project follows a controller pattern with clear separation of concerns:

- **Docker adapter**: read-only access to running containers and labels.
- **Label parser**: validates Cloudflare-specific labels and produces desired ingress and Access definitions.
- **Cloudflare API client**: reads and updates tunnel configurations plus Access apps/policies and DNS records.
- **Reconciliation engines**: compare desired vs actual state for ingress, Access, and DNS.
- **Controller loop**: polls Docker at a fixed interval and triggers reconciliation.

## Project structure

```
cmd/docker-cloudflare-tunnel-sync/
  main.go
internal/cloudflare/
  client.go
  types.go
internal/config/
  config.go
internal/controller/
  controller.go
internal/dns/
  engine.go
internal/docker/
  adapter.go
  types.go
internal/labels/
  parser.go
internal/model/
  access.go
  managed.go
  ownership.go
  route.go
internal/reconcile/
  engine.go
```

## Configuration (environment variables)

| Variable | Required | Default | Description |
| --- | --- | --- | --- |
| `CF_API_TOKEN` | yes | - | Cloudflare API token with Account permissions (`Cloudflare Tunnel:Edit`, plus `Access Apps and Policies:Edit` for Access labels) and Zone permissions (`Zone:Read` + `DNS:Edit` for DNS automation). |
| `CF_ACCOUNT_ID` | yes | - | Cloudflare account identifier. |
| `CF_TUNNEL_ID` | yes | - | Cloudflare Tunnel identifier. |
| `CF_API_BASE_URL` | no | `https://api.cloudflare.com/client/v4` | Override Cloudflare API base URL. |
| `DOCKER_HOST` | no | - | Docker daemon host (standard Docker env var). |
| `DOCKER_API_VERSION` | no | - | Docker API version override. |
| `SYNC_POLL_INTERVAL` | no | `30s` | Controller poll interval. |
| `SYNC_RUN_ONCE` | no | `false` | Run a single reconciliation and exit. |
| `SYNC_DRY_RUN` | no | `false` | Log changes without applying them. |
| `SYNC_MANAGED_TUNNEL` | no | `false` | Allow this tool to overwrite the tunnel ingress configuration. |
| `SYNC_MANAGED_ACCESS` | no | `false` | Allow this tool to create/update Access apps and policies. |
| `SYNC_MANAGED_DNS` | no | `false` | Allow this tool to create/update DNS CNAME records for tunnel hostnames. |
| `SYNC_DELETE_DNS` | no | `false` | Delete managed DNS records when hostnames are no longer labeled. |
| `SYNC_MANAGED_BY` | no | `docker-cf-tunnel-sync` | Override the managed-by tag/comment value (used for Access tags and DNS comments). |
| `LOG_LEVEL` | no | `info` | `debug`, `info`, `warn`, or `error`. |

## Usage

Build a local image:

```
docker build -t docker-cloudflare-tunnel-sync:local .
```

Pull the published image from GitHub Container Registry:

```
docker pull ghcr.io/<owner>/docker-cloudflare-tunnel-sync:latest
```

Run with Docker (read-only socket mount):

```
docker run --rm \
  -e CF_API_TOKEN=your-token \
  -e CF_ACCOUNT_ID=your-account-id \
  -e CF_TUNNEL_ID=your-tunnel-id \
  -e SYNC_MANAGED_TUNNEL=true \
  -e SYNC_MANAGED_ACCESS=true \
  -e SYNC_MANAGED_DNS=true \
  -e SYNC_DELETE_DNS=true \
  -e SYNC_POLL_INTERVAL=30s \
  -v /var/run/docker.sock:/var/run/docker.sock:ro \
  docker-cloudflare-tunnel-sync:local
```

Docker Compose example:

```
services:
  tunnel-sync:
    image: docker-cloudflare-tunnel-sync:local
    environment:
      CF_API_TOKEN: your-token
      CF_ACCOUNT_ID: your-account-id
      CF_TUNNEL_ID: your-tunnel-id
      SYNC_MANAGED_TUNNEL: "true"
      SYNC_MANAGED_ACCESS: "true"
      SYNC_MANAGED_DNS: "true"
      SYNC_DELETE_DNS: "true"
      SYNC_POLL_INTERVAL: 30s
    volumes:
      - /var/run/docker.sock:/var/run/docker.sock:ro
```

## Cloudflare prerequisites

`cloudflared` is responsible for keeping the tunnel connection alive, but it does not offer a local API to manage remote ingress rules or Cloudflare Access policies. This controller uses the Cloudflare `/configurations` endpoint to reconcile ingress rules and requires explicit permission via `SYNC_MANAGED_TUNNEL=true` to overwrite the tunnel's ingress list.

1. **Create or identify a tunnel**
   - `cloudflared login`
   - `cloudflared tunnel create <name>`
   - `cloudflared tunnel list` (copy the Tunnel ID)
2. **Find the Account ID**
   - Cloudflare dashboard → Account Home → Account ID
   - Or read it from the tunnel credentials JSON (`account_tag` field).
3. **Create an API token**
   - Cloudflare dashboard → My Profile → API Tokens → Create Token
   - Account permissions:
     - `Cloudflare Tunnel` → `Edit`
     - `Access Apps and Policies` → `Edit` (if using Access labels)
   - Zone permissions (only for DNS automation):
     - `Zone` → `Read`
     - `DNS` → `Edit`

If you already run a `cloudflared` container, the credentials file is typically mounted under `/etc/cloudflared/<tunnel-id>.json` (or the path you configured). Use the `tunnel_id` and `account_tag` fields from that file to set `CF_TUNNEL_ID` and `CF_ACCOUNT_ID`.

When DNS automation is enabled, the controller selects the zone by the longest matching suffix and creates CNAME records for each hostname. The managed tag/comment can be customized with `SYNC_MANAGED_BY`.

## Docker labels

All labels are explicit and namespaced. A container is only managed when `cloudflare.tunnel.enable=true`.

| Label | Required | Example | Description |
| --- | --- | --- | --- |
| `cloudflare.tunnel.enable` | yes | `true` | Opt-in flag for route creation. |
| `cloudflare.tunnel.hostname` | yes | `app.example.com` | Hostname for the route. |
| `cloudflare.tunnel.service` | yes | `http://api:8080` | Cloudflare service/origin URL. |
| `cloudflare.tunnel.path` | no | `/api` | Optional path prefix (must start with `/`). |

## Access labels

Access applications are only managed when `cloudflare.access.enable=true`. Policy indices (`policy.1`, `policy.2`, etc.) define evaluation order. Comma-separated lists are accepted for emails and IPs. If only `policy.N.id` is provided, the policy is referenced without updates. If `cloudflare.access.app.domain` is omitted, the controller uses `cloudflare.tunnel.hostname` and logs a warning.

| Label | Required | Example | Description |
| --- | --- | --- | --- |
| `cloudflare.access.enable` | yes | `true` | Opt-in flag for Access management. |
| `cloudflare.access.app.name` | yes | `nginx` | Access application name. |
| `cloudflare.access.app.domain` | yes* | `nginx.example.com` | Access application domain (required unless `cloudflare.tunnel.hostname` is set). |
| `cloudflare.access.app.id` | no | `app-uuid` | Optional existing app ID to update. |
| `cloudflare.access.policy.1.name` | yes* | `allow-team` | Policy name (required unless using ID-only mode). |
| `cloudflare.access.policy.1.action` | yes* | `allow` | Policy action (`allow` or `deny`, required unless using ID-only mode). |
| `cloudflare.access.policy.1.include.emails` | no | `me@example.com` | Comma-separated allowed emails. |
| `cloudflare.access.policy.1.include.ips` | no | `192.0.2.0/24` | Comma-separated allowed IPs/CIDRs. |
| `cloudflare.access.policy.1.id` | no | `policy-uuid` | Optional existing policy ID. If set without other policy fields, the policy is referenced only and not updated. |

When no app or policy ID is provided, the controller matches existing resources by name (and domain for apps). If multiple matches exist, reconciliation is skipped with a warning. If a policy ID is provided but not found in account-level policies, the controller will still attach the ID (useful for app-scoped policies).

## Reconciliation behavior

- Docker labels define the desired ingress state; there are no service configuration files.
- The controller reconciles the tunnel ingress list via the `/configurations` endpoint and appends a `http_status:404` fallback rule.
- When `SYNC_MANAGED_TUNNEL=true`, the ingress list is fully managed and any non-labeled rules are removed (warning: existing tunnel rules will be deleted).
- When the flag is `false`, differences are logged and skipped.
- Access apps/policies are reconciled when `SYNC_MANAGED_ACCESS=true` and are matched by ID or by name+domain; policy includes support emails and IPs only, and ID-only policies are never updated.
- Access apps tagged with `managed-by=<value>` are deleted when no longer defined by labels; Access policies are not deleted automatically.
- DNS records are created/updated when `SYNC_MANAGED_DNS=true` by matching the longest zone suffix; records are CNAMEs to `<tunnel-id>.cfargotunnel.com`, proxied, and only updated when already managed (comment `managed-by=<value>`) or already pointing to the tunnel. When `SYNC_DELETE_DNS=true`, managed records not backed by labels are deleted.
- Duplicate hostname/path definitions are rejected to keep outcomes deterministic.
- All operations are idempotent and safe to run continuously.

## Security and safety

- Mount the Docker socket read-only (`/var/run/docker.sock:/var/run/docker.sock:ro`).
- Scope the Cloudflare API token to `Cloudflare Tunnel:Edit` and `Access Apps and Policies:Edit` if using Access labels.
- Require `SYNC_MANAGED_TUNNEL=true` to allow ingress updates; otherwise the controller is read-only.
- Require `SYNC_MANAGED_ACCESS=true` to allow Access app/policy updates.
- Require `SYNC_MANAGED_DNS=true` to allow DNS record updates.

## Next steps

- Add Docker event-based watching for faster convergence.
- Add first-class Kubernetes provider while reusing the reconciliation engine.
