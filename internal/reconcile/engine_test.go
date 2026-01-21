package reconcile

import (
	"context"
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
		{Hostname: "a.example.com", Path: "/app", Service: "http://a", OriginRequest: []byte(`{"noTLSVerify":true}`)},
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
	if string(desiredIngress[0].OriginRequest) == "" {
		t.Fatalf("expected origin request to be preserved")
	}
	if desiredIngress[1].Hostname != "c.example.com" {
		t.Fatalf("unexpected second desired rule: %+v", desiredIngress[1])
	}
	if desiredIngress[2].Service != model.FallbackService {
		t.Fatalf("expected fallback rule at end")
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
