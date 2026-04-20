package token

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"time"

	http "github.com/bogdanfinn/fhttp"
	tls_client "github.com/bogdanfinn/tls-client"
	"github.com/crmmc/grokforge/internal/store"
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
	WindowSizeSeconds int `json:"windowSizeSeconds"`
}

const rateLimitsPath = "/rest/rate-limits"
const minCoolingDuration = 5 * time.Minute
const rateLimitsProbeModeName = "auto"

// Consume deducts quota from the token for the given category.
// cost allows variable deduction for different model types.
// Returns remaining quota after deduction.
func (m *TokenManager) Consume(tokenID uint, cat QuotaCategory, cost int) (remaining int, err error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	token, ok := m.tokens[tokenID]
	if !ok {
		return 0, ErrTokenNotFound
	}

	cur := GetQuota(token, cat)
	if cur <= 0 {
		return 0, ErrNoQuota
	}

	if cost <= 0 {
		cost = 1
	}
	newVal := cur - cost
	if newVal < 0 {
		newVal = 0
	}
	SetQuota(token, cat, newVal)

	now := time.Now()
	token.LastUsed = &now

	// Only enter cooling if ALL categories are exhausted
	if token.ChatQuota <= 0 && token.ImageQuota <= 0 && token.VideoQuota <= 0 && token.Grok43Quota <= 0 {
		coolUntil := now.Add(m.coolingDurationForToken(token))
		token.Status = string(StatusCooling)
		token.CoolUntil = &coolUntil
	}
	m.dirty[tokenID] = struct{}{}

	return newVal, nil
}

// SyncQuota fetches quota from upstream API and updates token state.
// Accepts token ID and auth token string to avoid holding a pointer across
// the network call (which would race with other goroutines).
func (m *TokenManager) SyncQuota(ctx context.Context, tokenID uint, authToken string, baseURL string) error {
	resp, err := fetchRateLimits(ctx, authToken, baseURL)
	if err != nil {
		return fmt.Errorf("fetch rate limits: %w", err)
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	token, ok := m.tokens[tokenID]
	if !ok {
		return ErrTokenNotFound
	}

	token.ChatQuota = resp.RemainingQueries
	token.InitialChatQuota = resp.RemainingQueries

	// Restore image/video quotas to configured defaults on sync
	if m.cfg.DefaultImageQuota > 0 {
		token.ImageQuota = m.cfg.DefaultImageQuota
		token.InitialImageQuota = m.cfg.DefaultImageQuota
	}
	if m.cfg.DefaultVideoQuota > 0 {
		token.VideoQuota = m.cfg.DefaultVideoQuota
		token.InitialVideoQuota = m.cfg.DefaultVideoQuota
	}
	if m.cfg.DefaultGrok43Quota > 0 {
		token.Grok43Quota = m.cfg.DefaultGrok43Quota
		token.InitialGrok43Quota = m.cfg.DefaultGrok43Quota
	}

	switch {
	case resp.RemainingQueries > 0 && Status(token.Status) == StatusCooling:
		token.Status = string(StatusActive)
		token.StatusReason = ""
		token.CoolUntil = nil
		token.FailCount = 0
	case resp.RemainingQueries <= 0 && Status(token.Status) == StatusActive:
		now := time.Now()
		token.Status = string(StatusCooling)
		token.CoolUntil = &now
	}

	m.dirty[token.ID] = struct{}{}
	return nil
}

// fetchRateLimits calls the rate-limits API using the stable "auto" mode name.
// The upstream endpoint is mode-based ("auto", "fast", "expert", "heavy"),
// not version-model based (for example "grok-3").
func fetchRateLimits(ctx context.Context, authToken, baseURL string) (*RateLimitsResponse, error) {
	reqBody := RateLimitsRequest{
		ModelName: rateLimitsProbeModeName,
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

func (m *TokenManager) coolingDurationForToken(token *store.Token) time.Duration {
	if token == nil || m.cfg == nil {
		return minCoolingDuration
	}
	var duration time.Duration
	switch token.Pool {
	case PoolSuper:
		duration = time.Duration(m.cfg.SuperCoolDurationMin) * time.Minute
	case PoolHeavy:
		duration = time.Duration(m.cfg.HeavyCoolDurationMin) * time.Minute
	default:
		duration = time.Duration(m.cfg.BasicCoolDurationMin) * time.Minute
	}
	if duration < minCoolingDuration {
		return minCoolingDuration
	}
	return duration
}
