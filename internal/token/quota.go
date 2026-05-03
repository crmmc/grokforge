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
	WaitTimeSeconds   int `json:"waitTimeSeconds,omitempty"`
}

type rawRateLimitsResponse struct {
	RemainingQueries  *int `json:"remainingQueries"`
	TotalQueries      *int `json:"totalQueries"`
	WindowSizeSeconds int  `json:"windowSizeSeconds"`
	WaitTimeSeconds   int  `json:"waitTimeSeconds,omitempty"`
}

const (
	rateLimitsPath                 = "/rest/rate-limits"
	rateLimitsBodyPreviewLimit     = int64(1024)
	rateLimitsResponseBodyLimit    = int64(1 << 20)
	rateLimitsClientTimeoutSeconds = 10
)

type rateLimitsHTTPError struct {
	statusCode    int
	bodyPreview   string
	bodyTruncated bool
}

func (e *rateLimitsHTTPError) Error() string {
	return fmt.Sprintf("rate-limits API returned %d", e.statusCode)
}

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

	client, err := tls_client.NewHttpClient(nil, tls_client.WithTimeoutSeconds(rateLimitsClientTimeoutSeconds))
	if err != nil {
		return nil, err
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyPreview, truncated, readErr := readBodyPreview(resp.Body, rateLimitsBodyPreviewLimit)
		if readErr != nil {
			return nil, readErr
		}
		return nil, &rateLimitsHTTPError{
			statusCode:    resp.StatusCode,
			bodyPreview:   bodyPreview,
			bodyTruncated: truncated,
		}
	}

	respBody, err := io.ReadAll(io.LimitReader(resp.Body, rateLimitsResponseBodyLimit))
	if err != nil {
		return nil, err
	}
	return decodeRateLimitsResponse(respBody)
}

func decodeRateLimitsResponse(body []byte) (*RateLimitsResponse, error) {
	var raw rawRateLimitsResponse
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, err
	}
	if raw.RemainingQueries == nil {
		return nil, errors.New("rate-limits response missing remainingQueries")
	}
	if raw.TotalQueries == nil {
		return nil, errors.New("rate-limits response missing totalQueries")
	}
	return &RateLimitsResponse{
		RemainingQueries:  *raw.RemainingQueries,
		TotalQueries:      *raw.TotalQueries,
		WindowSizeSeconds: raw.WindowSizeSeconds,
		WaitTimeSeconds:   raw.WaitTimeSeconds,
	}, nil
}

func readBodyPreview(r io.Reader, limit int64) (string, bool, error) {
	if limit <= 0 {
		limit = rateLimitsBodyPreviewLimit
	}
	body, err := io.ReadAll(io.LimitReader(r, limit+1))
	if err != nil {
		return "", false, err
	}
	truncated := int64(len(body)) > limit
	if truncated {
		body = body[:limit]
	}
	return string(body), truncated, nil
}
