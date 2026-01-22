# Agent notes

- Use Go 1.24+ for builds (see `go.mod`).
- Run `gofmt` on any Go files you touch.
- Keep labels explicit and namespaced; update `README.md` when label semantics change.
- Preserve the controller pattern: Docker adapter, label parser, Cloudflare client, reconciler.
- Avoid hidden defaults or implicit behavior; log warnings for skipped actions.
- Never add write access to the Docker socket without explicit user request.

## Project notes

- Go provides a stable Docker SDK, strong concurrency primitives for controller loops, and a single static binary that is easy to distribute.
- Architecture follows a controller pattern with clear separation of concerns:
  - **Docker adapter**: read-only access to running containers and labels.
  - **Label parser**: validates Cloudflare-specific labels and produces desired ingress and Access definitions.
  - **Cloudflare API client**: reads and updates tunnel configurations plus Access apps/policies and DNS records.
  - **Reconciliation engines**: compare desired vs actual state for ingress, Access, and DNS.
  - **Controller loop**: polls Docker at a fixed interval and triggers reconciliation.
- Project structure:
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
- Reconciliation behavior:
  - Docker labels define the desired ingress state; there are no service configuration files.
  - The controller reconciles the tunnel ingress list via the `/configurations` endpoint and appends a `http_status:404` fallback rule.
  - When `SYNC_MANAGED_TUNNEL=true`, the ingress list is fully managed and any non-labeled rules are removed (warning: existing tunnel rules will be deleted).
  - When the flag is `false`, differences are logged and skipped.
  - Access apps/policies are reconciled when `SYNC_MANAGED_ACCESS=true` and are matched by ID or by name+domain; policy includes support emails and IPs only, and ID-only policies are never updated.
  - Access apps tagged with `managed-by=<value>` are deleted when no longer defined by labels; Access policies are not deleted automatically.
  - DNS records are created/updated when `SYNC_MANAGED_DNS=true` by matching the longest zone suffix; records are CNAMEs to `<tunnel-id>.cfargotunnel.com`, proxied, and only updated when already managed (comment `managed-by=<value>`) or already pointing to the tunnel. When `SYNC_DELETE_DNS=true`, managed records not backed by labels are deleted.
  - Duplicate hostname/path definitions are rejected to keep outcomes deterministic.
  - All operations are idempotent and safe to run continuously.
- Security and safety reminders:
  - Mount the Docker socket read-only (`/var/run/docker.sock:/var/run/docker.sock:ro`).
  - Scope the Cloudflare API token to `Cloudflare Tunnel:Edit` and `Access Apps and Policies:Edit` if using Access labels.
  - Require `SYNC_MANAGED_TUNNEL=true` to allow ingress updates; otherwise the controller is read-only.
  - Require `SYNC_MANAGED_ACCESS=true` to allow Access app/policy updates.
  - Require `SYNC_MANAGED_DNS=true` to allow DNS record updates.
- Next steps:
  - Add Docker event-based watching for faster convergence.
  - Add first-class Kubernetes provider while reusing the reconciliation engine.
