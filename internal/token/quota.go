package token

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"

	http "github.com/bogdanfinn/fhttp"
	tls_client "github.com/bogdanfinn/tls-client"
)

var (
	// ErrNoQuota is returned when token has no remaining quota.
	ErrNoQuota = errors.New("no quota remaining")
	// ErrTokenNotFound is returned when token ID does not exist.
	ErrTokenNotFound = errors.New("token not found")
)

// RateLimitsRequest is the request body for rate-limits API.
type RateLimitsRequest struct {
	ModelName string `json:"modelName"`
}

// RateLimitsResponse is the response from rate-limits API.
type RateLimitsResponse struct {
	RemainingQueries  int `json:"remainingQueries"`
	TotalQueries      int `json:"totalQueries"`
	WindowSizeSeconds int `json:"windowSizeSeconds"`
}

const rateLimitsPath = "/rest/rate-limits"

// SyncModeQuota fetches quota for a specific mode from upstream and updates token state.
// upstreamName is the mode's upstream_name (e.g., "auto", "fast", "expert").
func (m *TokenManager) SyncModeQuota(ctx context.Context, tokenID uint, authToken string, baseURL string, upstreamName string) (*RateLimitsResponse, error) {
	resp, err := fetchRateLimits(ctx, authToken, baseURL, upstreamName)
	if err != nil {
		return nil, fmt.Errorf("fetch rate limits for %s: %w", upstreamName, err)
	}
	return resp, nil
}

// fetchRateLimits calls the rate-limits API for the given upstream mode name.
func fetchRateLimits(ctx context.Context, authToken, baseURL, upstreamName string) (*RateLimitsResponse, error) {
	reqBody := RateLimitsRequest{
		ModelName: upstreamName,
	}

	body, err := json.Marshal(reqBody)
	if err != nil {
		return nil, err
	}

	url := baseURL + rateLimitsPath
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Cookie", "sso="+authToken)

	client, err := tls_client.NewHttpClient(nil, tls_client.WithTimeoutSeconds(10))
	if err != nil {
		return nil, err
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("rate-limits API returned %d", resp.StatusCode)
	}

	respBody, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, err
	}

	var result RateLimitsResponse
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, err
	}

	return &result, nil
}
