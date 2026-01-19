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
	"strings"
	"time"

	"github.com/darkdragon/docker-cloudflare-tunnel-sync/internal/config"
)

const defaultBaseURL = "https://api.cloudflare.com/client/v4"

// Client implements the Cloudflare API for Tunnel configurations.
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

func (client *Client) addHeaders(request *http.Request) {
	request.Header.Set("Authorization", "Bearer "+client.token)
	request.Header.Set("User-Agent", client.userAgent)
}

func (client *Client) configBase() *url.URL {
	base := *client.baseURL
	base.Path = path.Join(base.Path, "accounts", client.accountID, "cfd_tunnel", client.tunnelID, "configurations")
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

type apiError struct {
	Message string `json:"message"`
}

type configResult struct {
	Config map[string]json.RawMessage `json:"config"`
}

type configPayload struct {
	Config map[string]json.RawMessage `json:"config"`
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
		return fmt.Errorf("cloudflare API request failed with status %s", resp.Status)
	}

	return nil
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
