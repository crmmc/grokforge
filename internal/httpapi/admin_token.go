package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/crmmc/grokforge/internal/registry"
	"github.com/crmmc/grokforge/internal/store"
	"github.com/crmmc/grokforge/internal/token"
	"github.com/go-chi/chi/v5"
)

// TokenStoreInterface defines the methods needed for token CRUD operations.
type TokenStoreInterface interface {
	ListTokens(ctx context.Context) ([]*store.Token, error)
	ListTokensFiltered(ctx context.Context, filter store.TokenFilter) ([]*store.Token, error)
	ListTokenIDs(ctx context.Context, filter store.TokenFilter) ([]uint, error)
	GetToken(ctx context.Context, id uint) (*store.Token, error)
	CreateToken(ctx context.Context, token *store.Token) error
	UpdateToken(ctx context.Context, token *store.Token) error
	DeleteToken(ctx context.Context, id uint) error
	BatchUpdateTokens(ctx context.Context, req store.BatchUpdateRequest) (int, error)
}

// TokenPoolSyncer syncs admin token changes to in-memory pools.
type TokenPoolSyncer interface {
	AddToPool(token *store.Token) error
	RemoveFromPool(id uint)
	SyncToken(ctx context.Context, id uint) error
}

// TokenRefresher performs immediate upstream quota refresh for a token.
type TokenRefresher interface {
	RefreshToken(ctx context.Context, id uint) (*store.Token, error)
}

// TokenInflightProvider provides in-memory inflight counts for tokens.
type TokenInflightProvider interface {
	GetInflight(id uint) int
}

func safeGetInflight(ip TokenInflightProvider, id uint) int {
	if ip == nil {
		return 0
	}
	return ip.GetInflight(id)
}

// TokenResponse is the API response for a token (with masked sensitive data).
type TokenResponse struct {
	ID            uint         `json:"id"`
	Token         string       `json:"token"`
	Pool          string       `json:"pool"`
	Status        string       `json:"status"`
	DisplayStatus string       `json:"display_status"`
	Quotas        store.IntMap `json:"quotas"`
	LimitQuotas   store.IntMap `json:"limit_quotas"`
	CoolUntils    store.IntMap `json:"cool_untils,omitempty"`
	Inflight      int          `json:"inflight"`
	FailCount     int          `json:"fail_count"`
	LastUsed      *time.Time   `json:"last_used,omitempty"`
	Remark        string       `json:"remark,omitempty"`
	NsfwEnabled   bool         `json:"nsfw_enabled"`
	StatusReason  string       `json:"status_reason,omitempty"`
	Priority      int          `json:"priority"`
	CreatedAt     time.Time    `json:"created_at"`
	UpdatedAt     time.Time    `json:"updated_at"`
}

// tokenToResponse converts a store.Token to TokenResponse with masked token.
func tokenToResponse(t *store.Token, reg *registry.ModelRegistry, inflight int) TokenResponse {
	return TokenResponse{
		ID:            t.ID,
		Token:         maskSecret(t.Token),
		Pool:          t.Pool,
		Status:        t.Status,
		DisplayStatus: deriveDisplayStatus(t, reg),
		Quotas:        filterModeMap(t.Quotas, reg),
		LimitQuotas:   filterModeMap(t.LimitQuotas, reg),
		CoolUntils:    filterModeMap(t.CoolUntils, reg),
		Inflight:      inflight,
		FailCount:     t.FailCount,
		LastUsed:      t.LastUsed,
		Remark:        t.Remark,
		NsfwEnabled:   t.NsfwEnabled,
		StatusReason:  t.StatusReason,
		Priority:      t.Priority,
		CreatedAt:     t.CreatedAt,
		UpdatedAt:     t.UpdatedAt,
	}
}

// deriveDisplayStatus computes the display status from persisted state.
func deriveDisplayStatus(t *store.Token, reg *registry.ModelRegistry) string {
	switch t.Status {
	case store.TokenStatusDisabled:
		return "disabled"
	case store.TokenStatusExpired:
		return "expired"
	}

	// Check if any mode is cooling.
	nowUnix := int(time.Now().Unix())
	anyCooling := false
	for _, until := range t.CoolUntils {
		if until > nowUnix {
			anyCooling = true
			break
		}
	}

	if reg == nil {
		for _, remaining := range t.Quotas {
			if remaining > 0 {
				if anyCooling {
					return "cooling"
				}
				return "active"
			}
		}
		if len(t.Quotas) > 0 {
			if anyCooling {
				return "cooling"
			}
			return "exhausted"
		}
		if anyCooling {
			return "cooling"
		}
		return "active"
	}

	supportedModes := supportedModesForPool(reg, t.Pool)
	if len(supportedModes) == 0 {
		if anyCooling {
			return "cooling"
		}
		return "active"
	}
	for _, mode := range supportedModes {
		if t.Quotas[mode] > 0 {
			if anyCooling {
				return "cooling"
			}
			return "active"
		}
	}
	if anyCooling {
		return "cooling"
	}
	return "exhausted"
}

func supportedModesForPool(reg *registry.ModelRegistry, pool string) []string {
	if reg == nil {
		return nil
	}
	modeSpecs := reg.SupportedModes(token.PoolToShort(pool))
	modes := make([]string, 0, len(modeSpecs))
	for _, mode := range modeSpecs {
		modes = append(modes, mode.ID)
	}
	return modes
}

func knownModeLimits(token *store.Token, reg *registry.ModelRegistry) store.IntMap {
	if reg == nil {
		return token.LimitQuotas
	}
	return filterModeMap(token.LimitQuotas, reg)
}

func filterModeMap(src store.IntMap, reg *registry.ModelRegistry) store.IntMap {
	if reg == nil || src == nil {
		return src
	}
	filtered := make(store.IntMap)
	for _, mode := range reg.AllModes() {
		val, ok := src[mode.ID]
		if !ok {
			continue
		}
		filtered[mode.ID] = val
	}
	return filtered
}

// PaginatedTokenResponse wraps tokens with pagination metadata.
type PaginatedTokenResponse struct {
	Data       []TokenResponse `json:"data"`
	Total      int             `json:"total"`
	Page       int             `json:"page"`
	PageSize   int             `json:"page_size"`
	TotalPages int             `json:"total_pages"`
}

// handleListTokens returns a handler that lists all tokens with pagination.
func handleListTokens(ts TokenStoreInterface, reg *registry.ModelRegistry, ip TokenInflightProvider) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		filter := store.TokenFilter{}

		// Parse status filter
		// "exhausted" is a derived display_status, not a DB status.
		// We fetch active tokens and filter in-memory.
		var filterExhausted bool
		if status := r.URL.Query().Get("status"); status != "" {
			if status == "exhausted" {
				filterExhausted = true
				active := store.TokenStatusActive
				filter.Status = &active
			} else {
				filter.Status = &status
			}
		}

		// Parse nsfw filter
		if nsfw := r.URL.Query().Get("nsfw"); nsfw != "" {
			val, err := strconv.ParseBool(nsfw)
			if err != nil {
				WriteError(w, 400, "invalid_request", "invalid_nsfw",
					"nsfw must be true or false")
				return
			}
			filter.NsfwEnabled = &val
		}

		// Parse pagination params
		page := 1
		pageSize := 20
		if p := r.URL.Query().Get("page"); p != "" {
			if v, err := strconv.Atoi(p); err == nil && v > 0 {
				page = v
			}
		}
		if ps := r.URL.Query().Get("page_size"); ps != "" {
			if v, err := strconv.Atoi(ps); err == nil && v > 0 && v <= 100 {
				pageSize = v
			}
		}

		var tokens []*store.Token
		var err error
		if filter.Status != nil || filter.NsfwEnabled != nil {
			tokens, err = ts.ListTokensFiltered(r.Context(), filter)
		} else {
			tokens, err = ts.ListTokens(r.Context())
		}

		if err != nil {
			WriteError(w, 500, "server_error", "list_failed",
				"Failed to list tokens")
			return
		}

		// Post-filter for derived display_status
		if filterExhausted {
			filtered := tokens[:0]
			for _, t := range tokens {
				if deriveDisplayStatus(t, reg) == "exhausted" {
					filtered = append(filtered, t)
				}
			}
			tokens = filtered
		}

		total := len(tokens)
		totalPages := 0
		if total > 0 {
			totalPages = (total + pageSize - 1) / pageSize
		}

		// Apply pagination
		offset := (page - 1) * pageSize
		end := offset + pageSize
		if offset > total {
			offset = total
		}
		if end > total {
			end = total
		}
		paged := tokens[offset:end]

		data := make([]TokenResponse, len(paged))
		for i, t := range paged {
			data[i] = tokenToResponse(t, reg, safeGetInflight(ip, t.ID))
		}

		resp := PaginatedTokenResponse{
			Data:       data,
			Total:      total,
			Page:       page,
			PageSize:   pageSize,
			TotalPages: totalPages,
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}
}

// handleGetToken returns a handler that gets a single token by ID.
func handleGetToken(ts TokenStoreInterface, reg *registry.ModelRegistry, ip TokenInflightProvider) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		idStr := chi.URLParam(r, "id")
		id, err := strconv.ParseUint(idStr, 10, 32)
		if err != nil {
			WriteError(w, 400, "invalid_request", "invalid_id",
				"Invalid token ID")
			return
		}

		token, err := ts.GetToken(r.Context(), uint(id))
		if err != nil {
			if errors.Is(err, store.ErrNotFound) {
				WriteError(w, 404, "not_found", "token_not_found",
					"Token not found")
				return
			}
			WriteError(w, 500, "server_error", "get_failed",
				"Failed to get token")
			return
		}

		resp := tokenToResponse(token, reg, safeGetInflight(ip, token.ID))
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}
}

// TokenUpdateRequest is the request body for updating a token.
type TokenUpdateRequest struct {
	Status      *string      `json:"status,omitempty"`
	Quotas      store.IntMap `json:"quotas,omitempty"`
	Remark      *string      `json:"remark,omitempty"`
	NsfwEnabled *bool        `json:"nsfw_enabled,omitempty"`
	Priority    *int         `json:"priority,omitempty"`
}

// handleUpdateToken returns a handler that updates an existing token.
func handleUpdateToken(
	ts TokenStoreInterface,
	syncer TokenPoolSyncer,
	reg *registry.ModelRegistry,
	ip TokenInflightProvider,
) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		idStr := chi.URLParam(r, "id")
		id, err := strconv.ParseUint(idStr, 10, 32)
		if err != nil {
			WriteError(w, 400, "invalid_request", "invalid_id",
				"Invalid token ID")
			return
		}

		// First get the existing token
		token, err := ts.GetToken(r.Context(), uint(id))
		if err != nil {
			if errors.Is(err, store.ErrNotFound) {
				WriteError(w, 404, "not_found", "token_not_found",
					"Token not found")
				return
			}
			WriteError(w, 500, "server_error", "get_failed",
				"Failed to get token")
			return
		}

		var req TokenUpdateRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			WriteError(w, 400, "invalid_request", "invalid_json",
				"Invalid JSON in request body")
			return
		}

		// Validate remark max length
		if req.Remark != nil && len(*req.Remark) > 500 {
			WriteError(w, 400, "invalid_request", "remark_too_long",
				"Remark must be 500 characters or less")
			return
		}

		// Validate status if provided
		if req.Status != nil {
			validStatuses := map[string]bool{
				store.TokenStatusActive:   true,
				store.TokenStatusDisabled: true,
				store.TokenStatusExpired:  true,
			}
			if !validStatuses[*req.Status] {
				WriteError(w, 400, "invalid_request", "invalid_status",
					"Invalid status. Must be: active, disabled, or expired")
				return
			}
		}

		// Validate quotas: keys must exist in LimitQuotas, values must not exceed limits
		if len(req.Quotas) > 0 {
			limits := knownModeLimits(token, reg)
			for mode, val := range req.Quotas {
				limit, ok := limits[mode]
				if !ok {
					WriteError(w, 400, "invalid_request", "unknown_mode",
						"Unknown quota mode: "+mode)
					return
				}
				if val < 0 {
					WriteError(w, 400, "invalid_request", "invalid_quota",
						"Quota for mode "+mode+" must be >= 0")
					return
				}
				if val > limit {
					WriteError(w, 400, "invalid_request", "quota_exceeds_limit",
						"Quota for mode "+mode+" exceeds limit")
					return
				}
			}
		}

		// Apply updates
		if req.Status != nil {
			token.Status = *req.Status
			switch *req.Status {
			case store.TokenStatusDisabled:
				token.StatusReason = "manual disable"
			case store.TokenStatusActive:
				token.StatusReason = ""
			}
		}
		if len(req.Quotas) > 0 {
			if token.Quotas == nil {
				token.Quotas = make(store.IntMap)
			}
			for mode, val := range req.Quotas {
				token.Quotas[mode] = val
			}
		}
		if req.Remark != nil {
			token.Remark = *req.Remark
		}
		if req.NsfwEnabled != nil {
			token.NsfwEnabled = *req.NsfwEnabled
		}
		if req.Priority != nil {
			token.Priority = *req.Priority
		}

		if err := ts.UpdateToken(r.Context(), token); err != nil {
			WriteError(w, 500, "server_error", "update_failed",
				"Failed to update token")
			return
		}

		// Sync to in-memory pool
		if syncer != nil {
			if err := syncer.SyncToken(r.Context(), uint(id)); err != nil {
				slog.Warn("failed to sync token to pool", "token_id", id, "error", err)
			}
		}

		resp := tokenToResponse(token, reg, safeGetInflight(ip, token.ID))
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}
}

// handleListTokenIDs returns a handler that lists token IDs matching a status filter.
func handleListTokenIDs(ts TokenStoreInterface, reg *registry.ModelRegistry) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		filter := store.TokenFilter{}
		if status := r.URL.Query().Get("status"); status != "" {
			if status == "exhausted" {
				active := store.TokenStatusActive
				filter.Status = &active
				tokens, err := ts.ListTokensFiltered(r.Context(), filter)
				if err != nil {
					WriteError(w, 500, "server_error", "list_failed", "Failed to list token IDs")
					return
				}
				ids := make([]uint, 0, len(tokens))
				for _, token := range tokens {
					if deriveDisplayStatus(token, reg) == "exhausted" {
						ids = append(ids, token.ID)
					}
				}
				WriteJSON(w, http.StatusOK, map[string][]uint{"ids": ids})
				return
			}
			filter.Status = &status
		}
		ids, err := ts.ListTokenIDs(r.Context(), filter)
		if err != nil {
			WriteError(w, 500, "server_error", "list_failed",
				"Failed to list token IDs")
			return
		}
		WriteJSON(w, http.StatusOK, map[string][]uint{"ids": ids})
	}
}

// handleDeleteToken returns a handler that deletes a token.
func handleDeleteToken(ts TokenStoreInterface, syncer TokenPoolSyncer) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		idStr := chi.URLParam(r, "id")
		id, err := strconv.ParseUint(idStr, 10, 32)
		if err != nil {
			WriteError(w, 400, "invalid_request", "invalid_id",
				"Invalid token ID")
			return
		}

		if err := ts.DeleteToken(r.Context(), uint(id)); err != nil {
			if errors.Is(err, store.ErrNotFound) {
				WriteError(w, 404, "not_found", "token_not_found",
					"Token not found")
				return
			}
			WriteError(w, 500, "server_error", "delete_failed",
				"Failed to delete token")
			return
		}

		// Sync to in-memory pool
		if syncer != nil {
			syncer.RemoveFromPool(uint(id))
		}

		w.WriteHeader(http.StatusNoContent)
	}
}

// handleRefreshToken forces an immediate upstream refresh for the token.
func handleRefreshToken(
	ts TokenStoreInterface,
	refresher TokenRefresher,
	reg *registry.ModelRegistry,
	ip TokenInflightProvider,
) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if refresher == nil {
			WriteError(w, 501, "server_error", "refresh_not_configured", "Token refresh not configured")
			return
		}

		idStr := chi.URLParam(r, "id")
		id, err := strconv.ParseUint(idStr, 10, 32)
		if err != nil {
			WriteError(w, 400, "invalid_request", "invalid_id", "Invalid token ID")
			return
		}

		token, err := refresher.RefreshToken(r.Context(), uint(id))
		if err != nil {
			if errors.Is(err, store.ErrNotFound) {
				WriteError(w, 404, "not_found", "token_not_found", "Token not found")
				return
			}
			WriteError(w, 502, "server_error", "refresh_failed", err.Error())
			return
		}
		if token == nil {
			token, err = ts.GetToken(r.Context(), uint(id))
			if err != nil {
				WriteError(w, 500, "server_error", "get_failed", "Failed to get token")
				return
			}
		}

		WriteJSON(w, http.StatusOK, tokenToResponse(token, reg, safeGetInflight(ip, token.ID)))
	}
}
