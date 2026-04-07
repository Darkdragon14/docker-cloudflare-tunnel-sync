package dns

import (
	"context"
	"io"
	"log/slog"
	"testing"

	"github.com/darkdragon/docker-cloudflare-tunnel-sync/internal/cloudflare"
	"github.com/darkdragon/docker-cloudflare-tunnel-sync/internal/model"
)

const testManagedBy = "test-managed"

func TestBuildZonePlanPrefersExplicitOverride(t *testing.T) {
	plan := buildZonePlan([]model.RouteSpec{
		{Key: model.RouteKey{Hostname: "app.dev.example.com"}, Service: "http://app"},
		{Key: model.RouteKey{Hostname: "app.dev.example.com", Path: "/api"}, Service: "http://app-api", DNSZoneOverride: "dev.example.com"},
	}, testLogger())

	if _, ok := plan.requiredZones["dev.example.com"]; !ok {
		t.Fatalf("expected explicit zone override to be selected, got %+v", plan.requiredZones)
	}
	if _, ok := plan.requiredZones["example.com"]; ok {
		t.Fatalf("did not expect auto-derived example.com when explicit override exists")
	}
	hosts := plan.hostnamesByZone["dev.example.com"]
	if len(hosts) != 1 || hosts[0] != "app.dev.example.com" {
		t.Fatalf("unexpected hostnames for explicit zone: %+v", hosts)
	}
}

func TestBuildZonePlanSkipsConflictingExplicitOverrides(t *testing.T) {
	plan := buildZonePlan([]model.RouteSpec{
		{Key: model.RouteKey{Hostname: "app.dev.example.com"}, Service: "http://app", DNSZoneOverride: "dev.example.com"},
		{Key: model.RouteKey{Hostname: "app.dev.example.com", Path: "/api"}, Service: "http://app-api", DNSZoneOverride: "example.com"},
	}, testLogger())

	if len(plan.requiredZones) != 0 {
		t.Fatalf("expected conflicting overrides to skip hostname, got %+v", plan.requiredZones)
	}
	if len(plan.hostnamesByZone) != 0 {
		t.Fatalf("expected no hostname plan for conflicting overrides, got %+v", plan.hostnamesByZone)
	}
}

func TestReconcileManageDisabledSkipsAPICalls(t *testing.T) {
	api := &stubDNSAPI{}
	engine := NewEngine(api, testLogger(), false, false, true, nil, "tunnel-id", testManagedBy)

	err := engine.Reconcile(context.Background(), []model.RouteSpec{{Key: model.RouteKey{Hostname: "app.example.com"}, Service: "http://app"}})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if api.listZonesCalls != 0 {
		t.Fatalf("expected no zone listing when manage is false, got %d", api.listZonesCalls)
	}
	if len(api.listDNSRecordsCalls) != 0 {
		t.Fatalf("expected no DNS record listing when manage is false, got %d", len(api.listDNSRecordsCalls))
	}
}

func TestReconcileSkipsUnrelatedZones(t *testing.T) {
	api := &stubDNSAPI{
		zones: []cloudflare.Zone{
			{ID: "zone-example-com", Name: "example.com"},
			{ID: "zone-example-org", Name: "example.org"},
			{ID: "zone-unrelated-net", Name: "unrelated.net"},
		},
	}
	engine := NewEngine(api, testLogger(), true, true, false, nil, "tunnel-id", testManagedBy)

	err := engine.Reconcile(context.Background(), []model.RouteSpec{
		{Key: model.RouteKey{Hostname: "app.example.com"}, Service: "http://app"},
		{Key: model.RouteKey{Hostname: "api.example.org"}, Service: "http://api"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	assertZoneQueried(t, api.listDNSRecordsCalls, "zone-example-com")
	assertZoneQueried(t, api.listDNSRecordsCalls, "zone-example-org")
	assertZoneNotQueried(t, api.listDNSRecordsCalls, "zone-unrelated-net")
}

func TestReconcileUsesExplicitOverrideZone(t *testing.T) {
	api := &stubDNSAPI{
		zones: []cloudflare.Zone{
			{ID: "zone-example-com", Name: "example.com"},
			{ID: "zone-dev-example-com", Name: "dev.example.com"},
		},
	}
	engine := NewEngine(api, testLogger(), true, true, false, nil, "tunnel-id", testManagedBy)

	err := engine.Reconcile(context.Background(), []model.RouteSpec{{
		Key:             model.RouteKey{Hostname: "app.dev.example.com"},
		Service:         "http://app",
		DNSZoneOverride: "dev.example.com",
	}})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	assertZoneQueried(t, api.listDNSRecordsCalls, "zone-dev-example-com")
	assertZoneNotQueried(t, api.listDNSRecordsCalls, "zone-example-com")
}

func TestReconcileSkipsHostnameWhenExplicitOverrideIsInvalid(t *testing.T) {
	api := &stubDNSAPI{
		zones: []cloudflare.Zone{{ID: "zone-example-com", Name: "example.com"}},
	}
	engine := NewEngine(api, testLogger(), true, true, false, nil, "tunnel-id", testManagedBy)

	err := engine.Reconcile(context.Background(), []model.RouteSpec{{
		Key:             model.RouteKey{Hostname: "app.example.com"},
		Service:         "http://app",
		DNSZoneOverride: "dev.example.com",
	}})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if api.listZonesCalls != 0 {
		t.Fatalf("expected no zone listing when hostname plan is invalid, got %d", api.listZonesCalls)
	}
	if len(api.listDNSRecordsCalls) != 0 {
		t.Fatalf("expected no DNS record queries when hostname plan is invalid, got %d", len(api.listDNSRecordsCalls))
	}
}

func TestReconcileDeleteOnlyTouchesSelectedZones(t *testing.T) {
	managedComment := model.DNSManagedComment(testManagedBy)
	api := &stubDNSAPI{
		zones: []cloudflare.Zone{
			{ID: "zone-example-com", Name: "example.com"},
			{ID: "zone-example-org", Name: "example.org"},
		},
		recordsByQuery: map[string][]cloudflare.DNSRecord{
			"zone-example-com|": {
				{ID: "orphan", Name: "old.example.com", Type: dnsRecordType, Comment: managedComment},
			},
			"zone-example-com|app.example.com": {
				{ID: "managed", Name: "app.example.com", Type: dnsRecordType, Content: "tunnel-id.cfargotunnel.com", Proxied: true, Comment: managedComment},
			},
			"zone-example-org|": {
				{ID: "other-orphan", Name: "old.example.org", Type: dnsRecordType, Comment: managedComment},
			},
		},
	}
	engine := NewEngine(api, testLogger(), false, true, true, nil, "tunnel-id", testManagedBy)

	err := engine.Reconcile(context.Background(), []model.RouteSpec{{Key: model.RouteKey{Hostname: "app.example.com"}, Service: "http://app"}})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(api.deleteCalls) != 1 {
		t.Fatalf("expected exactly one delete call, got %d", len(api.deleteCalls))
	}
	if api.deleteCalls[0].zoneID != "zone-example-com" {
		t.Fatalf("expected delete in example.com zone, got %+v", api.deleteCalls[0])
	}
	assertZoneNotQueried(t, api.listDNSRecordsCalls, "zone-example-org")
}

func TestReconcileDeleteScansConfiguredZonesWithoutRoutes(t *testing.T) {
	managedComment := model.DNSManagedComment(testManagedBy)
	api := &stubDNSAPI{
		zones: []cloudflare.Zone{{ID: "zone-darkdragon-fr", Name: "darkdragon.fr"}},
		recordsByQuery: map[string][]cloudflare.DNSRecord{
			"zone-darkdragon-fr|": {
				{ID: "orphan", Name: "test-cf.darkdragon.fr", Type: dnsRecordType, Comment: managedComment},
			},
		},
	}
	engine := NewEngine(api, testLogger(), false, true, true, []string{"darkdragon.fr"}, "tunnel-id", testManagedBy)

	err := engine.Reconcile(context.Background(), nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if api.listZonesCalls != 1 {
		t.Fatalf("expected configured cleanup zones to trigger zone listing, got %d", api.listZonesCalls)
	}
	if len(api.deleteCalls) != 1 {
		t.Fatalf("expected configured cleanup zone to delete orphan record, got %d", len(api.deleteCalls))
	}
	assertZoneQueried(t, api.listDNSRecordsCalls, "zone-darkdragon-fr")
	assertZoneNotQueriedForName(t, api.listDNSRecordsCalls, "zone-darkdragon-fr", "test-cf.darkdragon.fr")
}

func TestReconcileConfiguredZonesIgnoredWhenDeleteDisabled(t *testing.T) {
	api := &stubDNSAPI{}
	engine := NewEngine(api, testLogger(), false, true, false, []string{"darkdragon.fr"}, "tunnel-id", testManagedBy)

	err := engine.Reconcile(context.Background(), nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if api.listZonesCalls != 0 {
		t.Fatalf("expected no zone listing when only cleanup zones are configured and delete is false, got %d", api.listZonesCalls)
	}
	if len(api.listDNSRecordsCalls) != 0 {
		t.Fatalf("expected no DNS record queries when delete is false, got %d", len(api.listDNSRecordsCalls))
	}
}

func TestReconcileDeleteIncludesConfiguredCleanupZones(t *testing.T) {
	managedComment := model.DNSManagedComment(testManagedBy)
	api := &stubDNSAPI{
		zones: []cloudflare.Zone{
			{ID: "zone-example-com", Name: "example.com"},
			{ID: "zone-darkdragon-fr", Name: "darkdragon.fr"},
		},
		recordsByQuery: map[string][]cloudflare.DNSRecord{
			"zone-example-com|": {
				{ID: "managed", Name: "app.example.com", Type: dnsRecordType, Comment: managedComment},
			},
			"zone-example-com|app.example.com": {
				{ID: "managed", Name: "app.example.com", Type: dnsRecordType, Content: "tunnel-id.cfargotunnel.com", Proxied: true, Comment: managedComment},
			},
			"zone-darkdragon-fr|": {
				{ID: "orphan", Name: "test-cf.darkdragon.fr", Type: dnsRecordType, Comment: managedComment},
			},
		},
	}
	engine := NewEngine(api, testLogger(), false, true, true, []string{"darkdragon.fr"}, "tunnel-id", testManagedBy)

	err := engine.Reconcile(context.Background(), []model.RouteSpec{{Key: model.RouteKey{Hostname: "app.example.com"}, Service: "http://app"}})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertZoneQueried(t, api.listDNSRecordsCalls, "zone-example-com")
	assertZoneQueried(t, api.listDNSRecordsCalls, "zone-darkdragon-fr")
	if len(api.deleteCalls) != 1 || api.deleteCalls[0].zoneID != "zone-darkdragon-fr" {
		t.Fatalf("expected configured cleanup zone orphan to be deleted, got %+v", api.deleteCalls)
	}
}

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

type dnsListCall struct {
	zoneID string
	name   string
}

type dnsDeleteCall struct {
	zoneID   string
	recordID string
}

type stubDNSAPI struct {
	zones               []cloudflare.Zone
	recordsByQuery      map[string][]cloudflare.DNSRecord
	listZonesCalls      int
	listDNSRecordsCalls []dnsListCall
	deleteCalls         []dnsDeleteCall
}

func (api *stubDNSAPI) ListZones(ctx context.Context) ([]cloudflare.Zone, error) {
	api.listZonesCalls++
	return api.zones, nil
}

func (api *stubDNSAPI) ListDNSRecords(ctx context.Context, zoneID string, recordType string, name string) ([]cloudflare.DNSRecord, error) {
	api.listDNSRecordsCalls = append(api.listDNSRecordsCalls, dnsListCall{zoneID: zoneID, name: name})
	if api.recordsByQuery == nil {
		return nil, nil
	}
	return api.recordsByQuery[zoneID+"|"+name], nil
}

func (api *stubDNSAPI) CreateDNSRecord(ctx context.Context, zoneID string, input cloudflare.DNSRecordInput) (cloudflare.DNSRecord, error) {
	return cloudflare.DNSRecord{}, nil
}

func (api *stubDNSAPI) UpdateDNSRecord(ctx context.Context, zoneID string, recordID string, input cloudflare.DNSRecordInput) (cloudflare.DNSRecord, error) {
	return cloudflare.DNSRecord{}, nil
}

func (api *stubDNSAPI) DeleteDNSRecord(ctx context.Context, zoneID string, recordID string) error {
	api.deleteCalls = append(api.deleteCalls, dnsDeleteCall{zoneID: zoneID, recordID: recordID})
	return nil
}

func assertZoneQueried(t *testing.T, calls []dnsListCall, zoneID string) {
	t.Helper()
	for _, call := range calls {
		if call.zoneID == zoneID {
			return
		}
	}
	t.Fatalf("expected zone %s to be queried, got %+v", zoneID, calls)
}

func assertZoneNotQueried(t *testing.T, calls []dnsListCall, zoneID string) {
	t.Helper()
	for _, call := range calls {
		if call.zoneID == zoneID {
			t.Fatalf("expected zone %s not to be queried, got %+v", zoneID, calls)
		}
	}
}

func assertZoneNotQueriedForName(t *testing.T, calls []dnsListCall, zoneID string, name string) {
	t.Helper()
	for _, call := range calls {
		if call.zoneID == zoneID && call.name == name {
			t.Fatalf("expected zone %s not to be queried for %s, got %+v", zoneID, name, calls)
		}
	}
}
