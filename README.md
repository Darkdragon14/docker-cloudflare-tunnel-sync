# docker-cloudflare-tunnel-sync

Automatically reconcile Cloudflare Tunnel ingress routes from Docker container labels. Containers opt in via explicit, namespaced labels and remain the single source of truth for route definitions.

## Why Go

Go provides a stable Docker SDK, strong concurrency primitives for controller loops, and a single static binary that is easy to distribute in homelab and production environments.

## Architecture

The project follows a controller pattern with clear separation of concerns:

- **Docker adapter**: read-only access to running containers and labels.
- **Label parser**: validates Cloudflare-specific labels and produces desired ingress rules.
- **Cloudflare API client**: reads and updates tunnel configurations (ingress rules).
- **Reconciliation engine**: compares desired vs actual state and applies changes deterministically.
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
internal/docker/
  adapter.go
  types.go
internal/labels/
  parser.go
internal/model/
  managed.go
  route.go
internal/reconcile/
  engine.go
```

## Configuration (environment variables)

| Variable | Required | Default | Description |
| --- | --- | --- | --- |
| `CF_API_TOKEN` | yes | - | Cloudflare API token with `Cloudflare Tunnel:Edit` permission. |
| `CF_ACCOUNT_ID` | yes | - | Cloudflare account identifier. |
| `CF_TUNNEL_ID` | yes | - | Cloudflare Tunnel identifier. |
| `CF_API_BASE_URL` | no | `https://api.cloudflare.com/client/v4` | Override Cloudflare API base URL. |
| `DOCKER_HOST` | no | - | Docker daemon host (standard Docker env var). |
| `DOCKER_API_VERSION` | no | - | Docker API version override. |
| `SYNC_POLL_INTERVAL` | no | `30s` | Controller poll interval. |
| `SYNC_RUN_ONCE` | no | `false` | Run a single reconciliation and exit. |
| `SYNC_DRY_RUN` | no | `false` | Log changes without applying them. |
| `SYNC_MANAGED_TUNNEL` | no | `false` | Allow this tool to overwrite the tunnel ingress configuration. |
| `LOG_LEVEL` | no | `info` | `debug`, `info`, `warn`, or `error`. |

## Usage

Build a local image:

```
docker build -t docker-cloudflare-tunnel-sync:local .
```

Run with Docker (read-only socket mount):

```
docker run --rm \
  -e CF_API_TOKEN=your-token \
  -e CF_ACCOUNT_ID=your-account-id \
  -e CF_TUNNEL_ID=your-tunnel-id \
  -e SYNC_MANAGED_TUNNEL=true \
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
   - Minimum permission: `Account` → `Cloudflare Tunnel` → `Edit`

If you already run a `cloudflared` container, the credentials file is typically mounted under `/etc/cloudflared/<tunnel-id>.json` (or the path you configured). Use the `tunnel_id` and `account_tag` fields from that file to set `CF_TUNNEL_ID` and `CF_ACCOUNT_ID`.

## Docker labels

All labels are explicit and namespaced. A container is only managed when `cloudflare.tunnel.enable=true`.

| Label | Required | Example | Description |
| --- | --- | --- | --- |
| `cloudflare.tunnel.enable` | yes | `true` | Opt-in flag for route creation. |
| `cloudflare.tunnel.hostname` | yes | `app.example.com` | Hostname for the route. |
| `cloudflare.tunnel.service` | yes | `http://api:8080` | Cloudflare service/origin URL. |
| `cloudflare.tunnel.path` | no | `/api` | Optional path prefix (must start with `/`). |

## Reconciliation behavior

- Docker labels define the desired ingress state; there are no service configuration files.
- The controller reconciles the tunnel ingress list via the `/configurations` endpoint and appends a `http_status:404` fallback rule.
- When `SYNC_MANAGED_TUNNEL=true`, the ingress list is fully managed and any non-labeled rules are removed.
- When the flag is `false`, differences are logged and skipped.
- Duplicate hostname/path definitions are rejected to keep outcomes deterministic.
- All operations are idempotent and safe to run continuously.

## Security and safety

- Mount the Docker socket read-only (`/var/run/docker.sock:/var/run/docker.sock:ro`).
- Scope the Cloudflare API token to `Cloudflare Tunnel:Edit`.
- Require `SYNC_MANAGED_TUNNEL=true` to allow ingress updates; otherwise the controller is read-only.

## Next steps

- Add Docker event-based watching for faster convergence.
- Add first-class Kubernetes provider while reusing the reconciliation engine.
