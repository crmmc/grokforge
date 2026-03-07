package token

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"

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
	RequestKind string `json:"requestKind"`
	ModelName   string `json:"modelName"`
}

// RateLimitsResponse is the response from rate-limits API.
type RateLimitsResponse struct {
	RemainingQueries  int `json:"remainingQueries"`
	WindowSizeSeconds int `json:"windowSizeSeconds"`
}

const rateLimitsPath = "/rest/rate-limits"

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
	if token.ChatQuota <= 0 && token.ImageQuota <= 0 && token.VideoQuota <= 0 {
		token.Status = string(StatusCooling)
		token.CoolUntil = &now
	}
	m.dirty[tokenID] = struct{}{}

	return newVal, nil
}

// SyncQuota fetches quota from upstream API and updates token state.
// The upstream rate-limits API returns a single remainingQueries value
// which maps to ChatQuota (the primary category).
// If quota recovered and token is cooling, restores to active.
func (m *TokenManager) SyncQuota(ctx context.Context, token *store.Token, baseURL string) error {
	resp, err := fetchRateLimits(ctx, token.Token, baseURL)
	if err != nil {
		return fmt.Errorf("fetch rate limits: %w", err)
	}

	m.mu.Lock()
	defer m.mu.Unlock()

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

	switch {
	case resp.RemainingQueries > 0 && Status(token.Status) == StatusCooling:
		// Restore cooling token to active if quota recovered
		token.Status = string(StatusActive)
		token.StatusReason = ""
		token.CoolUntil = nil
		token.FailCount = 0
	case resp.RemainingQueries <= 0 && Status(token.Status) == StatusActive:
		// Prevent zombie: active token with no quota must enter cooling
		now := time.Now()
		token.Status = string(StatusCooling)
		token.CoolUntil = &now
	}

	m.dirty[token.ID] = struct{}{}
	return nil
}

// fetchRateLimits calls the rate-limits API.
func fetchRateLimits(ctx context.Context, authToken, baseURL string) (*RateLimitsResponse, error) {
	reqBody := RateLimitsRequest{
		RequestKind: "DEFAULT",
		ModelName:   "grok-3",
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

	client := &http.Client{Timeout: 10 * time.Second}
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
