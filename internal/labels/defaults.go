package labels

import (
	"bytes"
	"fmt"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"text/template"

	"github.com/darkdragon/docker-cloudflare-tunnel-sync/internal/docker"
)

const (
	LabelPort    = LabelPrefix + "port"
	LabelNetwork = LabelPrefix + "network"

	composeServiceLabel = "com.docker.compose.service"
	composeProjectLabel = "com.docker.compose.project"

	// DefaultServiceTemplate is used when no service template is configured explicitly.
	DefaultServiceTemplate = "http://{{ .Name }}:{{ .Port }}"
)

// Defaults configures fallback values for containers that omit hostname or service labels.
type Defaults struct {
	// HostnameTemplate is a Go template rendered when cloudflare.tunnel.hostname is missing.
	// It may render a single hostname, a comma-separated list, or Traefik-style Host(`...`) rules.
	HostnameTemplate string
	// ServiceTemplate is a Go template rendered when cloudflare.tunnel.service is missing.
	ServiceTemplate string
	// Network selects which Docker network IP is exposed to templates as .IP.
	Network string
}

// TemplateData is the data available to default hostname and service templates.
type TemplateData struct {
	ID      string
	Name    string
	Service string
	Project string
	Labels  map[string]string
	Port    uint16
	Network string
	IP      string
}

var templateFuncs = template.FuncMap{
	"lower":      strings.ToLower,
	"upper":      strings.ToUpper,
	"trimPrefix": func(prefix, value string) string { return strings.TrimPrefix(value, prefix) },
	"trimSuffix": func(suffix, value string) string { return strings.TrimSuffix(value, suffix) },
	"replace":    func(old, new, value string) string { return strings.ReplaceAll(value, old, new) },
	"normalize":  normalizeName,
}

var (
	nonDNSChars    = regexp.MustCompile(`[^a-zA-Z0-9-]+`)
	traefikHostArg = regexp.MustCompile("`([^`]*)`")
)

func normalizeName(value string) string {
	return strings.Trim(nonDNSChars.ReplaceAllString(value, "-"), "-")
}

func compileTemplate(name, text string) (*template.Template, error) {
	if strings.TrimSpace(text) == "" {
		return nil, nil
	}
	tmpl, err := template.New(name).Funcs(templateFuncs).Option("missingkey=zero").Parse(text)
	if err != nil {
		return nil, fmt.Errorf("invalid %s template: %w", name, err)
	}
	return tmpl, nil
}

func renderTemplate(tmpl *template.Template, data TemplateData) (string, error) {
	var buffer bytes.Buffer
	if err := tmpl.Execute(&buffer, data); err != nil {
		return "", err
	}
	return strings.TrimSpace(buffer.String()), nil
}

func (parser *Parser) templateData(container docker.ContainerInfo, port uint16) TemplateData {
	labels := container.Labels
	if labels == nil {
		labels = map[string]string{}
	}
	service := strings.TrimSpace(labels[composeServiceLabel])
	if service == "" {
		service = container.Name
	}
	networkName, ip := selectNetwork(container, parser.defaults.Network)
	return TemplateData{
		ID:      container.ID,
		Name:    container.Name,
		Service: service,
		Project: strings.TrimSpace(labels[composeProjectLabel]),
		Labels:  labels,
		Port:    port,
		Network: networkName,
		IP:      ip,
	}
}

// resolveHostnames returns the base hostnames for a container: the explicit label when set
// (a single hostname, a comma/space separated list, or Traefik-style Host(`...`) rules),
// otherwise the rendered default hostname template.
func (parser *Parser) resolveHostnames(container docker.ContainerInfo) ([]string, error) {
	if value := strings.TrimSpace(container.Labels[LabelHost]); value != "" {
		hostnames, err := splitHostnames(value)
		if err != nil {
			return nil, fmt.Errorf("container %s: %s %q: %w", container.Name, LabelHost, value, err)
		}
		return hostnames, nil
	}
	if parser.hostnameTemplate == nil {
		return nil, fmt.Errorf("container %s: missing required %s label", container.Name, LabelHost)
	}

	rendered, err := renderTemplate(parser.hostnameTemplate, parser.templateData(container, 0))
	if err != nil {
		return nil, fmt.Errorf("container %s: render default hostname: %w", container.Name, err)
	}
	hostnames, err := splitHostnames(rendered)
	if err != nil {
		return nil, fmt.Errorf("container %s: default hostname %q: %w", container.Name, rendered, err)
	}
	return hostnames, nil
}

// splitHostnames accepts a comma/whitespace separated list or Traefik-style Host(`a`) || Host(`b`) rules.
func splitHostnames(value string) ([]string, error) {
	var candidates []string
	if strings.Contains(value, "Host(") {
		for _, match := range traefikHostArg.FindAllStringSubmatch(value, -1) {
			candidates = append(candidates, match[1])
		}
	} else {
		candidates = strings.FieldsFunc(value, func(r rune) bool {
			return r == ',' || r == ' ' || r == '\t' || r == '\n'
		})
	}

	seen := map[string]struct{}{}
	hostnames := []string{}
	for _, candidate := range candidates {
		hostname := strings.ToLower(strings.TrimSuffix(strings.TrimSpace(candidate), "."))
		if hostname == "" {
			continue
		}
		if strings.ContainsAny(hostname, "/:` ") || strings.HasPrefix(hostname, ".") || strings.Contains(hostname, "..") {
			return nil, fmt.Errorf("invalid hostname %q", hostname)
		}
		if _, ok := seen[hostname]; ok {
			continue
		}
		seen[hostname] = struct{}{}
		hostnames = append(hostnames, hostname)
	}
	if len(hostnames) == 0 {
		return nil, fmt.Errorf("rendered no hostnames")
	}
	return hostnames, nil
}

// resolveService returns the base service: the explicit label when set, otherwise the rendered
// default service template using the container's exposed port and network.
func (parser *Parser) resolveService(container docker.ContainerInfo) (string, error) {
	if service := strings.TrimSpace(container.Labels[LabelService]); service != "" {
		return service, nil
	}
	if parser.serviceTemplate == nil {
		return "", fmt.Errorf("container %s: missing required %s label", container.Name, LabelService)
	}

	port, err := resolvePort(container)
	if err != nil {
		return "", err
	}
	rendered, err := renderTemplate(parser.serviceTemplate, parser.templateData(container, port))
	if err != nil {
		return "", fmt.Errorf("container %s: render default service: %w", container.Name, err)
	}
	if err := validateServiceURL(rendered); err != nil {
		return "", fmt.Errorf("container %s: default service %q: %w; set %s explicitly", container.Name, rendered, err, LabelService)
	}
	return rendered, nil
}

// resolvePort picks the port from cloudflare.tunnel.port, or the lowest exposed TCP port.
func resolvePort(container docker.ContainerInfo) (uint16, error) {
	if value, ok := container.Labels[LabelPort]; ok {
		parsed, err := strconv.ParseUint(strings.TrimSpace(value), 10, 16)
		if err != nil || parsed == 0 {
			return 0, fmt.Errorf("container %s: invalid %s label %q", container.Name, LabelPort, value)
		}
		return uint16(parsed), nil
	}

	ports := []int{}
	seen := map[uint16]struct{}{}
	for _, port := range container.Ports {
		if port.PrivatePort == 0 || (port.Type != "" && port.Type != "tcp") {
			continue
		}
		if _, ok := seen[port.PrivatePort]; ok {
			continue
		}
		seen[port.PrivatePort] = struct{}{}
		ports = append(ports, int(port.PrivatePort))
	}
	if len(ports) == 0 {
		return 0, fmt.Errorf("container %s: no exposed TCP port found; set %s or %s", container.Name, LabelPort, LabelService)
	}
	sort.Ints(ports)
	return uint16(ports[0]), nil
}

// selectNetwork returns the network name and container IP used for templates.
// Priority: cloudflare.tunnel.network label, configured default network, first network by name.
func selectNetwork(container docker.ContainerInfo, defaultNetwork string) (string, string) {
	preferred := strings.TrimSpace(container.Labels[LabelNetwork])
	if preferred == "" {
		preferred = strings.TrimSpace(defaultNetwork)
	}
	if preferred != "" {
		if ip, ok := container.Networks[preferred]; ok {
			return preferred, ip
		}
		// Compose prefixes network names with the project name.
		if project := strings.TrimSpace(container.Labels[composeProjectLabel]); project != "" {
			prefixed := project + "_" + preferred
			if ip, ok := container.Networks[prefixed]; ok {
				return prefixed, ip
			}
		}
		return preferred, ""
	}

	names := make([]string, 0, len(container.Networks))
	for name, ip := range container.Networks {
		if ip != "" {
			names = append(names, name)
		}
	}
	if len(names) == 0 {
		return "", ""
	}
	sort.Strings(names)
	return names[0], container.Networks[names[0]]
}

func validateServiceURL(value string) error {
	if value == "" {
		return fmt.Errorf("rendered an empty service")
	}
	parsed, err := url.Parse(value)
	if err != nil {
		return err
	}
	if parsed.Scheme == "" {
		return fmt.Errorf("missing scheme")
	}
	if strings.Contains(value, "://") && parsed.Hostname() == "" && parsed.Scheme != "unix" && parsed.Scheme != "unix+tls" {
		return fmt.Errorf("missing host")
	}
	return nil
}
