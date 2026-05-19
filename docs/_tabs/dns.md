---
title: DNS Synchronization
icon: fas fa-globe
order: 4
toc: true
---

# DNS Synchronization

Docker Cloudflare Tunnel Sync can optionally manage Cloudflare DNS records for hostnames declared by Docker labels.

DNS synchronization is disabled by default. Enable it with `SYNC_MANAGED_DNS=true`.

## What it manages

For each managed tunnel hostname, the controller can create or update a Cloudflare DNS CNAME record pointing to the Cloudflare Tunnel target.

The hostname comes from `cloudflare.tunnel.hostname` or from additional suffix routes.

## Zone selection

By default, the controller derives the Cloudflare zone from the hostname.

If Cloudflare manages a delegated sub-zone, set the DNS zone explicitly with `cloudflare.tunnel.dns.zone`.

For suffix routes, use `cloudflare.tunnel.dns.zone.<suffix>`.

## DNS zone label vs SYNC_DNS_ZONES

| Setting | Purpose |
| --- | --- |
| `cloudflare.tunnel.dns.zone` | Selects the Cloudflare zone for a specific hostname. |
| `SYNC_DNS_ZONES` | Keeps whole zones in the DNS reconciliation scope. |
