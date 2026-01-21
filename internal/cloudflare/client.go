package cloudflare

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"strconv"
	"strings"
	"time"

	"github.com/darkdragon/docker-cloudflare-tunnel-sync/internal/config"
)

const defaultBaseURL = "https://api.cloudflare.com/client/v4"

// Client implements the Cloudflare API for Tunnel configurations and Access resources.
type Client struct {
	baseURL    *url.URL
	accountID  string
	tunnelID   string
	token      string
	userAgent  string
	httpClient *http.Client
}

// NewClient creates a Cloudflare API client.
func NewClient(cfg config.CloudflareConfig) (*Client, error) {
	base := cfg.BaseURL
	if base == "" {
		base = defaultBaseURL
	}
	parsed, err := url.Parse(base)
	if err != nil {
		return nil, fmt.Errorf("invalid Cloudflare base URL: %w", err)
	}

	return &Client{
		baseURL:   parsed,
		accountID: cfg.AccountID,
		tunnelID:  cfg.TunnelID,
		token:     cfg.APIToken,
		userAgent: "docker-cloudflare-tunnel-sync",
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}, nil
}

// GetConfig returns the current tunnel configuration and ingress rules.
func (client *Client) GetConfig(ctx context.Context) (TunnelConfig, error) {
	endpoint := client.configBase().String()
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return TunnelConfig{}, err
	}
	client.addHeaders(request)

	var response apiResponse[configResult]
	if err := client.do(request, &response); err != nil {
		return TunnelConfig{}, err
	}
	if err := response.Err(); err != nil {
		return TunnelConfig{}, err
	}

	config := response.Result.Config
	if config == nil {
		config = make(map[string]json.RawMessage)
	}

	ingress := []IngressRule{}
	if rawIngress, ok := config["ingress"]; ok && len(rawIngress) > 0 {
		if err := json.Unmarshal(rawIngress, &ingress); err != nil {
			return TunnelConfig{}, fmt.Errorf("invalid ingress rules: %w", err)
		}
	}

	return TunnelConfig{Ingress: ingress, Raw: config}, nil
}

// UpdateConfig replaces the tunnel configuration using the supplied ingress rules.
func (client *Client) UpdateConfig(ctx context.Context, config TunnelConfig) error {
	payloadConfig := config.Raw
	if payloadConfig == nil {
		payloadConfig = make(map[string]json.RawMessage)
	}

	ingressRaw, err := json.Marshal(config.Ingress)
	if err != nil {
		return err
	}
	payloadConfig["ingress"] = ingressRaw

	payload := configPayload{Config: payloadConfig}
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	endpoint := client.configBase().String()
	request, err := http.NewRequestWithContext(ctx, http.MethodPut, endpoint, bytes.NewBuffer(body))
	if err != nil {
		return err
	}
	client.addHeaders(request)
	request.Header.Set("Content-Type", "application/json")

	var response apiResponse[configResult]
	if err := client.do(request, &response); err != nil {
		return err
	}
	return response.Err()
}

// ListAccessApps returns all Access applications for the account.
func (client *Client) ListAccessApps(ctx context.Context) ([]AccessAppRecord, error) {
	endpoint := client.accessAppsBase().String()
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	client.addHeaders(request)

	var response apiResponse[[]accessAppPayload]
	if err := client.do(request, &response); err != nil {
		return nil, err
	}
	if err := response.Err(); err != nil {
		return nil, err
	}

	apps := make([]AccessAppRecord, 0, len(response.Result))
	for _, app := range response.Result {
		apps = append(apps, AccessAppRecord{
			ID:       app.ID,
			Name:     app.Name,
			Domain:   app.Domain,
			Type:     app.Type,
			Policies: parsePolicyRefs(app.Policies),
			Tags:     app.Tags,
		})
	}

	return apps, nil
}

// CreateAccessApp creates a new Access application.
func (client *Client) CreateAccessApp(ctx context.Context, input AccessAppInput) (AccessAppRecord, error) {
	payload := accessAppWritePayload{
		Name:     input.Name,
		Domain:   input.Domain,
		Type:     accessAppType(input.Type),
		Policies: encodePolicyRefs(input.Policies),
		Tags:     input.Tags,
	}

	return client.writeAccessApp(ctx, http.MethodPost, client.accessAppsBase(), payload)
}

// UpdateAccessApp updates an existing Access application.
func (client *Client) UpdateAccessApp(ctx context.Context, id string, input AccessAppInput) (AccessAppRecord, error) {
	payload := accessAppWritePayload{
		Name:     input.Name,
		Domain:   input.Domain,
		Type:     accessAppType(input.Type),
		Policies: encodePolicyRefs(input.Policies),
		Tags:     input.Tags,
	}
	endpoint := client.accessAppsBase()
	endpoint.Path = path.Join(endpoint.Path, id)
	return client.writeAccessApp(ctx, http.MethodPut, endpoint, payload)
}

// DeleteAccessApp removes an Access application.
func (client *Client) DeleteAccessApp(ctx context.Context, id string) error {
	endpoint := client.accessAppsBase()
	endpoint.Path = path.Join(endpoint.Path, id)

	request, err := http.NewRequestWithContext(ctx, http.MethodDelete, endpoint.String(), nil)
	if err != nil {
		return err
	}
	client.addHeaders(request)

	var response apiResponse[map[string]any]
	if err := client.do(request, &response); err != nil {
		return err
	}
	return response.Err()
}

// ListAccessPolicies returns all Access policies for the account.
func (client *Client) ListAccessPolicies(ctx context.Context) ([]AccessPolicyRecord, error) {
	endpoint := client.accessPoliciesBase().String()
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	client.addHeaders(request)

	var response apiResponse[[]accessPolicyPayload]
	if err := client.do(request, &response); err != nil {
		return nil, err
	}
	if err := response.Err(); err != nil {
		return nil, err
	}

	policies := make([]AccessPolicyRecord, 0, len(response.Result))
	for _, policy := range response.Result {
		include, unsupported := parseAccessRules(policy.Include)
		policies = append(policies, AccessPolicyRecord{
			ID:                  policy.ID,
			Name:                policy.Name,
			Action:              policy.Decision,
			Include:             include,
			HasUnsupportedRules: unsupported,
		})
	}

	return policies, nil
}

// CreateAccessPolicy creates a new Access policy.
func (client *Client) CreateAccessPolicy(ctx context.Context, input AccessPolicyInput) (AccessPolicyRecord, error) {
	payload := accessPolicyPayload{
		Name:     input.Name,
		Decision: input.Action,
		Include:  buildAccessRules(input.Include),
	}

	return client.writeAccessPolicy(ctx, http.MethodPost, client.accessPoliciesBase(), payload)
}

// UpdateAccessPolicy updates an existing Access policy.
func (client *Client) UpdateAccessPolicy(ctx context.Context, id string, input AccessPolicyInput) (AccessPolicyRecord, error) {
	payload := accessPolicyPayload{
		Name:     input.Name,
		Decision: input.Action,
		Include:  buildAccessRules(input.Include),
	}
	endpoint := client.accessPoliciesBase()
	endpoint.Path = path.Join(endpoint.Path, id)
	return client.writeAccessPolicy(ctx, http.MethodPut, endpoint, payload)
}

// EnsureAccessTag ensures the Access tag exists.
func (client *Client) EnsureAccessTag(ctx context.Context, name string) error {
	if strings.TrimSpace(name) == "" {
		return nil
	}

	exists, err := client.accessTagExists(ctx, name)
	if err != nil {
		return err
	}
	if exists {
		return nil
	}

	payload := accessTagPayload{Name: name}
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	endpoint := client.accessTagsBase().String()
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewBuffer(body))
	if err != nil {
		return err
	}
	client.addHeaders(request)
	request.Header.Set("Content-Type", "application/json")

	var response apiResponse[accessTagPayload]
	if err := client.do(request, &response); err != nil {
		return err
	}
	return response.Err()
}

func (client *Client) accessTagExists(ctx context.Context, name string) (bool, error) {
	endpoint := client.accessTagsBase()
	endpoint.Path = path.Join(endpoint.Path, url.PathEscape(name))
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint.String(), nil)
	if err != nil {
		return false, err
	}
	client.addHeaders(request)

	resp, err := client.httpClient.Do(request)
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return false, err
	}

	if resp.StatusCode == http.StatusNotFound {
		return false, nil
	}
	if resp.StatusCode >= http.StatusBadRequest {
		return false, fmt.Errorf("cloudflare API request failed with status %s: %s", resp.Status, strings.TrimSpace(string(body)))
	}

	var response apiResponse[accessTagPayload]
	if err := json.Unmarshal(body, &response); err != nil {
		return false, fmt.Errorf("cloudflare API returned non-JSON response with status %s: %w", resp.Status, err)
	}
	if err := response.Err(); err != nil {
		return false, err
	}
	return response.Result.Name != "", nil
}

func (client *Client) writeAccessApp(ctx context.Context, method string, endpoint *url.URL, payload accessAppWritePayload) (AccessAppRecord, error) {
	body, err := json.Marshal(payload)
	if err != nil {
		return AccessAppRecord{}, err
	}

	request, err := http.NewRequestWithContext(ctx, method, endpoint.String(), bytes.NewBuffer(body))
	if err != nil {
		return AccessAppRecord{}, err
	}
	client.addHeaders(request)
	request.Header.Set("Content-Type", "application/json")

	var response apiResponse[accessAppPayload]
	if err := client.do(request, &response); err != nil {
		return AccessAppRecord{}, err
	}
	if err := response.Err(); err != nil {
		return AccessAppRecord{}, err
	}

	return AccessAppRecord{
		ID:       response.Result.ID,
		Name:     response.Result.Name,
		Domain:   response.Result.Domain,
		Type:     response.Result.Type,
		Policies: parsePolicyRefs(response.Result.Policies),
		Tags:     response.Result.Tags,
	}, nil
}

func (client *Client) writeAccessPolicy(ctx context.Context, method string, endpoint *url.URL, payload accessPolicyPayload) (AccessPolicyRecord, error) {
	body, err := json.Marshal(payload)
	if err != nil {
		return AccessPolicyRecord{}, err
	}

	request, err := http.NewRequestWithContext(ctx, method, endpoint.String(), bytes.NewBuffer(body))
	if err != nil {
		return AccessPolicyRecord{}, err
	}
	client.addHeaders(request)
	request.Header.Set("Content-Type", "application/json")

	var response apiResponse[accessPolicyPayload]
	if err := client.do(request, &response); err != nil {
		return AccessPolicyRecord{}, err
	}
	if err := response.Err(); err != nil {
		return AccessPolicyRecord{}, err
	}

	include, unsupported := parseAccessRules(response.Result.Include)
	return AccessPolicyRecord{
		ID:                  response.Result.ID,
		Name:                response.Result.Name,
		Action:              response.Result.Decision,
		Include:             include,
		HasUnsupportedRules: unsupported,
	}, nil
}

// ListZones returns all DNS zones for the account.
func (client *Client) ListZones(ctx context.Context) ([]Zone, error) {
	zones := []Zone{}
	page := 1

	for {
		endpoint := client.zonesBase()
		query := endpoint.Query()
		query.Set("account.id", client.accountID)
		query.Set("per_page", "50")
		query.Set("page", strconv.Itoa(page))
		endpoint.RawQuery = query.Encode()

		request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint.String(), nil)
		if err != nil {
			return nil, err
		}
		client.addHeaders(request)

		var response apiResponseWithInfo[[]zonePayload]
		if err := client.do(request, &response); err != nil {
			return nil, err
		}
		if err := response.Err(); err != nil {
			return nil, err
		}
		for _, zone := range response.Result {
			zones = append(zones, Zone{ID: zone.ID, Name: zone.Name})
		}
		if response.ResultInfo.TotalPages == 0 || page >= response.ResultInfo.TotalPages {
			break
		}
		page++
	}

	return zones, nil
}

// ListDNSRecords returns DNS records for a zone by name and type.
func (client *Client) ListDNSRecords(ctx context.Context, zoneID string, recordType string, name string) ([]DNSRecord, error) {
	endpoint := client.dnsRecordsBase(zoneID)
	query := endpoint.Query()
	if recordType != "" {
		query.Set("type", recordType)
	}
	if name != "" {
		query.Set("name", name)
	}
	query.Set("per_page", "100")
	endpoint.RawQuery = query.Encode()

	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint.String(), nil)
	if err != nil {
		return nil, err
	}
	client.addHeaders(request)

	var response apiResponseWithInfo[[]dnsRecordPayload]
	if err := client.do(request, &response); err != nil {
		return nil, err
	}
	if err := response.Err(); err != nil {
		return nil, err
	}

	records := make([]DNSRecord, 0, len(response.Result))
	for _, record := range response.Result {
		records = append(records, DNSRecord{
			ID:      record.ID,
			Type:    record.Type,
			Name:    record.Name,
			Content: record.Content,
			Proxied: record.Proxied,
			Comment: record.Comment,
			TTL:     record.TTL,
		})
	}

	return records, nil
}

// CreateDNSRecord creates a DNS record in the given zone.
func (client *Client) CreateDNSRecord(ctx context.Context, zoneID string, input DNSRecordInput) (DNSRecord, error) {
	payload := dnsRecordWritePayload{
		Type:    input.Type,
		Name:    input.Name,
		Content: input.Content,
		Proxied: input.Proxied,
		TTL:     input.TTL,
		Comment: input.Comment,
	}
	return client.writeDNSRecord(ctx, http.MethodPost, client.dnsRecordsBase(zoneID), payload)
}

// UpdateDNSRecord updates a DNS record in the given zone.
func (client *Client) UpdateDNSRecord(ctx context.Context, zoneID string, recordID string, input DNSRecordInput) (DNSRecord, error) {
	payload := dnsRecordWritePayload{
		Type:    input.Type,
		Name:    input.Name,
		Content: input.Content,
		Proxied: input.Proxied,
		TTL:     input.TTL,
		Comment: input.Comment,
	}
	endpoint := client.dnsRecordsBase(zoneID)
	endpoint.Path = path.Join(endpoint.Path, recordID)
	return client.writeDNSRecord(ctx, http.MethodPut, endpoint, payload)
}

func (client *Client) writeDNSRecord(ctx context.Context, method string, endpoint *url.URL, payload dnsRecordWritePayload) (DNSRecord, error) {
	body, err := json.Marshal(payload)
	if err != nil {
		return DNSRecord{}, err
	}

	request, err := http.NewRequestWithContext(ctx, method, endpoint.String(), bytes.NewBuffer(body))
	if err != nil {
		return DNSRecord{}, err
	}
	client.addHeaders(request)
	request.Header.Set("Content-Type", "application/json")

	var response apiResponse[dnsRecordPayload]
	if err := client.do(request, &response); err != nil {
		return DNSRecord{}, err
	}
	if err := response.Err(); err != nil {
		return DNSRecord{}, err
	}

	return DNSRecord{
		ID:      response.Result.ID,
		Type:    response.Result.Type,
		Name:    response.Result.Name,
		Content: response.Result.Content,
		Proxied: response.Result.Proxied,
		Comment: response.Result.Comment,
		TTL:     response.Result.TTL,
	}, nil
}

func (client *Client) addHeaders(request *http.Request) {
	request.Header.Set("Authorization", "Bearer "+client.token)
	request.Header.Set("User-Agent", client.userAgent)
}

func (client *Client) configBase() *url.URL {
	base := *client.baseURL
	base.Path = path.Join(base.Path, "accounts", client.accountID, "cfd_tunnel", client.tunnelID, "configurations")
	return &base
}

func (client *Client) accessAppsBase() *url.URL {
	base := *client.baseURL
	base.Path = path.Join(base.Path, "accounts", client.accountID, "access", "apps")
	return &base
}

func (client *Client) accessPoliciesBase() *url.URL {
	base := *client.baseURL
	base.Path = path.Join(base.Path, "accounts", client.accountID, "access", "policies")
	return &base
}

func (client *Client) accessTagsBase() *url.URL {
	base := *client.baseURL
	base.Path = path.Join(base.Path, "accounts", client.accountID, "access", "tags")
	return &base
}

func (client *Client) zonesBase() *url.URL {
	base := *client.baseURL
	base.Path = path.Join(base.Path, "zones")
	return &base
}

func (client *Client) dnsRecordsBase(zoneID string) *url.URL {
	base := *client.baseURL
	base.Path = path.Join(base.Path, "zones", zoneID, "dns_records")
	return &base
}

type apiResponse[T any] struct {
	Success bool       `json:"success"`
	Errors  []apiError `json:"errors"`
	Result  T          `json:"result"`
}

func (response apiResponse[T]) Err() error {
	if response.Success {
		return nil
	}
	return fmt.Errorf("cloudflare API error: %s", joinErrors(response.Errors))
}

func (response apiResponse[T]) ErrorSummary() string {
	return joinErrors(response.Errors)
}

type apiResponseWithInfo[T any] struct {
	Success    bool       `json:"success"`
	Errors     []apiError `json:"errors"`
	Result     T          `json:"result"`
	ResultInfo resultInfo `json:"result_info"`
}

func (response apiResponseWithInfo[T]) Err() error {
	if response.Success {
		return nil
	}
	return fmt.Errorf("cloudflare API error: %s", joinErrors(response.Errors))
}

func (response apiResponseWithInfo[T]) ErrorSummary() string {
	return joinErrors(response.Errors)
}

type resultInfo struct {
	Page       int `json:"page"`
	PerPage    int `json:"per_page"`
	TotalPages int `json:"total_pages"`
}

type apiError struct {
	Message string `json:"message"`
}

type configResult struct {
	Config map[string]json.RawMessage `json:"config"`
}

type configPayload struct {
	Config map[string]json.RawMessage `json:"config"`
}

type accessAppPayload struct {
	ID       string            `json:"id,omitempty"`
	Name     string            `json:"name,omitempty"`
	Domain   string            `json:"domain,omitempty"`
	Type     string            `json:"type,omitempty"`
	Policies []json.RawMessage `json:"policies,omitempty"`
	Tags     []string          `json:"tags,omitempty"`
}

type accessAppWritePayload struct {
	Name     string                   `json:"name,omitempty"`
	Domain   string                   `json:"domain,omitempty"`
	Type     string                   `json:"type,omitempty"`
	Policies []accessPolicyRefPayload `json:"policies,omitempty"`
	Tags     []string                 `json:"tags,omitempty"`
}

type accessPolicyRefPayload struct {
	ID         string `json:"id"`
	Precedence int    `json:"precedence,omitempty"`
}

type accessPolicyPayload struct {
	ID       string                         `json:"id,omitempty"`
	Name     string                         `json:"name"`
	Decision string                         `json:"decision"`
	Include  []map[string]map[string]string `json:"include"`
}

type accessTagPayload struct {
	Name string `json:"name"`
}

type zonePayload struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type dnsRecordPayload struct {
	ID      string `json:"id,omitempty"`
	Type    string `json:"type"`
	Name    string `json:"name"`
	Content string `json:"content"`
	Proxied bool   `json:"proxied"`
	Comment string `json:"comment,omitempty"`
	TTL     int    `json:"ttl"`
}

type dnsRecordWritePayload struct {
	Type    string `json:"type"`
	Name    string `json:"name"`
	Content string `json:"content"`
	Proxied bool   `json:"proxied"`
	TTL     int    `json:"ttl,omitempty"`
	Comment string `json:"comment,omitempty"`
}

func (client *Client) do(request *http.Request, response any) error {
	resp, err := client.httpClient.Do(request)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	if len(body) == 0 {
		return fmt.Errorf("cloudflare API returned empty response with status %s", resp.Status)
	}
	if err := json.Unmarshal(body, response); err != nil {
		return fmt.Errorf("cloudflare API returned non-JSON response with status %s: %w", resp.Status, err)
	}

	if resp.StatusCode >= http.StatusBadRequest {
		summary := ""
		if payload, ok := response.(interface{ ErrorSummary() string }); ok {
			summary = strings.TrimSpace(payload.ErrorSummary())
		}
		if summary == "" || summary == "unknown error" {
			summary = strings.TrimSpace(string(body))
		}
		return fmt.Errorf("cloudflare API request failed with status %s: %s", resp.Status, summary)
	}

	return nil
}

func accessAppType(value string) string {
	if strings.TrimSpace(value) == "" {
		return "self_hosted"
	}
	return value
}

func parsePolicyRefs(raw []json.RawMessage) []AccessPolicyRef {
	refs := make([]AccessPolicyRef, 0, len(raw))
	for index, item := range raw {
		var id string
		if err := json.Unmarshal(item, &id); err == nil {
			if id != "" {
				refs = append(refs, AccessPolicyRef{ID: id, Precedence: index + 1})
			}
			continue
		}
		var payload struct {
			ID         string `json:"id"`
			Precedence int    `json:"precedence"`
		}
		if err := json.Unmarshal(item, &payload); err == nil {
			if payload.ID != "" {
				precedence := payload.Precedence
				if precedence == 0 {
					precedence = index + 1
				}
				refs = append(refs, AccessPolicyRef{ID: payload.ID, Precedence: precedence})
			}
		}
	}
	return refs
}

func encodePolicyRefs(refs []AccessPolicyRef) []accessPolicyRefPayload {
	payloads := make([]accessPolicyRefPayload, 0, len(refs))
	for _, ref := range refs {
		if ref.ID == "" {
			continue
		}
		payloads = append(payloads, accessPolicyRefPayload{ID: ref.ID, Precedence: ref.Precedence})
	}
	return payloads
}

func buildAccessRules(rules []AccessRule) []map[string]map[string]string {
	result := make([]map[string]map[string]string, 0, len(rules))
	for _, rule := range rules {
		if rule.Email != "" {
			result = append(result, map[string]map[string]string{"email": {"email": rule.Email}})
		}
		if rule.IP != "" {
			result = append(result, map[string]map[string]string{"ip": {"ip": rule.IP}})
		}
	}
	return result
}

func parseAccessRules(raw []map[string]map[string]string) ([]AccessRule, bool) {
	result := []AccessRule{}
	unsupported := false
	for _, entry := range raw {
		for key, value := range entry {
			switch key {
			case "email":
				if email, ok := value["email"]; ok && email != "" {
					result = append(result, AccessRule{Email: email})
				}
			case "ip":
				if ip, ok := value["ip"]; ok && ip != "" {
					result = append(result, AccessRule{IP: ip})
				}
			default:
				unsupported = true
			}
		}
	}
	return result, unsupported
}

func joinErrors(errors []apiError) string {
	if len(errors) == 0 {
		return "unknown error"
	}
	messages := make([]string, 0, len(errors))
	for _, item := range errors {
		messages = append(messages, item.Message)
	}
	return strings.Join(messages, "; ")
}
