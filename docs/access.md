---
layout: page
title: Cloudflare Access
permalink: /access/
---

# Cloudflare Access

Docker Cloudflare Tunnel Sync can optionally manage Cloudflare Access applications and policies from Docker labels.

Access synchronization is disabled by default. Enable it with `SYNC_MANAGED_ACCESS=true`.

A container must also opt in with `cloudflare.access.enable=true`.

## Overview

Cloudflare Access support lets a container describe both its tunnel route and the Access application that protects it.

A typical flow is:

1. the container declares a tunnel hostname;
2. the container enables Access management;
3. the controller creates or updates the Access application;
4. ordered policies are attached to the application.

This keeps the public route and its access rules close to the Docker Compose service that owns them.

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

## Minimal application example

```yaml
services:
  app:
    image: nginx
    labels:
      cloudflare.tunnel.enable: "true"
      cloudflare.tunnel.hostname: app.example.test
      cloudflare.tunnel.service: http://app:80
      cloudflare.access.enable: "true"
      cloudflare.access.app.name: app
```

In this example, the Access application domain is inferred from `cloudflare.tunnel.hostname`.

## Policy labels

Policies use ordered indices such as `policy.1`, `policy.2`, and `policy.3`.

| Label | Required | Description |
| --- | --- | --- |
| `cloudflare.access.policy.N.name` | yes* | Policy name. Required unless using ID-only reference. |
| `cloudflare.access.policy.N.action` | yes* | Policy action, for example allow or deny. |
| `cloudflare.access.policy.N.id` | no | Existing policy ID. |

The numeric index defines policy evaluation order.

Additional include rules are supported by the controller and remain documented in the README while the v1 documentation is being migrated.

## Policy example

```yaml
services:
  app:
    image: nginx
    labels:
      cloudflare.tunnel.enable: "true"
      cloudflare.tunnel.hostname: app.example.test
      cloudflare.tunnel.service: http://app:80
      cloudflare.access.enable: "true"
      cloudflare.access.app.name: app
      cloudflare.access.policy.1.name: allow-team
      cloudflare.access.policy.1.action: allow
```

## Policy ordering

Policy indices define evaluation order. For example, `policy.1` is evaluated before `policy.2`.

Use stable indices in your Compose file so the resulting Access application stays predictable over time.

## Reference-only policies

If only `policy.N.id` or `policy.N.name` is provided, the policy is referenced without updates.

This is useful when you want the controller to attach an existing Cloudflare Access policy but not manage its content.

## Matching behavior

When no app or policy ID is provided, the controller matches existing resources by name.

For Access applications, the domain is also considered.

If multiple matches exist, reconciliation is skipped with a warning.

If a policy ID is provided but not found in account-level policies, the controller can still attach the ID. This is useful for app-scoped policies.

## Tags and managed marker

When `cloudflare.access.app.tags` is set, the controller ensures the listed tags exist and applies them to the application.

The managed-by marker can also be controlled with `SYNC_MANAGED_BY`.

Changing this value may affect how existing managed resources are detected.

## Token permissions

Cloudflare Access synchronization requires the account-level permission for Access applications and policies with edit access.

If you do not enable Access synchronization, this permission is not required.
