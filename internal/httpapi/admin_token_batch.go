package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"

	"github.com/crmmc/grokforge/internal/registry"
	"github.com/crmmc/grokforge/internal/store"
	"github.com/crmmc/grokforge/internal/token"
)

// handleBatchTokens returns a handler for batch token operations.
func handleBatchTokens(ts TokenStoreInterface, syncer TokenPoolSyncer, reg *registry.ModelRegistry) http.HandlerFunc {
	return handleBatchTokensFromProvider(ts, syncer, reg)
}

func handleBatchTokensFromProvider(ts TokenStoreInterface, syncer TokenPoolSyncer, reg *registry.ModelRegistry) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req BatchTokenRequest
		decoder := json.NewDecoder(r.Body)
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&req); err != nil {
			WriteError(w, 400, "invalid_request", "invalid_json",
				"Invalid JSON in request body")
			return
		}

		var resp BatchTokenResponse
		resp.Operation = req.Operation

		switch req.Operation {
		case BatchOpImport:
			resp = handleBatchImport(r.Context(), ts, syncer, req, reg)
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
	Quotas      store.IntMap   `json:"quotas,omitempty"`       // For import: initial quotas (mode -> value)
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

// handleBatchImport imports multiple tokens.
func handleBatchImport(ctx context.Context, ts TokenStoreInterface, syncer TokenPoolSyncer, req BatchTokenRequest, reg *registry.ModelRegistry) BatchTokenResponse {
	resp := BatchTokenResponse{Operation: BatchOpImport}
	pool, err := token.NormalizePoolName(req.Pool)
	if err != nil {
		resp.Failed = len(req.Tokens)
		resp.Errors = append(resp.Errors, BatchError{Message: "invalid pool"})
		return resp
	}

	quotas, limitQuotas, err := buildImportQuotaMaps(pool, req.Quotas, reg)
	if err != nil {
		resp.Failed = len(req.Tokens)
		resp.Errors = append(resp.Errors, BatchError{Message: err.Error()})
		return resp
	}

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

		t := &store.Token{
			Token:       tokenStr,
			Pool:        pool,
			Quotas:      copyIntMap(quotas),
			LimitQuotas: copyIntMap(limitQuotas),
			Priority:    req.Priority,
			Status:      importStatus,
			Remark:      req.Remark,
			NsfwEnabled: req.NsfwEnabled != nil && *req.NsfwEnabled,
		}

		if err := ts.CreateToken(ctx, t); err != nil {
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
			if err := syncer.AddToPool(t); err != nil {
				_ = ts.DeleteToken(ctx, t.ID)
				resp.Failed++
				resp.Errors = append(resp.Errors, BatchError{
					Index:   i,
					Token:   maskSecret(tokenStr),
					Message: "failed to sync token to pool",
				})
				continue
			}
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
			resp.Tokens[i] = tokenToResponse(t, nil)
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

// ptrString returns a pointer to a string.
func ptrString(s string) *string {
	return &s
}

// ptrBool returns a pointer to a bool.
func ptrBool(b bool) *bool {
	return &b
}

func buildImportQuotaMaps(
	pool string,
	overrides store.IntMap,
	reg *registry.ModelRegistry,
) (store.IntMap, store.IntMap, error) {
	if reg == nil {
		quotas := copyIntMap(overrides)
		return quotas, copyIntMap(overrides), nil
	}

	catalogPool := token.PoolToShort(pool)
	quotas := make(store.IntMap)
	limits := make(store.IntMap)
	for _, mode := range reg.SupportedModes(catalogPool) {
		limit := mode.DefaultQuota[catalogPool]
		if limit <= 0 {
			continue
		}
		quotas[mode.ID] = limit
		limits[mode.ID] = limit
	}

	for mode, val := range overrides {
		limit, ok := limits[mode]
		if !ok {
			return nil, nil, errors.New("unknown quota mode: " + mode)
		}
		if val < 0 {
			return nil, nil, errors.New("quota for mode " + mode + " must be >= 0")
		}
		if val > limit {
			return nil, nil, errors.New("quota for mode " + mode + " exceeds limit")
		}
		quotas[mode] = val
	}

	return quotas, limits, nil
}

func copyIntMap(src store.IntMap) store.IntMap {
	if src == nil {
		return nil
	}
	dst := make(store.IntMap, len(src))
	for key, val := range src {
		dst[key] = val
	}
	return dst
}
