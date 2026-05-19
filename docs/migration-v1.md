---
layout: page
title: Migration to v1.0
permalink: /migration-v1/
---

# Migration to v1.0

Docker Cloudflare Tunnel Sync is being prepared for a stable v1.0 release.

The goal is to keep the existing behavior understandable while making the documentation, labels, and safety model clearer.

## Goals

The v1.0 documentation focuses on:

- a clearer project pitch;
- stable label examples;
- safer onboarding;
- better separation between basic and advanced features;
- clearer DNS and Access behavior;
- a shorter README that links to the full documentation.

## Recommended v1 label style

The basic route format remains simple:

```yaml
labels:
  cloudflare.tunnel.enable: "true"
  cloudflare.tunnel.hostname: app.example.test
  cloudflare.tunnel.service: http://app:80
```

This is the recommended starting point for v1.0.

## Multiple routes

Additional routes should use suffix-based labels:

```yaml
labels:
  cloudflare.tunnel.hostname.admin: admin.example.test
  cloudflare.tunnel.service.admin: http://app:8080
```

A suffix route is only valid when both the hostname and service labels are present.

## Recommended rollout

1. Update the controller image.
2. Run once with dry-run mode enabled.
3. Check the planned tunnel, DNS, and Access changes.
4. Disable dry-run mode.
5. Enable DNS or Access synchronization only if needed.

## README transition

During the documentation migration, the README remains the complete historical reference.

Once the documentation site is complete, the README can become a shorter landing page with:

- project pitch;
- quick start;
- safety warning;
- documentation links;
- release and container badges.
