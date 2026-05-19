---
layout: page
title: Cloudflare Access
permalink: /access/
---

# Cloudflare Access

Docker Cloudflare Tunnel Sync can optionally manage Cloudflare Access applications and policies from Docker labels.

Access synchronization is disabled by default.

Enable it with `SYNC_MANAGED_ACCESS=true`.

A container must also opt in with `cloudflare.access.enable=true`.

## Application labels

| Label | Required | Description |
| --- | --- | --- |
| `cloudflare.access.enable` | yes | Enables Access management for the container. |
| `cloudflare.access.app.name` | yes | Access application name. |
| `cloudflare.access.app.domain` | no | Access application domain. Defaults to the tunnel hostname when omitted. |
| `cloudflare.access.app.id` | no | Existing Access application ID to update. |
| `cloudflare.access.app.tags` | no | Comma-separated Access app tags. |

When app tags are set, the controller ensures they exist and manages the application tag list.

When app tags are omitted, existing tags are preserved.

## Policy labels

Policies use ordered indices such as `policy.1`, `policy.2`, and `policy.3`.

| Label | Required | Description |
| --- | --- | --- |
| `cloudflare.access.policy.N.name` | yes* | Policy name. Required unless using ID-only reference. |
| `cloudflare.access.policy.N.action` | yes* | Policy action such as `allow` or `deny`. |
| `cloudflare.access.policy.N.id` | no | Existing policy ID. |

The numeric index defines policy evaluation order.

Additional include rules are supported by the controller and are documented in the README while the v1 documentation is being migrated.

## Reference-only policies

If only `policy.N.id` or `policy.N.name` is provided, the policy is referenced without updates.

This is useful when you want the controller to attach an existing Cloudflare Access policy but not manage its content.

## Matching behavior

When no app or policy ID is provided, the controller matches existing resources by name.

For Access applications, the domain is also considered.

If multiple matches exist, reconciliation is skipped with a warning.

If a policy ID is provided but not found in account-level policies, the controller can still attach the ID. This is useful for app-scoped policies.

## Token permissions

Cloudflare Access synchronization requires the account-level permission for Access applications and policies with edit access.
