---
layout: page
title: DNS Synchronization
permalink: /dns/
---

# DNS Synchronization

Docker Cloudflare Tunnel Sync can optionally manage Cloudflare DNS records for hostnames declared by Docker labels.

DNS synchronization is disabled by default.

Enable it with `SYNC_MANAGED_DNS=true`.

## What it manages

For each managed tunnel hostname, the controller can create or update a Cloudflare DNS CNAME record pointing to the Cloudflare Tunnel target.

The hostname comes from `cloudflare.tunnel.hostname` or from additional suffix routes such as `cloudflare.tunnel.hostname.admin`.

## Zone selection

By default, the controller derives the Cloudflare zone from the hostname.

If Cloudflare manages a delegated sub-zone, set the DNS zone explicitly with `cloudflare.tunnel.dns.zone`.

For suffix routes, use `cloudflare.tunnel.dns.zone.<suffix>`.

## Cleanup behavior

DNS cleanup is disabled by default.

Enable it with `SYNC_DELETE_DNS=true`.

When enabled, the controller deletes managed DNS records that are no longer declared by Docker labels.

The cleanup scan is scoped. It does not scan every zone in your Cloudflare account.

The controller scans:

- zones selected by current Docker labels;
- zones listed in `SYNC_DNS_ZONES`.

## Keeping zones in cleanup scope

`SYNC_DNS_ZONES` keeps zones in the cleanup scan even when no current labels point to them.

This is useful when an entire zone disappears from labels but you still want old managed DNS records to be removed.

## DNS zone label vs SYNC_DNS_ZONES

| Setting | Purpose |
| --- | --- |
| `cloudflare.tunnel.dns.zone` | Selects the Cloudflare zone for a specific hostname. |
| `SYNC_DNS_ZONES` | Keeps whole zones in the orphan cleanup scan. |

Use route labels to control where records are created.

Use `SYNC_DNS_ZONES` to control which zones remain eligible for cleanup.

## Token permissions

DNS synchronization requires these Cloudflare API token permissions:

| Scope | Resource | Access |
| --- | --- | --- |
| Zone | Zone | Read |
| Zone | DNS | Edit |

If your token only has access to specific zones, the controller skips unrelated zones instead of trying to manage the entire account.
