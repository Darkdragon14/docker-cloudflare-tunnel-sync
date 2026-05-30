# Changelog

All notable changes to this project will be documented in this file.

This project follows semantic versioning.

## [v1.0.0] - 2026-05-30

First stable release of Docker Cloudflare Tunnel Sync.

### Highlights

- Stable Docker label format for the v1.x series.
- Cloudflare Tunnel ingress synchronization from Docker labels.
- Optional Cloudflare DNS synchronization.
- Optional Cloudflare Access application and policy synchronization.
- Multiple routes per container with suffix-based labels.
- Route-level origin settings support.
- Docker secrets support for Cloudflare credentials.
- Dry-run mode for safer first deployments and audits.
- Run-once mode for manual or scheduled reconciliation.
- Scoped DNS reconciliation to avoid account-wide cleanup.
- Documentation site based on the same theme as VolumeVault.

### Stability promise

The current label and configuration format is considered stable for the v1.x series.

Breaking label or configuration changes will be reserved for a future major version.

### Recommended upgrade notes

- Use a dedicated Cloudflare Tunnel for this controller.
- Start with `SYNC_DRY_RUN=true` before enabling live synchronization.
- Enable DNS and Access synchronization only when needed.
- Keep `SYNC_DELETE_DNS=false` until the DNS cleanup scope has been reviewed.
