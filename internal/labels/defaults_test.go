package labels

import (
	"strings"
	"testing"

	"github.com/darkdragon/docker-cloudflare-tunnel-sync/internal/docker"
)

func mustParserWithDefaults(t *testing.T, defaults Defaults) *Parser {
	t.Helper()
	parser, err := NewParserWithDefaults(defaults)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	return parser
}

func TestDefaultHostnameTraefikStyle(t *testing.T) {
	parser := mustParserWithDefaults(t, Defaults{
		HostnameTemplate: "Host(`{{ index .Labels \"com.docker.compose.service\" }}.a.example.com`) || Host(`{{ .Service }}.example.com`)",
		ServiceTemplate:  DefaultServiceTemplate,
	})

	routes, errs := parser.ParseContainers([]docker.ContainerInfo{{
		ID:   "1",
		Name: "stack-web-1",
		Labels: map[string]string{
			LabelEnable:                  "true",
			"com.docker.compose.service": "web",
		},
		Ports: []docker.PortInfo{{PrivatePort: 8080, Type: "tcp"}, {PrivatePort: 80, Type: "tcp"}, {PrivatePort: 53, Type: "udp"}},
	}})
	if len(errs) != 0 {
		t.Fatalf("expected no errors, got %v", errs)
	}
	if len(routes) != 2 {
		t.Fatalf("expected 2 routes, got %d", len(routes))
	}
	if routes[0].Key.Hostname != "web.a.example.com" || routes[1].Key.Hostname != "web.example.com" {
		t.Fatalf("unexpected hostnames: %s, %s", routes[0].Key.Hostname, routes[1].Key.Hostname)
	}
	for _, route := range routes {
		if route.Service != "http://stack-web-1:80" {
			t.Fatalf("expected lowest TCP port service, got %s", route.Service)
		}
	}
}

func TestDefaultsDoNotOverrideExplicitLabels(t *testing.T) {
	parser := mustParserWithDefaults(t, Defaults{
		HostnameTemplate: "{{ .Name }}.example.com",
		ServiceTemplate:  DefaultServiceTemplate,
	})

	routes, errs := parser.ParseContainers([]docker.ContainerInfo{{
		ID:   "1",
		Name: "app",
		Labels: map[string]string{
			LabelEnable:  "true",
			LabelHost:    "custom.example.com",
			LabelService: "http://custom:9000",
		},
	}})
	if len(errs) != 0 {
		t.Fatalf("expected no errors, got %v", errs)
	}
	if len(routes) != 1 || routes[0].Key.Hostname != "custom.example.com" || routes[0].Service != "http://custom:9000" {
		t.Fatalf("unexpected routes: %+v", routes)
	}
}

func TestDefaultServicePortLabelAndNetworkIP(t *testing.T) {
	parser := mustParserWithDefaults(t, Defaults{
		HostnameTemplate: "{{ .Service }}.example.com",
		ServiceTemplate:  "http://{{ .IP }}:{{ .Port }}",
		Network:          "proxy",
	})

	routes, errs := parser.ParseContainers([]docker.ContainerInfo{{
		ID:   "1",
		Name: "app",
		Labels: map[string]string{
			LabelEnable:                  "true",
			LabelPort:                    "3000",
			"com.docker.compose.project": "stack",
		},
		Ports:    []docker.PortInfo{{PrivatePort: 80, Type: "tcp"}},
		Networks: map[string]string{"stack_default": "172.18.0.2", "stack_proxy": "172.20.0.5"},
	}})
	if len(errs) != 0 {
		t.Fatalf("expected no errors, got %v", errs)
	}
	if len(routes) != 1 || routes[0].Service != "http://172.20.0.5:3000" {
		t.Fatalf("unexpected routes: %+v", routes)
	}
}

func TestDefaultServiceWithoutPorts(t *testing.T) {
	parser := mustParserWithDefaults(t, Defaults{
		HostnameTemplate: "{{ .Name }}.example.com",
		ServiceTemplate:  DefaultServiceTemplate,
	})

	routes, errs := parser.ParseContainers([]docker.ContainerInfo{{
		ID:     "1",
		Name:   "app",
		Labels: map[string]string{LabelEnable: "true"},
	}})
	if len(routes) != 0 {
		t.Fatalf("expected no routes, got %+v", routes)
	}
	if len(errs) != 1 || !strings.Contains(errs[0].Error(), "no exposed TCP port") {
		t.Fatalf("unexpected errors: %v", errs)
	}
}

func TestDefaultHostnameUsedForAccessDomain(t *testing.T) {
	parser := mustParserWithDefaults(t, Defaults{HostnameTemplate: "{{ .Name }}.example.com"})

	apps, errs := parser.ParseAccessContainers([]docker.ContainerInfo{{
		ID:   "1",
		Name: "app",
		Labels: map[string]string{
			AccessLabelEnable:                            "true",
			AccessLabelAppName:                           "App",
			AccessLabelPolicyPrefix + "1.name":           "allow",
			AccessLabelPolicyPrefix + "1.action":         "allow",
			AccessLabelPolicyPrefix + "1.include.emails": "a@example.com",
		},
	}})
	if len(errs) != 0 {
		t.Fatalf("expected no errors, got %v", errs)
	}
	if len(apps) != 1 || apps[0].Domain != "app.example.com" {
		t.Fatalf("unexpected apps: %+v", apps)
	}
}

func TestInvalidDefaultTemplate(t *testing.T) {
	if _, err := NewParserWithDefaults(Defaults{HostnameTemplate: "{{ .Name"}); err == nil {
		t.Fatal("expected template parse error")
	}
}

func TestSplitHostnames(t *testing.T) {
	hosts, err := splitHostnames("a.example.com, B.example.com a.example.com")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if strings.Join(hosts, ",") != "a.example.com,b.example.com" {
		t.Fatalf("unexpected hosts: %v", hosts)
	}
	if _, err := splitHostnames("http://bad"); err == nil {
		t.Fatal("expected invalid hostname error")
	}
	if _, err := splitHostnames(".example.com"); err == nil {
		t.Fatal("expected invalid hostname error for empty label")
	}
}
