package xai

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"

	http "github.com/bogdanfinn/fhttp"
	tls_client "github.com/bogdanfinn/tls-client"
)

// ---------- Endpoints ----------

const (
	acceptTOSURL   = "https://accounts.x.ai/auth_mgmt.AuthManagement/SetTosAcceptedVersion"
	setBirthURL    = "https://grok.com/rest/auth/set-birth-date"
	setNsfwURL     = "https://grok.com/auth_mgmt.AuthManagement/UpdateUserFeatureControls"
	accountsOrigin = "https://accounts.x.ai"
)

// ---------- nsfwHTTPClient interface ----------

// nsfwHTTPClient is the minimal interface NsfwClient needs for HTTP calls.
// In production this is satisfied by tls_client.HttpClient; in tests by a mock.
type nsfwHTTPClient interface {
	Do(req *http.Request) (*http.Response, error)
}

// ---------- NsfwClient ----------

// NsfwClient handles the upstream NSFW enable sequence (AcceptTOS → SetBirthDate → SetNSFW).
type NsfwClient struct {
	token     string
	opts      *Options
	statsigID string
	http      nsfwHTTPClient
}

// NewNsfwClient creates a new NsfwClient with the given token and options.
func NewNsfwClient(token string, opts ...ClientOption) (*NsfwClient, error) {
	options := DefaultOptions()
	for _, opt := range opts {
		opt(options)
	}

	httpClient, err := newTLSClient(options, options.ProxyURL)
	if err != nil {
		return nil, fmt.Errorf("nsfw: failed to create tls client: %w", err)
	}

	c := &NsfwClient{
		token: token,
		opts:  options,
		http:  httpClient,
	}
	if !options.DynamicStatsig {
		c.statsigID = staticStatsigID
	}
	return c, nil
}

// Close releases resources held by the NsfwClient.
func (c *NsfwClient) Close() {
	if closer, ok := c.http.(tls_client.HttpClient); ok {
		closer.CloseIdleConnections()
	}
}

// AcceptTOS calls the upstream AcceptTOS endpoint.
func (c *NsfwClient) AcceptTOS(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, acceptTOSURL, bytes.NewReader(acceptTOSPayload))
	if err != nil {
		return fmt.Errorf("nsfw: accept_tos: build request: %w", err)
	}

	c.applyHeaders(req, headerOverride{
		contentType:  "application/grpc-web+proto",
		origin:       accountsOrigin,
		referer:      accountsOrigin + "/accept-tos",
		secFetchSite: "same-origin",
		grpcWeb:      true,
	})

	return c.doGRPCCall(req, "accept_tos")
}

// SetBirthDate calls the upstream SetBirthDate REST endpoint.
func (c *NsfwClient) SetBirthDate(ctx context.Context) error {
	payload, err := json.Marshal(map[string]string{
		"birthDate": "1990-01-15T12:00:00.000Z",
	})
	if err != nil {
		return fmt.Errorf("nsfw: set_birth_date: marshal: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, setBirthURL, bytes.NewReader(payload))
	if err != nil {
		return fmt.Errorf("nsfw: set_birth_date: build request: %w", err)
	}

	c.applyHeaders(req, headerOverride{
		referer: "https://grok.com/?_s=data",
	})

	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("nsfw: set_birth_date: %w", err)
	}
	defer resp.Body.Close()

	body, readErr := io.ReadAll(resp.Body)
	if readErr != nil {
		return fmt.Errorf("nsfw: set_birth_date: read body: %w", readErr)
	}

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusNoContent {
		return fmt.Errorf("nsfw: set_birth_date: HTTP %d body=%s", resp.StatusCode, truncate(body, 200))
	}

	// Non-empty body must be valid JSON (catches HTML login pages, CF challenges, etc.)
	if len(bytes.TrimSpace(body)) > 0 {
		var out map[string]any
		if err := json.Unmarshal(body, &out); err != nil {
			return fmt.Errorf("nsfw: set_birth_date: non-JSON response: %w", err)
		}
	}

	slog.Debug("nsfw: set_birth_date completed", "status", resp.StatusCode)
	return nil
}

// SetNSFW calls the upstream SetNSFW gRPC-Web endpoint.
func (c *NsfwClient) SetNSFW(ctx context.Context, enabled bool) error {
	payload := buildNsfwPayload(enabled)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, setNsfwURL, bytes.NewReader(payload))
	if err != nil {
		return fmt.Errorf("nsfw: set_nsfw: build request: %w", err)
	}

	c.applyHeaders(req, headerOverride{
		contentType: "application/grpc-web+proto",
		referer:     "https://grok.com/?_s=data",
		grpcWeb:     true,
	})

	return c.doGRPCCall(req, "set_nsfw")
}

// ---------- header helpers ----------

type headerOverride struct {
	contentType  string
	origin       string
	referer      string
	secFetchSite string
	grpcWeb      bool
}

func (c *NsfwClient) applyHeaders(req *http.Request, ov headerOverride) {
	base := buildHeaders(c.token, c.opts, c.statsigID)
	for k, v := range base {
		if k == http.HeaderOrderKey {
			continue
		}
		req.Header.Set(k, v[0])
	}

	if ov.contentType != "" {
		req.Header.Set("Content-Type", ov.contentType)
	}
	if ov.origin != "" {
		req.Header.Set("Origin", ov.origin)
	}
	if ov.referer != "" {
		req.Header.Set("Referer", ov.referer)
	}
	if ov.secFetchSite != "" {
		req.Header.Set("Sec-Fetch-Site", ov.secFetchSite)
	}

	order := make([]string, len(base[http.HeaderOrderKey]))
	copy(order, base[http.HeaderOrderKey])

	if ov.grpcWeb {
		req.Header.Set("x-grpc-web", "1")
		req.Header.Set("x-user-agent", "connect-es/2.1.1")
		req.Header.Set("Cache-Control", "no-cache")
		req.Header.Set("Pragma", "no-cache")
		order = append(order, "x-grpc-web", "x-user-agent", "cache-control", "pragma")
	}

	req.Header[http.HeaderOrderKey] = order
}

// ---------- gRPC response handling ----------

func (c *NsfwClient) doGRPCCall(req *http.Request, label string) error {
	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("nsfw: %s: %w", label, err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("nsfw: %s: read body: %w", label, err)
	}

	if resp.StatusCode >= 400 {
		return fmt.Errorf("nsfw: %s: HTTP %d body=%s", label, resp.StatusCode, truncate(body, 200))
	}

	code, msg, err := grpcwebParseTrailers(body)
	if err != nil {
		return fmt.Errorf("nsfw: %s: parse trailers: %w", label, err)
	}
	if code != 0 {
		return fmt.Errorf("nsfw: %s: gRPC error code=%d message=%q", label, code, msg)
	}
	slog.Debug("nsfw: grpc call completed", "label", label, "grpc_code", code)
	return nil
}

// ---------- Sequence orchestration ----------

// EnableNSFW runs the 3-step sequence: AcceptTOS → SetBirthDate → SetNSFW(true).
// Any step failure returns an error immediately.
func EnableNSFW(ctx context.Context, c *NsfwClient) error {
	if err := c.AcceptTOS(ctx); err != nil {
		return fmt.Errorf("nsfw sequence step 1 (accept_tos): %w", err)
	}
	if err := c.SetBirthDate(ctx); err != nil {
		return fmt.Errorf("nsfw sequence step 2 (set_birth_date): %w", err)
	}
	if err := c.SetNSFW(ctx, true); err != nil {
		return fmt.Errorf("nsfw sequence step 3 (set_nsfw): %w", err)
	}
	slog.Info("nsfw: enable sequence completed")
	return nil
}
