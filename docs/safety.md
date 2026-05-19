---
layout: page
title: Safety Model
permalink: /safety/
---

# Safety Model

Docker Cloudflare Tunnel Sync is designed to be explicit and predictable.

It does not manage every Docker container automatically. A service must opt in with labels before it is considered by the controller.

## Opt-in by label

A service is managed only when the tunnel enable label is set to true.

This keeps unrelated containers outside of the synchronization scope.

## Dedicated tunnel

Use a dedicated Cloudflare Tunnel for routes managed by this controller.

This keeps generated routes separated from routes you maintain manually.

## Progressive enablement

The controller separates tunnel, DNS, and Access synchronization behind different settings.

This allows you to start with the smallest useful setup and enable additional features later.

## Preview changes first

Before applying changes in production, run the controller in dry-run mode.

Dry-run mode prints the planned changes without applying them.

## Recommended rollout

1. Start with a dedicated tunnel.
2. Enable dry-run mode.
3. Add labels to one test service.
4. Review the planned changes.
5. Disable dry-run mode when the output looks correct.
6. Add DNS and Access synchronization only when needed.
