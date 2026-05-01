package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"

	"github.com/crmmc/grokforge/internal/store"
)

// BatchRefreshRequest is the request body for batch token refresh.
type BatchRefreshRequest struct {
	IDs []uint `json:"ids,omitempty"` // empty = refresh all active tokens
}

// BatchRefreshEvent is a single SSE progress event for batch refresh.
type BatchRefreshEvent struct {
	Type    string `json:"type"`               // "progress" or "complete"
	TokenID uint   `json:"token_id,omitempty"` // only for progress events
	Status  string `json:"status,omitempty"`   // "success" or "error"
	Error   string `json:"error,omitempty"`    // only when status=error
	Current int    `json:"current"`
	Total   int    `json:"total"`
	Success int    `json:"success,omitempty"` // only for complete events
	Failed  int    `json:"failed,omitempty"`  // only for complete events
}

// handleBatchRefresh refreshes quota for multiple tokens with SSE progress streaming.
func handleBatchRefresh(ts TokenStoreInterface, refresher TokenRefresher) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if refresher == nil {
			WriteError(w, http.StatusNotImplemented, "server_error", "refresh_not_configured",
				"Token refresh not configured")
			return
		}

		ids, ok := resolveRefreshIDs(w, r, ts)
		if !ok {
			return
		}

		streamBatchRefresh(r.Context(), NewSSEWriter(w), refresher, ids)
	}
}

// resolveRefreshIDs parses the request body and returns the token IDs to refresh.
// Returns false if an HTTP error was written.
func resolveRefreshIDs(w http.ResponseWriter, r *http.Request, ts TokenStoreInterface) ([]uint, bool) {
	var req BatchRefreshRequest
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&req); err != nil && !errors.Is(err, io.EOF) {
		WriteError(w, http.StatusBadRequest, "invalid_request", "invalid_json",
			"Invalid JSON in request body")
		return nil, false
	}

	if len(req.IDs) > 0 {
		return req.IDs, true
	}

	status := store.TokenStatusActive
	ids, err := ts.ListTokenIDs(r.Context(), store.TokenFilter{Status: &status})
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "server_error", "list_failed",
			"Failed to list tokens")
		return nil, false
	}

	if len(ids) == 0 {
		WriteError(w, http.StatusBadRequest, "invalid_request", "no_tokens",
			"No tokens to refresh")
		return nil, false
	}

	return ids, true
}

// streamBatchRefresh sends SSE progress events for each token refresh.
func streamBatchRefresh(ctx context.Context, sse *SSEWriter, refresher TokenRefresher, ids []uint) {
	total := len(ids)
	var success, failed int

	for i, id := range ids {
		if ctx.Err() != nil {
			slog.Info("batch refresh: client disconnected",
				"processed", i, "total", total)
			return
		}

		evt := BatchRefreshEvent{
			Type:    "progress",
			TokenID: id,
			Current: i + 1,
			Total:   total,
		}

		if _, err := refresher.RefreshToken(ctx, id); err != nil {
			evt.Status = "error"
			evt.Error = err.Error()
			failed++
		} else {
			evt.Status = "success"
			success++
		}

		_ = sse.WriteSSE(evt)
	}

	_ = sse.WriteSSE(BatchRefreshEvent{
		Type:    "complete",
		Current: total,
		Total:   total,
		Success: success,
		Failed:  failed,
	})
	sse.WriteSSEDone()
}
