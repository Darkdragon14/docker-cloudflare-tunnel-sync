---
layout: page
title: Cloudflare Access
permalink: /access/
icon: fas fa-shield-halved
order: 5
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

## Application labels

| Label | Required | Description |
| --- | --- | --- |
| `cloudflare.access.enable` | yes | Enables Access management for the container. |
| `cloudflare.access.app.name` | yes | Access application name. |
| `cloudflare.access.app.domain` | no | Access application domain. Defaults to the tunnel hostname when omitted. |
| `cloudflare.access.app.id` | no | Existing Access application ID to update. |
| `cloudflare.access.app.tags` | no | Comma-separated Access app tags. |

## Policy labels

Policies use ordered indices such as `policy.1`, `policy.2`, and `policy.3`.

| Label | Required | Description |
| --- | --- | --- |
| `cloudflare.access.policy.N.name` | yes* | Policy name. Required unless using ID-only reference. |
| `cloudflare.access.policy.N.action` | yes* | Policy action, for example allow or deny. |
| `cloudflare.access.policy.N.id` | no | Existing policy ID. |

## Reference-only policies

If only `policy.N.id` or `policy.N.name` is provided, the policy is referenced without updates.

This is useful when you want the controller to attach an existing Cloudflare Access policy but not manage its content.
