package reconcile

import (
	"context"
	"encoding/json"
	"log/slog"
	"testing"

	"github.com/darkdragon/docker-cloudflare-tunnel-sync/internal/cloudflare"
	"github.com/darkdragon/docker-cloudflare-tunnel-sync/internal/model"
)

func TestBuildDesiredIngress(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(testWriter{t}, nil))
	engine := NewEngine(nil, logger, false, true)

	existing := []cloudflare.IngressRule{
		{Hostname: "b.example.com", Service: "http://b1"},
		{Hostname: "b.example.com", Service: "http://b2"},
		{Hostname: "a.example.com", Path: "/app", Service: "http://a", OriginRequest: []byte(`{"noTLSVerify":true,"originServerName":"legacy.internal","httpHostHeader":"app.internal"}`)},
		{Service: model.FallbackService},
	}
	desired := []model.RouteSpec{
		{Key: model.RouteKey{Hostname: "a.example.com", Path: "/app"}, Service: "http://a"},
		{Key: model.RouteKey{Hostname: "c.example.com"}, Service: "http://c"},
	}

	desiredIngress, removed := engine.buildDesiredIngress(desired, existing)

	if len(removed) != 1 {
		t.Fatalf("expected 1 removed rule, got %d", len(removed))
	}
	if removed[0].Hostname != "b.example.com" || removed[0].Service != "http://b1" {
		t.Fatalf("unexpected removed rule: %+v", removed[0])
	}

	if len(desiredIngress) != 3 {
		t.Fatalf("expected 3 desired rules, got %d", len(desiredIngress))
	}
	if desiredIngress[0].Hostname != "a.example.com" || desiredIngress[0].Path != "/app" {
		t.Fatalf("unexpected first desired rule: %+v", desiredIngress[0])
	}
	originRequest := decodeOriginRequest(t, desiredIngress[0].OriginRequest)
	if _, ok := originRequest["noTLSVerify"]; ok {
		t.Fatalf("expected noTLSVerify to be removed when label is absent")
	}
	if _, ok := originRequest["originServerName"]; ok {
		t.Fatalf("expected originServerName to be removed when label is absent")
	}
	if originRequest["httpHostHeader"] != "app.internal" {
		t.Fatalf("expected unmanaged originRequest keys to be preserved, got %+v", originRequest)
	}
	if desiredIngress[1].Hostname != "c.example.com" {
		t.Fatalf("unexpected second desired rule: %+v", desiredIngress[1])
	}
	if desiredIngress[2].Service != model.FallbackService {
		t.Fatalf("expected fallback rule at end")
	}
}

func TestBuildDesiredIngressAppliesOriginLabels(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(testWriter{t}, nil))
	engine := NewEngine(nil, logger, false, true)

	existing := []cloudflare.IngressRule{
		{Hostname: "a.example.com", Service: "https://a", OriginRequest: []byte(`{"httpHostHeader":"app.internal"}`)},
		{Service: model.FallbackService},
	}
	originServerName := "origin.internal"
	noTLSVerify := false
	desired := []model.RouteSpec{
		{
			Key:              model.RouteKey{Hostname: "a.example.com"},
			Service:          "https://a",
			OriginServerName: &originServerName,
			NoTLSVerify:      &noTLSVerify,
		},
	}

	desiredIngress, _ := engine.buildDesiredIngress(desired, existing)
	if len(desiredIngress) != 2 {
		t.Fatalf("expected 2 desired rules, got %d", len(desiredIngress))
	}
	originRequest := decodeOriginRequest(t, desiredIngress[0].OriginRequest)
	if originRequest["originServerName"] != "origin.internal" {
		t.Fatalf("expected originServerName to be set, got %+v", originRequest)
	}
	if originRequest["noTLSVerify"] != false {
		t.Fatalf("expected noTLSVerify to be false, got %+v", originRequest)
	}
	if originRequest["httpHostHeader"] != "app.internal" {
		t.Fatalf("expected unmanaged originRequest keys to be preserved, got %+v", originRequest)
	}
}

func TestIngressEqual(t *testing.T) {
	ruleA := cloudflare.IngressRule{Hostname: "a.example.com", Service: "http://a"}
	ruleB := cloudflare.IngressRule{Hostname: "a.example.com", Service: "http://a", OriginRequest: []byte(`{"noTLSVerify":true}`)}

	if ingressEqual([]cloudflare.IngressRule{ruleA}, []cloudflare.IngressRule{ruleA}) != true {
		t.Fatalf("expected ingressEqual to return true")
	}
	if ingressEqual([]cloudflare.IngressRule{ruleA}, []cloudflare.IngressRule{ruleB}) {
		t.Fatalf("expected ingressEqual to detect origin request differences")
	}
}

func decodeOriginRequest(t *testing.T, raw json.RawMessage) map[string]any {
	t.Helper()
	if len(raw) == 0 {
		return map[string]any{}
	}
	decoded := map[string]any{}
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("failed to decode origin request JSON: %v", err)
	}
	return decoded
}

type testWriter struct {
	t *testing.T
}

func (w testWriter) Write(p []byte) (n int, err error) {
	w.t.Log(string(p))
	return len(p), nil
}

func TestEngineReconcileNoChanges(t *testing.T) {
	ctx := context.Background()
	api := &stubAPI{config: cloudflare.TunnelConfig{Ingress: []cloudflare.IngressRule{{Hostname: "a.example.com", Service: "http://a"}, {Service: model.FallbackService}}}}
	logger := slog.New(slog.NewTextHandler(testWriter{t}, nil))
	engine := NewEngine(api, logger, false, true)

	err := engine.Reconcile(ctx, []model.RouteSpec{{Key: model.RouteKey{Hostname: "a.example.com"}, Service: "http://a"}})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if api.updated {
		t.Fatalf("expected no update when ingress matches")
	}
}

func TestEngineReconcileManageDisabledSkipsUpdate(t *testing.T) {
	ctx := context.Background()
	api := &stubAPI{config: cloudflare.TunnelConfig{Ingress: []cloudflare.IngressRule{{Hostname: "a.example.com", Service: "http://a"}, {Service: model.FallbackService}}}}
	logger := slog.New(slog.NewTextHandler(testWriter{t}, nil))
	engine := NewEngine(api, logger, false, false)

	err := engine.Reconcile(ctx, []model.RouteSpec{{Key: model.RouteKey{Hostname: "b.example.com"}, Service: "http://b"}})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if api.updated {
		t.Fatalf("expected no update when manage tunnel is false")
	}
}

type stubAPI struct {
	config  cloudflare.TunnelConfig
	updated bool
}

func (api *stubAPI) GetConfig(ctx context.Context) (cloudflare.TunnelConfig, error) {
	return api.config, nil
}

func (api *stubAPI) UpdateConfig(ctx context.Context, config cloudflare.TunnelConfig) error {
	api.updated = true
	api.config = config
	return nil
}
