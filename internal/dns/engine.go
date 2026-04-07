package dns

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"log/slog"

	"github.com/darkdragon/docker-cloudflare-tunnel-sync/internal/cloudflare"
	"github.com/darkdragon/docker-cloudflare-tunnel-sync/internal/model"
	"golang.org/x/net/publicsuffix"
)

const (
	dnsRecordType = "CNAME"
	dnsRecordTTL  = 1
)

// Engine reconciles DNS records for tunnel hostnames.
type Engine struct {
	api             cloudflare.DNSAPI
	log             *slog.Logger
	dryRun          bool
	manage          bool
	delete          bool
	configuredZones []string
	tunnelID        string
	managedComment  string
}

func NewEngine(api cloudflare.DNSAPI, logger *slog.Logger, dryRun bool, manage bool, delete bool, configuredZones []string, tunnelID string, managedBy string) *Engine {
	return &Engine{
		api:             api,
		log:             logger,
		dryRun:          dryRun,
		manage:          manage,
		delete:          delete,
		configuredZones: append([]string(nil), configuredZones...),
		tunnelID:        tunnelID,
		managedComment:  model.DNSManagedComment(managedBy),
	}
}

type zonePlan struct {
	requiredZones   map[string]struct{}
	hostnamesByZone map[string][]string
}

type hostnameZoneState struct {
	explicitZones   map[string]struct{}
	invalidExplicit bool
}

func (engine *Engine) Reconcile(ctx context.Context, routes []model.RouteSpec) error {
	if !engine.manage {
		return nil
	}

	plan := buildZonePlan(routes, engine.log)
	selectedZones := engine.selectedZones(plan)
	if len(selectedZones) == 0 {
		engine.log.Debug("no DNS zones selected from managed hostnames or configured cleanup zones; DNS sync skipped")
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

	orderedZones := filterZones(zones, selectedZones, engine.log)
	if len(orderedZones) == 0 {
		engine.log.Warn("no matching Cloudflare zones found for managed hostnames or configured cleanup zones; DNS sync skipped")
		return nil
	}

	for _, zone := range orderedZones {
		zoneName := normalizeDNSName(zone.Name)
		knownHostnames := append([]string(nil), plan.hostnamesByZone[zoneName]...)
		if len(knownHostnames) == 0 && !engine.delete {
			continue
		}

		byName := map[string]struct{}{}
		for _, hostname := range knownHostnames {
			byName[hostname] = struct{}{}
		}

		if engine.delete {
			if len(knownHostnames) == 0 {
				engine.log.Debug("scanning configured DNS zone for orphan cleanup", "zone", zone.Name)
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

func (engine *Engine) selectedZones(plan zonePlan) map[string]struct{} {
	selected := map[string]struct{}{}
	for zone := range plan.requiredZones {
		selected[zone] = struct{}{}
	}
	if !engine.delete {
		return selected
	}

	for _, zone := range engine.configuredZones {
		normalized := normalizeDNSName(zone)
		if normalized == "" {
			continue
		}
		if _, ok := selected[normalized]; ok {
			continue
		}
		engine.log.Debug("configured DNS cleanup zone selected", "zone", normalized)
		selected[normalized] = struct{}{}
	}

	return selected
}

func buildZonePlan(routes []model.RouteSpec, logger *slog.Logger) zonePlan {
	states := map[string]*hostnameZoneState{}

	for _, route := range routes {
		hostname := normalizeDNSName(route.Key.Hostname)
		if hostname == "" {
			continue
		}

		state, ok := states[hostname]
		if !ok {
			state = &hostnameZoneState{explicitZones: map[string]struct{}{}}
			states[hostname] = state
		}

		if route.DNSZoneOverride == "" {
			continue
		}

		zone := normalizeDNSName(route.DNSZoneOverride)
		if zone == "" {
			logger.Warn("configured DNS zone override is empty; skipping hostname", "hostname", hostname)
			state.invalidExplicit = true
			continue
		}
		if !hostnameMatchesZone(hostname, zone) {
			logger.Warn("configured DNS zone override does not match hostname; skipping hostname", "hostname", hostname, "zone", zone)
			state.invalidExplicit = true
			continue
		}

		state.explicitZones[zone] = struct{}{}
	}

	plan := zonePlan{
		requiredZones:   map[string]struct{}{},
		hostnamesByZone: map[string][]string{},
	}

	for hostname, state := range states {
		if state.invalidExplicit {
			continue
		}

		zone, ok := selectZoneForHostname(hostname, state, logger)
		if !ok {
			continue
		}

		plan.requiredZones[zone] = struct{}{}
		plan.hostnamesByZone[zone] = append(plan.hostnamesByZone[zone], hostname)
	}

	for zone := range plan.hostnamesByZone {
		sort.Strings(plan.hostnamesByZone[zone])
	}

	return plan
}

func orderZones(zones []cloudflare.Zone) []cloudflare.Zone {
	ordered := make([]cloudflare.Zone, len(zones))
	copy(ordered, zones)
	sort.SliceStable(ordered, func(i, j int) bool {
		left := normalizeDNSName(ordered[i].Name)
		right := normalizeDNSName(ordered[j].Name)
		if len(left) == len(right) {
			return left < right
		}
		return len(left) > len(right)
	})
	return ordered
}

func filterZones(zones []cloudflare.Zone, requiredZones map[string]struct{}, logger *slog.Logger) []cloudflare.Zone {
	filtered := make([]cloudflare.Zone, 0, len(requiredZones))
	found := map[string]struct{}{}

	for _, zone := range zones {
		normalized := normalizeDNSName(zone.Name)
		if _, ok := requiredZones[normalized]; !ok {
			logger.Debug("skipping unrelated DNS zone", "zone", zone.Name)
			continue
		}
		filtered = append(filtered, zone)
		found[normalized] = struct{}{}
	}

	for _, zone := range missingZones(requiredZones, found) {
		logger.Warn("required DNS zone not found in accessible Cloudflare zones; skipping", "zone", zone)
	}

	return orderZones(filtered)
}

func selectZoneForHostname(hostname string, state *hostnameZoneState, logger *slog.Logger) (string, bool) {
	if len(state.explicitZones) > 1 {
		zones := make([]string, 0, len(state.explicitZones))
		for zone := range state.explicitZones {
			zones = append(zones, zone)
		}
		sort.Strings(zones)
		logger.Warn("conflicting DNS zone overrides for hostname; skipping hostname", "hostname", hostname, "zones", strings.Join(zones, ","))
		return "", false
	}

	if len(state.explicitZones) == 1 {
		for zone := range state.explicitZones {
			return zone, true
		}
	}

	zone, err := autoZoneForHostname(hostname)
	if err != nil {
		logger.Warn("failed to derive DNS zone from hostname; skipping hostname", "hostname", hostname, "error", err)
		return "", false
	}

	return zone, true
}

func autoZoneForHostname(hostname string) (string, error) {
	zone, err := publicsuffix.EffectiveTLDPlusOne(hostname)
	if err != nil {
		return "", err
	}
	return normalizeDNSName(zone), nil
}

func missingZones(requiredZones map[string]struct{}, found map[string]struct{}) []string {
	missing := make([]string, 0)
	for zone := range requiredZones {
		if _, ok := found[zone]; ok {
			continue
		}
		missing = append(missing, zone)
	}
	sort.Strings(missing)
	return missing
}

func normalizeDNSName(value string) string {
	return strings.ToLower(strings.TrimSuffix(strings.TrimSpace(value), "."))
}

func hostnameMatchesZone(hostname string, zone string) bool {
	return hostname == zone || strings.HasSuffix(hostname, "."+zone)
}

func dnsRecordEqual(record cloudflare.DNSRecord, desired cloudflare.DNSRecordInput) bool {
	return strings.EqualFold(record.Content, desired.Content) &&
		record.Proxied == desired.Proxied &&
		record.Comment == desired.Comment
}
