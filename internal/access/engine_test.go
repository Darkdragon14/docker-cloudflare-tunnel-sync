package access

import (
	"context"
	"log/slog"
	"testing"

	"github.com/darkdragon/docker-cloudflare-tunnel-sync/internal/cloudflare"
	"github.com/darkdragon/docker-cloudflare-tunnel-sync/internal/model"
)

func TestEnsurePoliciesIDOnlyReference(t *testing.T) {
	api := &stubAccessAPI{}
	logger := slog.New(slog.NewTextHandler(testWriter{t}, nil))
	engine := NewEngine(api, logger, false, true)

	app := model.AccessAppSpec{
		Name: "app",
		Policies: []model.AccessPolicySpec{
			{ID: "policy-1", Managed: false},
		},
	}

	refs, ok := engine.ensurePolicies(context.Background(), app, map[string]cloudflare.AccessPolicyRecord{}, map[string][]cloudflare.AccessPolicyRecord{})
	if !ok {
		t.Fatalf("expected ok to be true")
	}
	if len(refs) != 1 || refs[0].ID != "policy-1" {
		t.Fatalf("unexpected policy refs: %+v", refs)
	}
	if api.updatePolicyCalls != 0 {
		t.Fatalf("expected no policy updates, got %d", api.updatePolicyCalls)
	}
}

func TestEnsurePoliciesManagedMissingStops(t *testing.T) {
	api := &stubAccessAPI{}
	logger := slog.New(slog.NewTextHandler(testWriter{t}, nil))
	engine := NewEngine(api, logger, false, true)

	app := model.AccessAppSpec{
		Name: "app",
		Policies: []model.AccessPolicySpec{
			{ID: "missing", Managed: true},
		},
	}

	_, ok := engine.ensurePolicies(context.Background(), app, map[string]cloudflare.AccessPolicyRecord{}, map[string][]cloudflare.AccessPolicyRecord{})
	if ok {
		t.Fatalf("expected ok to be false when managed policy id is missing")
	}
}

func TestUpdatePolicyIfNeededDryRun(t *testing.T) {
	api := &stubAccessAPI{}
	logger := slog.New(slog.NewTextHandler(testWriter{t}, nil))
	engine := NewEngine(api, logger, true, true)

	spec := model.AccessPolicySpec{
		Name:          "policy",
		Action:        "allow",
		IncludeEmails: []string{"user@example.com"},
		Managed:       true,
	}
	record := cloudflare.AccessPolicyRecord{
		ID:     "policy-id",
		Name:   "policy",
		Action: "deny",
		Include: []cloudflare.AccessRule{
			{Email: "user@example.com"},
		},
	}

	engine.updatePolicyIfNeeded(context.Background(), model.AccessAppSpec{Name: "app"}, spec, record)

	if api.updatePolicyCalls != 0 {
		t.Fatalf("expected no policy updates during dry-run, got %d", api.updatePolicyCalls)
	}
}

func TestReconcileSkipsCreateWhenManageDisabled(t *testing.T) {
	api := &stubAccessAPI{}
	logger := slog.New(slog.NewTextHandler(testWriter{t}, nil))
	engine := NewEngine(api, logger, false, false)

	apps := []model.AccessAppSpec{
		{
			Name:   "app",
			Domain: "app.example.com",
			Policies: []model.AccessPolicySpec{
				{ID: "policy-1", Managed: false},
			},
		},
	}

	if err := engine.Reconcile(context.Background(), apps); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if api.createAppCalls != 0 {
		t.Fatalf("expected no app creation when manage is false, got %d", api.createAppCalls)
	}
}

func TestDeleteOrphanedAppsDeletesManaged(t *testing.T) {
	api := &stubAccessAPI{}
	logger := slog.New(slog.NewTextHandler(testWriter{t}, nil))
	engine := NewEngine(api, logger, false, true)

	existing := []cloudflare.AccessAppRecord{
		{ID: "app-1", Name: "app", Tags: []string{model.AccessManagedTag}},
	}
	engine.deleteOrphanedApps(context.Background(), existing, map[string]struct{}{})

	if api.deleteAppCalls != 1 {
		t.Fatalf("expected 1 delete call, got %d", api.deleteAppCalls)
	}
}

type testWriter struct {
	t *testing.T
}

func (w testWriter) Write(p []byte) (n int, err error) {
	w.t.Log(string(p))
	return len(p), nil
}

type stubAccessAPI struct {
	listApps          []cloudflare.AccessAppRecord
	listPolicies      []cloudflare.AccessPolicyRecord
	createAppCalls    int
	updateAppCalls    int
	deleteAppCalls    int
	createPolicyCalls int
	updatePolicyCalls int
	ensureTagCalls    int
}

func (api *stubAccessAPI) ListAccessApps(ctx context.Context) ([]cloudflare.AccessAppRecord, error) {
	return api.listApps, nil
}

func (api *stubAccessAPI) CreateAccessApp(ctx context.Context, input cloudflare.AccessAppInput) (cloudflare.AccessAppRecord, error) {
	api.createAppCalls++
	return cloudflare.AccessAppRecord{ID: "created", Name: input.Name, Domain: input.Domain, Policies: input.Policies, Tags: input.Tags}, nil
}

func (api *stubAccessAPI) UpdateAccessApp(ctx context.Context, id string, input cloudflare.AccessAppInput) (cloudflare.AccessAppRecord, error) {
	api.updateAppCalls++
	return cloudflare.AccessAppRecord{ID: id, Name: input.Name, Domain: input.Domain, Policies: input.Policies, Tags: input.Tags}, nil
}

func (api *stubAccessAPI) DeleteAccessApp(ctx context.Context, id string) error {
	api.deleteAppCalls++
	return nil
}

func (api *stubAccessAPI) ListAccessPolicies(ctx context.Context) ([]cloudflare.AccessPolicyRecord, error) {
	return api.listPolicies, nil
}

func (api *stubAccessAPI) CreateAccessPolicy(ctx context.Context, input cloudflare.AccessPolicyInput) (cloudflare.AccessPolicyRecord, error) {
	api.createPolicyCalls++
	return cloudflare.AccessPolicyRecord{ID: "policy", Name: input.Name, Action: input.Action, Include: input.Include}, nil
}

func (api *stubAccessAPI) UpdateAccessPolicy(ctx context.Context, id string, input cloudflare.AccessPolicyInput) (cloudflare.AccessPolicyRecord, error) {
	api.updatePolicyCalls++
	return cloudflare.AccessPolicyRecord{ID: id, Name: input.Name, Action: input.Action, Include: input.Include}, nil
}

func (api *stubAccessAPI) EnsureAccessTag(ctx context.Context, name string) error {
	api.ensureTagCalls++
	return nil
}
