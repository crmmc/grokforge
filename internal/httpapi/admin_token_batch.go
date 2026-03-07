package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/crmmc/grokforge/internal/config"
	"github.com/crmmc/grokforge/internal/store"
	"github.com/crmmc/grokforge/internal/token"
	"github.com/go-chi/chi/v5"
)

// handleBatchTokens returns a handler for batch token operations.
func handleBatchTokens(ts TokenStoreInterface, syncer TokenPoolSyncer, cfg *config.Config) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req BatchTokenRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			WriteError(w, 400, "invalid_request", "invalid_json",
				"Invalid JSON in request body")
			return
		}

		var resp BatchTokenResponse
		resp.Operation = req.Operation

		switch req.Operation {
		case BatchOpImport:
			var tokenCfg *config.TokenConfig
			if cfg != nil {
				tokenCfg = &cfg.Token
			} else {
				tokenCfg = &config.TokenConfig{}
			}
			resp = handleBatchImport(r.Context(), ts, syncer, req, tokenCfg)
		case BatchOpExport:
			resp = handleBatchExport(r.Context(), ts, req.IDs, r.URL.Query().Get("raw") == "true")
		case BatchOpDelete:
			resp = handleBatchDelete(r.Context(), ts, syncer, req)
		case BatchOpEnable, BatchOpDisable, BatchOpEnableNsfw, BatchOpDisableNsfw:
			resp = handleBatchUpdate(r.Context(), ts, syncer, req)
		default:
			WriteError(w, 400, "invalid_request", "invalid_operation",
				"Invalid operation. Must be: import, export, delete, enable, disable, enable_nsfw, or disable_nsfw")
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}
}

// BatchOperation represents the type of batch operation.
type BatchOperation string

const (
	BatchOpImport      BatchOperation = "import"
	BatchOpExport      BatchOperation = "export"
	BatchOpDelete      BatchOperation = "delete"
	BatchOpEnable      BatchOperation = "enable"
	BatchOpDisable     BatchOperation = "disable"
	BatchOpEnableNsfw  BatchOperation = "enable_nsfw"
	BatchOpDisableNsfw BatchOperation = "disable_nsfw"
)

// BatchTokenRequest is the request body for batch token operations.
type BatchTokenRequest struct {
	Operation   BatchOperation `json:"operation"`
	Tokens      []string       `json:"tokens,omitempty"`       // For import: raw token strings
	IDs         []uint         `json:"ids,omitempty"`          // For delete/enable/disable
	Pool        string         `json:"pool,omitempty"`         // For import: default pool
	Quota       *int           `json:"quota,omitempty"`        // For import: default quota (nil = auto-resolve from config)
	Priority    int            `json:"priority"`               // For import: token priority
	Status      string         `json:"status,omitempty"`       // For import: initial status (active or disabled, default: active)
	Remark      string         `json:"remark,omitempty"`       // For import: default remark
	NsfwEnabled *bool          `json:"nsfw_enabled,omitempty"` // For import: default nsfw
}

// BatchTokenResponse is the response for batch token operations.
type BatchTokenResponse struct {
	Operation BatchOperation  `json:"operation"`
	Success   int             `json:"success"`
	Failed    int             `json:"failed"`
	Errors    []BatchError    `json:"errors,omitempty"`
	Tokens    []TokenResponse `json:"tokens,omitempty"`     // For export (masked)
	RawTokens []string        `json:"raw_tokens,omitempty"` // For export with raw=true
}

// BatchError represents an error for a single item in a batch operation.
type BatchError struct {
	Index   int    `json:"index,omitempty"`
	ID      uint   `json:"id,omitempty"`
	Token   string `json:"token,omitempty"`
	Message string `json:"message"`
}

// resolveImportQuota resolves the chat quota for an imported token.
// If quota is nil or *quota <= 0, auto-resolve based on config defaults.
func resolveImportQuota(quota *int, cfg *config.TokenConfig) int {
	if quota != nil && *quota > 0 {
		return *quota
	}
	if cfg != nil && cfg.DefaultChatQuota > 0 {
		return cfg.DefaultChatQuota
	}
	return 50 // hardcoded fallback
}

// handleBatchImport imports multiple tokens.
func handleBatchImport(ctx context.Context, ts TokenStoreInterface, syncer TokenPoolSyncer, req BatchTokenRequest, cfg *config.TokenConfig) BatchTokenResponse {
	resp := BatchTokenResponse{Operation: BatchOpImport}

	// Resolve import status: default to "active", only allow "active" or "disabled"
	importStatus := store.TokenStatusActive
	if req.Status == store.TokenStatusDisabled {
		importStatus = store.TokenStatusDisabled
	}

	for i, tokenStr := range req.Tokens {
		if tokenStr == "" {
			resp.Failed++
			resp.Errors = append(resp.Errors, BatchError{
				Index:   i,
				Message: "empty token string",
			})
			continue
		}

		if len(tokenStr) < 20 {
			resp.Failed++
			resp.Errors = append(resp.Errors, BatchError{
				Index:   i,
				Token:   maskSecret(tokenStr),
				Message: "token too short (minimum 20 characters)",
			})
			continue
		}

		chatQ := resolveImportQuota(req.Quota, cfg)
		imageQ := 20
		videoQ := 10
		if cfg != nil {
			if cfg.DefaultImageQuota > 0 {
				imageQ = cfg.DefaultImageQuota
			}
			if cfg.DefaultVideoQuota > 0 {
				videoQ = cfg.DefaultVideoQuota
			}
		}
		token := &store.Token{
			Token:             tokenStr,
			Pool:              req.Pool,
			ChatQuota:         chatQ,
			InitialChatQuota:  chatQ,
			ImageQuota:        imageQ,
			InitialImageQuota: imageQ,
			VideoQuota:        videoQ,
			InitialVideoQuota: videoQ,
			Priority:          req.Priority,
			Status:            importStatus,
			Remark:            req.Remark,
			NsfwEnabled:       req.NsfwEnabled != nil && *req.NsfwEnabled,
		}

		if err := ts.CreateToken(ctx, token); err != nil {
			resp.Failed++
			resp.Errors = append(resp.Errors, BatchError{
				Index:   i,
				Token:   maskSecret(tokenStr),
				Message: "failed to create token",
			})
			continue
		}
		// Sync to in-memory pool
		if syncer != nil {
			syncer.AddToPool(token)
		}
		resp.Success++
	}

	return resp
}

// handleBatchExport exports tokens. If ids is non-empty, only exports those tokens.
func handleBatchExport(ctx context.Context, ts TokenStoreInterface, ids []uint, raw bool) BatchTokenResponse {
	resp := BatchTokenResponse{Operation: BatchOpExport}

	var tokens []*store.Token
	var err error

	if len(ids) > 0 {
		// Export only selected tokens
		for _, id := range ids {
			t, e := ts.GetToken(ctx, id)
			if e != nil {
				continue
			}
			tokens = append(tokens, t)
		}
	} else {
		tokens, err = ts.ListTokens(ctx)
		if err != nil {
			resp.Errors = append(resp.Errors, BatchError{
				Message: "failed to list tokens",
			})
			return resp
		}
	}

	if raw {
		resp.RawTokens = make([]string, len(tokens))
		for i, t := range tokens {
			resp.RawTokens[i] = t.Token
		}
	} else {
		resp.Tokens = make([]TokenResponse, len(tokens))
		for i, t := range tokens {
			resp.Tokens[i] = tokenToResponse(t)
		}
	}
	resp.Success = len(tokens)

	return resp
}

// handleBatchDelete deletes multiple tokens by ID.
func handleBatchDelete(ctx context.Context, ts TokenStoreInterface, syncer TokenPoolSyncer, req BatchTokenRequest) BatchTokenResponse {
	resp := BatchTokenResponse{Operation: BatchOpDelete}

	for _, id := range req.IDs {
		if err := ts.DeleteToken(ctx, id); err != nil {
			resp.Failed++
			resp.Errors = append(resp.Errors, BatchError{
				ID:      id,
				Message: "failed to delete token",
			})
			continue
		}
		// Sync to in-memory pool
		if syncer != nil {
			syncer.RemoveFromPool(id)
		}
		resp.Success++
	}

	return resp
}

// handleBatchUpdate handles enable/disable and nsfw batch operations.
func handleBatchUpdate(ctx context.Context, ts TokenStoreInterface, syncer TokenPoolSyncer, req BatchTokenRequest) BatchTokenResponse {
	resp := BatchTokenResponse{Operation: req.Operation}

	var batchReq store.BatchUpdateRequest
	batchReq.IDs = req.IDs

	switch req.Operation {
	case BatchOpEnable:
		batchReq.Status = ptrString(store.TokenStatusActive)
		batchReq.StatusReason = ptrString("")
	case BatchOpDisable:
		batchReq.Status = ptrString(store.TokenStatusDisabled)
		batchReq.StatusReason = ptrString("manual disable")
	case BatchOpEnableNsfw:
		batchReq.NsfwEnabled = ptrBool(true)
	case BatchOpDisableNsfw:
		batchReq.NsfwEnabled = ptrBool(false)
	}

	count, err := ts.BatchUpdateTokens(ctx, batchReq)
	if err != nil {
		resp.Errors = append(resp.Errors, BatchError{
			Message: "batch update failed: " + err.Error(),
		})
		return resp
	}

	// Sync each updated token to in-memory pool
	if syncer != nil {
		for _, id := range req.IDs {
			if err := syncer.SyncToken(ctx, id); err != nil {
				slog.Warn("failed to sync token to pool", "token_id", id, "error", err)
			}
		}
	}

	resp.Success = count
	return resp
}

// handleRefreshToken returns a handler that refreshes a token's quota.
func handleRefreshToken(tr TokenRefresher) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		idStr := chi.URLParam(r, "id")
		id, err := strconv.ParseUint(idStr, 10, 32)
		if err != nil {
			WriteError(w, 400, "invalid_request", "invalid_id",
				"Invalid token ID")
			return
		}

		refreshed, err := tr.RefreshToken(r.Context(), uint(id))
		if err != nil {
			if errors.Is(err, token.ErrTokenNotFound) {
				WriteError(w, 404, "not_found", "token_not_found",
					"Token not found")
				return
			}
			WriteError(w, 502, "server_error", "upstream_error",
				"Failed to refresh token quota")
			return
		}

		resp := tokenToResponse(refreshed)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}
}

// ptrString returns a pointer to a string.
func ptrString(s string) *string {
	return &s
}

// ptrBool returns a pointer to a bool.
func ptrBool(b bool) *bool {
	return &b
}
