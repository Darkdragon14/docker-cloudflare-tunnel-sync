package labels

import (
	"strings"
	"testing"

	"github.com/darkdragon/docker-cloudflare-tunnel-sync/internal/docker"
)

func TestParseContainers(t *testing.T) {
	parser := NewParser()

	containers := []docker.ContainerInfo{
		{
			ID:   "b",
			Name: "container-b",
			Labels: map[string]string{
				LabelEnable:  "true",
				LabelHost:    "b.example.com",
				LabelService: "http://b",
			},
		},
		{
			ID:   "a",
			Name: "container-a",
			Labels: map[string]string{
				LabelEnable:  "true",
				LabelHost:    "a.example.com",
				LabelPath:    "/api",
				LabelService: "http://a",
			},
		},
		{
			ID:   "c",
			Name: "container-c",
			Labels: map[string]string{
				LabelEnable: "false",
				LabelHost:   "ignored.example.com",
			},
		},
	}

	routes, errs := parser.ParseContainers(containers)
	if len(errs) != 0 {
		t.Fatalf("expected no errors, got %v", errs)
	}
	if len(routes) != 2 {
		t.Fatalf("expected 2 routes, got %d", len(routes))
	}
	if got := routes[0].Key.String(); got != "a.example.com/api" {
		t.Fatalf("expected first route to be a.example.com/api, got %s", got)
	}
	if got := routes[1].Key.String(); got != "b.example.com" {
		t.Fatalf("expected second route to be b.example.com, got %s", got)
	}
}

func TestParseContainersWithOriginLabels(t *testing.T) {
	parser := NewParser()

	containers := []docker.ContainerInfo{
		{
			ID:   "1",
			Name: "with-origin",
			Labels: map[string]string{
				LabelEnable:            "true",
				LabelHost:              "app.example.com",
				LabelService:           "https://app:443",
				LabelOriginServerName:  "app.internal",
				LabelOriginNoTLSVerify: "true",
			},
		},
	}

	routes, errs := parser.ParseContainers(containers)
	if len(errs) != 0 {
		t.Fatalf("expected no errors, got %v", errs)
	}
	if len(routes) != 1 {
		t.Fatalf("expected 1 route, got %d", len(routes))
	}
	route := routes[0]
	if route.OriginServerName == nil || *route.OriginServerName != "app.internal" {
		t.Fatalf("expected origin server name to be app.internal, got %+v", route.OriginServerName)
	}
	if route.NoTLSVerify == nil || !*route.NoTLSVerify {
		t.Fatalf("expected no TLS verify to be true, got %+v", route.NoTLSVerify)
	}
}

func TestParseContainersWithSuffixRoutes(t *testing.T) {
	parser := NewParser()

	containers := []docker.ContainerInfo{
		{
			ID:   "1",
			Name: "soulsync",
			Labels: map[string]string{
				LabelEnable:                         "true",
				LabelHost:                           "soulsync.example.com",
				LabelService:                        "http://soulsync:8008",
				LabelHost + ".spotify":              "soulsync-spotify.example.com",
				LabelService + ".spotify":           "http://soulsync:8888",
				LabelPath + ".spotify":              "/spotify",
				LabelHost + ".tidal":                "soulsync-tidal.example.com",
				LabelService + ".tidal":             "http://soulsync:8889",
				LabelPath + ".tidal":                "/tidal",
				LabelOriginServerName + ".tidal":    "tidal.internal",
				LabelOriginNoTLSVerify + ".tidal":   "true",
				LabelOriginNoTLSVerify + ".spotify": "false",
			},
		},
	}

	routes, errs := parser.ParseContainers(containers)
	if len(errs) != 0 {
		t.Fatalf("expected no errors, got %v", errs)
	}
	if len(routes) != 3 {
		t.Fatalf("expected 3 routes, got %d", len(routes))
	}

	if got := routes[0].Key.String(); got != "soulsync.example.com" {
		t.Fatalf("expected base route first, got %s", got)
	}
	if got := routes[1].Key.String(); got != "soulsync-spotify.example.com/spotify" {
		t.Fatalf("expected spotify route second, got %s", got)
	}
	if got := routes[2].Key.String(); got != "soulsync-tidal.example.com/tidal" {
		t.Fatalf("expected tidal route third, got %s", got)
	}

	if routes[1].NoTLSVerify == nil || *routes[1].NoTLSVerify {
		t.Fatalf("expected spotify no TLS verify to be false, got %+v", routes[1].NoTLSVerify)
	}
	if routes[2].OriginServerName == nil || *routes[2].OriginServerName != "tidal.internal" {
		t.Fatalf("expected tidal origin server name to be tidal.internal, got %+v", routes[2].OriginServerName)
	}
	if routes[2].NoTLSVerify == nil || !*routes[2].NoTLSVerify {
		t.Fatalf("expected tidal no TLS verify to be true, got %+v", routes[2].NoTLSVerify)
	}
}

func TestParseContainersMissingSuffixService(t *testing.T) {
	parser := NewParser()

	containers := []docker.ContainerInfo{
		{
			ID:   "1",
			Name: "missing-suffix-service",
			Labels: map[string]string{
				LabelEnable:            "true",
				LabelHost:              "app.example.com",
				LabelService:           "http://app:8000",
				LabelHost + ".spotify": "spotify.example.com",
			},
		},
	}

	routes, errs := parser.ParseContainers(containers)
	if len(routes) != 1 {
		t.Fatalf("expected 1 route, got %d", len(routes))
	}
	if len(errs) != 1 {
		t.Fatalf("expected 1 error, got %d: %v", len(errs), errs)
	}
	assertContains(t, []string{errs[0].Error()}, LabelHost+".spotify is set without matching "+LabelService+".spotify")
}

func TestParseContainersMissingSuffixHostname(t *testing.T) {
	parser := NewParser()

	containers := []docker.ContainerInfo{
		{
			ID:   "1",
			Name: "missing-suffix-hostname",
			Labels: map[string]string{
				LabelEnable:               "true",
				LabelHost:                 "app.example.com",
				LabelService:              "http://app:8000",
				LabelService + ".spotify": "http://spotify:9000",
			},
		},
	}

	routes, errs := parser.ParseContainers(containers)
	if len(routes) != 1 {
		t.Fatalf("expected 1 route, got %d", len(routes))
	}
	if len(errs) != 1 {
		t.Fatalf("expected 1 error, got %d: %v", len(errs), errs)
	}
	assertContains(t, []string{errs[0].Error()}, LabelService+".spotify is set without matching "+LabelHost+".spotify")
}

func TestParseContainersMixedSuffixValidation(t *testing.T) {
	parser := NewParser()

	containers := []docker.ContainerInfo{
		{
			ID:   "1",
			Name: "mixed-suffixes",
			Labels: map[string]string{
				LabelEnable:                       "true",
				LabelHost:                         "soulsync.example.com",
				LabelService:                      "http://soulsync:8008",
				LabelHost + ".spotify":            "soulsync-spotify.example.com",
				LabelService + ".spotify":         "http://soulsync:8888",
				LabelPath + ".spotify":            "/spotify",
				LabelHost + ".apple":              "soulsync-apple.example.com",
				LabelService + ".deezer":          "http://soulsync:8890",
				LabelHost + ".tidal":              "soulsync-tidal.example.com",
				LabelService + ".tidal":           "http://soulsync:8889",
				LabelPath + ".tidal":              "tidal",
				LabelHost + ".qobuz":              "soulsync-qobuz.example.com",
				LabelService + ".qobuz":           "http://soulsync:8891",
				LabelOriginNoTLSVerify + ".qobuz": "notabool",
				LabelHost + ".":                   "ignored-empty-suffix.example.com",
				LabelService + ".":                "http://ignored:9999",
			},
		},
	}

	routes, errs := parser.ParseContainers(containers)
	if len(routes) != 2 {
		t.Fatalf("expected 2 valid routes, got %d", len(routes))
	}
	if got := routes[0].Key.String(); got != "soulsync.example.com" {
		t.Fatalf("expected base route first, got %s", got)
	}
	if got := routes[1].Key.String(); got != "soulsync-spotify.example.com/spotify" {
		t.Fatalf("expected spotify route second, got %s", got)
	}

	if len(errs) != 4 {
		t.Fatalf("expected 4 errors, got %d: %v", len(errs), errs)
	}
	messages := make([]string, 0, len(errs))
	for _, err := range errs {
		messages = append(messages, err.Error())
	}
	assertContains(t, messages, LabelHost+".apple is set without matching "+LabelService+".apple")
	assertContains(t, messages, LabelService+".deezer is set without matching "+LabelHost+".deezer")
	assertContains(t, messages, LabelPath+".tidal must start with '/'")
	assertContains(t, messages, "invalid "+LabelOriginNoTLSVerify+".qobuz label")
}

func TestParseContainersOriginLabelsValidationErrors(t *testing.T) {
	parser := NewParser()

	containers := []docker.ContainerInfo{
		{
			ID:   "1",
			Name: "empty-origin-server-name",
			Labels: map[string]string{
				LabelEnable:           "true",
				LabelHost:             "app.example.com",
				LabelService:          "https://app:443",
				LabelOriginServerName: " ",
			},
		},
		{
			ID:   "2",
			Name: "bad-no-tls-verify",
			Labels: map[string]string{
				LabelEnable:            "true",
				LabelHost:              "app2.example.com",
				LabelService:           "https://app2:443",
				LabelOriginNoTLSVerify: "notabool",
			},
		},
	}

	routes, errs := parser.ParseContainers(containers)
	if len(routes) != 0 {
		t.Fatalf("expected no routes, got %d", len(routes))
	}
	if len(errs) != 2 {
		t.Fatalf("expected 2 errors, got %d: %v", len(errs), errs)
	}
	messages := []string{errs[0].Error(), errs[1].Error()}
	assertContains(t, messages, LabelOriginServerName+" cannot be empty")
	assertContains(t, messages, "invalid "+LabelOriginNoTLSVerify+" label")
}

func TestParseContainersValidationErrors(t *testing.T) {
	parser := NewParser()

	containers := []docker.ContainerInfo{
		{
			ID:   "1",
			Name: "missing-host",
			Labels: map[string]string{
				LabelEnable:  "true",
				LabelService: "http://app",
			},
		},
		{
			ID:   "2",
			Name: "bad-path",
			Labels: map[string]string{
				LabelEnable:  "true",
				LabelHost:    "example.com",
				LabelPath:    "api",
				LabelService: "http://app",
			},
		},
		{
			ID:   "3",
			Name: "duplicate-1",
			Labels: map[string]string{
				LabelEnable:  "true",
				LabelHost:    "dup.example.com",
				LabelService: "http://one",
			},
		},
		{
			ID:   "4",
			Name: "duplicate-2",
			Labels: map[string]string{
				LabelEnable:  "true",
				LabelHost:    "dup.example.com",
				LabelService: "http://two",
			},
		},
		{
			ID:   "5",
			Name: "bad-enable",
			Labels: map[string]string{
				LabelEnable:  "notabool",
				LabelHost:    "bad.example.com",
				LabelService: "http://bad",
			},
		},
	}

	_, errs := parser.ParseContainers(containers)
	if len(errs) != 4 {
		t.Fatalf("expected 4 errors, got %d: %v", len(errs), errs)
	}
	messages := []string{errs[0].Error(), errs[1].Error(), errs[2].Error(), errs[3].Error()}
	assertContains(t, messages, "missing required")
	assertContains(t, messages, "must start with '/'")
	assertContains(t, messages, "duplicate route definition")
	assertContains(t, messages, "invalid cloudflare.tunnel.enable label")
}

func TestParseAccessContainers(t *testing.T) {
	parser := NewParser()

	containers := []docker.ContainerInfo{
		{
			ID:   "1",
			Name: "access-app",
			Labels: map[string]string{
				AccessLabelEnable:                            "true",
				AccessLabelAppName:                           "internal",
				AccessLabelAppDomain:                         "internal.example.com",
				AccessLabelAppTags:                           "team,internal",
				AccessLabelPolicyPrefix + "1.name":           "employees",
				AccessLabelPolicyPrefix + "1.action":         "allow",
				AccessLabelPolicyPrefix + "1.include.emails": "a@example.com,b@example.com",
			},
		},
	}

	apps, errs := parser.ParseAccessContainers(containers)
	if len(errs) != 0 {
		t.Fatalf("expected no errors, got %v", errs)
	}
	if len(apps) != 1 {
		t.Fatalf("expected 1 app, got %d", len(apps))
	}
	app := apps[0]
	if app.Name != "internal" || app.Domain != "internal.example.com" {
		t.Fatalf("unexpected app details: %+v", app)
	}
	if !app.TagsSet {
		t.Fatalf("expected app tags to be set")
	}
	if len(app.Tags) != 2 || app.Tags[0] != "team" || app.Tags[1] != "internal" {
		t.Fatalf("unexpected app tags: %+v", app.Tags)
	}
	if len(app.Policies) != 1 {
		t.Fatalf("expected 1 policy, got %d", len(app.Policies))
	}
	policy := app.Policies[0]
	if !policy.Managed {
		t.Fatalf("expected managed policy")
	}
	if policy.Name != "employees" || policy.Action != "allow" {
		t.Fatalf("unexpected policy: %+v", policy)
	}
	if len(policy.IncludeEmails) != 2 {
		t.Fatalf("expected 2 include emails, got %d", len(policy.IncludeEmails))
	}
}

func TestParseAccessContainersIDOnlyPolicy(t *testing.T) {
	parser := NewParser()

	containers := []docker.ContainerInfo{
		{
			ID:   "1",
			Name: "access-app",
			Labels: map[string]string{
				AccessLabelEnable:                "true",
				AccessLabelAppName:               "id-only",
				AccessLabelAppDomain:             "id-only.example.com",
				AccessLabelPolicyPrefix + "1.id": "policy-id",
			},
		},
	}

	apps, errs := parser.ParseAccessContainers(containers)
	if len(errs) != 0 {
		t.Fatalf("expected no errors, got %v", errs)
	}
	if len(apps) != 1 {
		t.Fatalf("expected 1 app, got %d", len(apps))
	}
	policy := apps[0].Policies[0]
	if policy.Managed {
		t.Fatalf("expected id-only policy to be unmanaged")
	}
	if policy.ID != "policy-id" {
		t.Fatalf("expected policy id to be policy-id, got %s", policy.ID)
	}
}

func TestParseAccessContainersNameOnlyPolicy(t *testing.T) {
	parser := NewParser()

	containers := []docker.ContainerInfo{
		{
			ID:   "1",
			Name: "access-app",
			Labels: map[string]string{
				AccessLabelEnable:                  "true",
				AccessLabelAppName:                 "name-only",
				AccessLabelAppDomain:               "name-only.example.com",
				AccessLabelPolicyPrefix + "1.name": "existing-policy",
			},
		},
	}

	apps, errs := parser.ParseAccessContainers(containers)
	if len(errs) != 0 {
		t.Fatalf("expected no errors, got %v", errs)
	}
	if len(apps) != 1 {
		t.Fatalf("expected 1 app, got %d", len(apps))
	}
	policy := apps[0].Policies[0]
	if policy.Managed {
		t.Fatalf("expected name-only policy to be unmanaged")
	}
	if policy.Name != "existing-policy" {
		t.Fatalf("expected policy name to be existing-policy, got %s", policy.Name)
	}
}

func TestParseAccessContainersErrors(t *testing.T) {
	parser := NewParser()

	containers := []docker.ContainerInfo{
		{
			ID:   "1",
			Name: "missing-app-name",
			Labels: map[string]string{
				AccessLabelEnable:    "true",
				AccessLabelAppDomain: "example.com",
			},
		},
		{
			ID:   "2",
			Name: "bad-policy",
			Labels: map[string]string{
				AccessLabelEnable:                  "true",
				AccessLabelAppName:                 "app",
				AccessLabelAppDomain:               "app.example.com",
				AccessLabelPolicyPrefix + "0.name": "invalid",
			},
		},
	}

	_, errs := parser.ParseAccessContainers(containers)
	if len(errs) < 2 {
		t.Fatalf("expected at least 2 errors, got %d: %v", len(errs), errs)
	}
	messages := make([]string, 0, len(errs))
	for _, err := range errs {
		messages = append(messages, err.Error())
	}
	assertContains(t, messages, "missing required")
	assertContains(t, messages, "invalid access policy index")
}

func assertContains(t *testing.T, messages []string, needle string) {
	t.Helper()
	for _, message := range messages {
		if strings.Contains(message, needle) {
			return
		}
	}
	t.Fatalf("expected error containing %q, got %v", needle, messages)
}
