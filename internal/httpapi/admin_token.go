package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/crmmc/grokforge/internal/store"
	tokenpkg "github.com/crmmc/grokforge/internal/token"
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

// TokenRefresher defines the interface for refreshing token quota.
type TokenRefresher interface {
	RefreshToken(ctx context.Context, id uint) (*store.Token, error)
}

// TokenPoolSyncer syncs admin token changes to in-memory pools.
type TokenPoolSyncer interface {
	AddToPool(token *store.Token) error
	RemoveFromPool(id uint)
	SyncToken(ctx context.Context, id uint) error
}

// TokenResponse is the API response for a token (with masked sensitive data).
type TokenResponse struct {
	ID              uint       `json:"id"`
	Token           string     `json:"token"`
	Pool            string     `json:"pool"`
	Status          string     `json:"status"`
	StatusReason    string     `json:"status_reason,omitempty"`
	ChatQuota       int        `json:"chat_quota"`
	TotalChatQuota  int        `json:"total_chat_quota"`
	ImageQuota      int        `json:"image_quota"`
	TotalImageQuota int        `json:"total_image_quota"`
	VideoQuota      int        `json:"video_quota"`
	TotalVideoQuota int        `json:"total_video_quota"`
	Grok43Quota      int        `json:"grok43_quota"`
	TotalGrok43Quota int        `json:"total_grok43_quota"`
	FailCount       int        `json:"fail_count"`
	CoolUntil       *time.Time `json:"cool_until,omitempty"`
	LastUsed        *time.Time `json:"last_used,omitempty"`
	Remark          string     `json:"remark,omitempty"`
	NsfwEnabled     bool       `json:"nsfw_enabled"`
	Priority        int        `json:"priority"`
	CreatedAt       time.Time  `json:"created_at"`
	UpdatedAt       time.Time  `json:"updated_at"`
}

// tokenToResponse converts a store.Token to TokenResponse with masked token.
func tokenToResponse(t *store.Token) TokenResponse {
	totalChat, totalImage, totalVideo, totalGrok43 := resolveTokenQuotaTotals(t, nil)

	return TokenResponse{
		ID:               t.ID,
		Token:            maskSecret(t.Token),
		Pool:             t.Pool,
		Status:           t.Status,
		StatusReason:     t.StatusReason,
		ChatQuota:        t.ChatQuota,
		TotalChatQuota:   totalChat,
		ImageQuota:       t.ImageQuota,
		TotalImageQuota:  totalImage,
		VideoQuota:       t.VideoQuota,
		TotalVideoQuota:  totalVideo,
		Grok43Quota:      t.Grok43Quota,
		TotalGrok43Quota: totalGrok43,
		FailCount:       t.FailCount,
		CoolUntil:       t.CoolUntil,
		LastUsed:        t.LastUsed,
		Remark:          t.Remark,
		NsfwEnabled:     t.NsfwEnabled,
		Priority:        t.Priority,
		CreatedAt:       t.CreatedAt,
		UpdatedAt:       t.UpdatedAt,
	}
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
func handleListTokens(ts TokenStoreInterface) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		filter := store.TokenFilter{}

		// Parse status filter
		if status := r.URL.Query().Get("status"); status != "" {
			filter.Status = &status
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
			data[i] = tokenToResponse(t)
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
func handleGetToken(ts TokenStoreInterface) http.HandlerFunc {
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

		resp := tokenToResponse(token)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}
}

// TokenUpdateRequest is the request body for updating a token.
type TokenUpdateRequest struct {
	Status      *string `json:"status,omitempty"`
	Pool        *string `json:"pool,omitempty"`
	ChatQuota   *int    `json:"chat_quota,omitempty"`
	ImageQuota  *int    `json:"image_quota,omitempty"`
	VideoQuota  *int    `json:"video_quota,omitempty"`
	Grok43Quota *int    `json:"grok43_quota,omitempty"`
	Remark      *string `json:"remark,omitempty"`
	NsfwEnabled *bool   `json:"nsfw_enabled,omitempty"`
}

// handleUpdateToken returns a handler that updates an existing token.
func handleUpdateToken(ts TokenStoreInterface, syncer TokenPoolSyncer) http.HandlerFunc {
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
				store.TokenStatusCooling:  true,
			}
			if !validStatuses[*req.Status] {
				WriteError(w, 400, "invalid_request", "invalid_status",
					"Invalid status. Must be: active, disabled, expired, or cooling")
				return
			}
		}
		if req.Pool != nil {
			pool, err := tokenpkg.NormalizePoolName(*req.Pool)
			if err != nil {
				WriteError(w, 400, "invalid_request", "invalid_pool",
					"Invalid pool. Must be one of: ssoBasic, ssoSuper, ssoHeavy, basic, super, heavy")
				return
			}
			req.Pool = &pool
		}

		// Apply updates
		if req.Status != nil {
			token.Status = *req.Status
			// Record status reason for manual changes
			switch *req.Status {
			case store.TokenStatusDisabled:
				token.StatusReason = "manual disable"
			case store.TokenStatusActive:
				token.StatusReason = ""
			}
		}
		if req.Pool != nil {
			token.Pool = *req.Pool
		}
		if req.ChatQuota != nil {
			token.ChatQuota = *req.ChatQuota
			token.InitialChatQuota = *req.ChatQuota
		}
		if req.ImageQuota != nil {
			token.ImageQuota = *req.ImageQuota
			token.InitialImageQuota = *req.ImageQuota
		}
		if req.VideoQuota != nil {
			token.VideoQuota = *req.VideoQuota
			token.InitialVideoQuota = *req.VideoQuota
		}
		if req.Grok43Quota != nil {
			token.Grok43Quota = *req.Grok43Quota
			token.InitialGrok43Quota = *req.Grok43Quota
		}
		if req.Remark != nil {
			token.Remark = *req.Remark
		}
		if req.NsfwEnabled != nil {
			token.NsfwEnabled = *req.NsfwEnabled
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

		resp := tokenToResponse(token)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}
}

// handleListTokenIDs returns a handler that lists token IDs matching a status filter.
func handleListTokenIDs(ts TokenStoreInterface) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		filter := store.TokenFilter{}
		if status := r.URL.Query().Get("status"); status != "" {
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
