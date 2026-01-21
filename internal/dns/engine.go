package dns

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"log/slog"

	"github.com/darkdragon/docker-cloudflare-tunnel-sync/internal/cloudflare"
	"github.com/darkdragon/docker-cloudflare-tunnel-sync/internal/model"
)

const (
	dnsRecordType = "CNAME"
	dnsRecordTTL  = 1
)

// Engine reconciles DNS records for tunnel hostnames.
type Engine struct {
	api            cloudflare.DNSAPI
	log            *slog.Logger
	dryRun         bool
	manage         bool
	delete         bool
	tunnelID       string
	managedComment string
}

func NewEngine(api cloudflare.DNSAPI, logger *slog.Logger, dryRun bool, manage bool, delete bool, tunnelID string, managedBy string) *Engine {
	return &Engine{
		api:            api,
		log:            logger,
		dryRun:         dryRun,
		manage:         manage,
		delete:         delete,
		tunnelID:       tunnelID,
		managedComment: model.DNSManagedComment(managedBy),
	}
}

func (engine *Engine) Reconcile(ctx context.Context, routes []model.RouteSpec) error {
	if !engine.manage {
		return nil
	}

	hostnames := uniqueHostnames(routes)
	if len(hostnames) == 0 && !engine.delete {
		return nil
	}

	zones, err := engine.api.ListZones(ctx)
	if err != nil {
		return err
	}
	if len(zones) == 0 {
		engine.log.Warn("no zones returned for account; DNS sync skipped")
		return nil
	}

	orderedZones := orderZones(zones)
	for _, zone := range orderedZones {
		knownHostnames := filterHostnamesForZone(hostnames, zone.Name)
		byName := map[string]struct{}{}
		for _, hostname := range knownHostnames {
			byName[hostname] = struct{}{}
		}

		records, err := engine.api.ListDNSRecords(ctx, zone.ID, dnsRecordType, "")
		if err != nil {
			engine.log.Error("failed to list DNS records", "zone", zone.Name, "error", err)
			continue
		}

		for _, record := range records {
			hostname := strings.ToLower(strings.TrimSuffix(record.Name, "."))
			if _, ok := byName[hostname]; ok {
				continue
			}
			if !engine.delete {
				continue
			}
			if record.Comment != engine.managedComment {
				continue
			}
			engine.log.Warn("deleting managed DNS record no longer desired", "hostname", hostname, "zone", zone.Name)
			if engine.dryRun {
				continue
			}
			if err := engine.api.DeleteDNSRecord(ctx, zone.ID, record.ID); err != nil {
				engine.log.Error("failed to delete DNS record", "hostname", hostname, "zone", zone.Name, "error", err)
			}
		}

		for _, hostname := range knownHostnames {
			records, err := engine.api.ListDNSRecords(ctx, zone.ID, dnsRecordType, hostname)
			if err != nil {
				engine.log.Error("failed to list DNS records", "hostname", hostname, "zone", zone.Name, "error", err)
				continue
			}
			if len(records) > 1 {
				engine.log.Warn("multiple DNS records found; skipping", "hostname", hostname, "zone", zone.Name)
				continue
			}

			desired := cloudflare.DNSRecordInput{
				Type:    dnsRecordType,
				Name:    hostname,
				Content: engine.tunnelTarget(),
				Proxied: true,
				TTL:     dnsRecordTTL,
				Comment: engine.managedComment,
			}

			if len(records) == 0 {
				engine.log.Info("creating DNS record", "hostname", hostname, "zone", zone.Name)
				if engine.dryRun {
					continue
				}
				_, err := engine.api.CreateDNSRecord(ctx, zone.ID, desired)
				if err != nil {
					engine.log.Error("failed to create DNS record", "hostname", hostname, "zone", zone.Name, "error", err)
				}
				continue
			}

			record := records[0]
			if record.Type != dnsRecordType {
				engine.log.Warn("existing DNS record has non-CNAME type; skipping", "hostname", hostname, "zone", zone.Name, "type", record.Type)
				continue
			}
			if !engine.isManagedRecord(record, desired) {
				engine.log.Warn("existing DNS record is not managed; skipping", "hostname", hostname, "zone", zone.Name)
				continue
			}
			if dnsRecordEqual(record, desired) {
				engine.log.Debug("DNS record up-to-date", "hostname", hostname, "zone", zone.Name)
				continue
			}

			engine.log.Info("updating DNS record", "hostname", hostname, "zone", zone.Name)
			if engine.dryRun {
				continue
			}
			_, err = engine.api.UpdateDNSRecord(ctx, zone.ID, record.ID, desired)
			if err != nil {
				engine.log.Error("failed to update DNS record", "hostname", hostname, "zone", zone.Name, "error", err)
			}
		}
	}

	return nil
}

func (engine *Engine) tunnelTarget() string {
	return fmt.Sprintf("%s.cfargotunnel.com", engine.tunnelID)
}

func (engine *Engine) isManagedRecord(record cloudflare.DNSRecord, desired cloudflare.DNSRecordInput) bool {
	if record.Comment == engine.managedComment {
		return true
	}
	return strings.EqualFold(record.Content, desired.Content)
}

func uniqueHostnames(routes []model.RouteSpec) []string {
	seen := map[string]struct{}{}
	items := make([]string, 0, len(routes))
	for _, route := range routes {
		host := strings.TrimSpace(route.Key.Hostname)
		if host == "" {
			continue
		}
		host = strings.ToLower(strings.TrimSuffix(host, "."))
		if _, ok := seen[host]; ok {
			continue
		}
		seen[host] = struct{}{}
		items = append(items, host)
	}
	sort.Strings(items)
	return items
}

func orderZones(zones []cloudflare.Zone) []cloudflare.Zone {
	ordered := make([]cloudflare.Zone, len(zones))
	copy(ordered, zones)
	sort.SliceStable(ordered, func(i, j int) bool {
		return len(ordered[i].Name) > len(ordered[j].Name)
	})
	return ordered
}

func matchZone(hostname string, zones []cloudflare.Zone) (cloudflare.Zone, bool) {
	lower := strings.ToLower(strings.TrimSuffix(hostname, "."))
	for _, zone := range zones {
		zoneName := strings.ToLower(strings.TrimSuffix(zone.Name, "."))
		if lower == zoneName || strings.HasSuffix(lower, "."+zoneName) {
			return zone, true
		}
	}
	return cloudflare.Zone{}, false
}

func filterHostnamesForZone(hostnames []string, zoneName string) []string {
	items := []string{}
	lowerZone := strings.ToLower(strings.TrimSuffix(zoneName, "."))
	for _, hostname := range hostnames {
		lower := strings.ToLower(strings.TrimSuffix(hostname, "."))
		if lower == lowerZone || strings.HasSuffix(lower, "."+lowerZone) {
			items = append(items, lower)
		}
	}
	return items
}

func dnsRecordEqual(record cloudflare.DNSRecord, desired cloudflare.DNSRecordInput) bool {
	return strings.EqualFold(record.Content, desired.Content) &&
		record.Proxied == desired.Proxied &&
		record.Comment == desired.Comment
}
