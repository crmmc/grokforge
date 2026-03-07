package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/crmmc/grokforge/internal/store"
	"github.com/go-chi/chi/v5"
)

// APIKeyStoreInterface defines methods for API key operations.
type APIKeyStoreInterface interface {
	List(ctx context.Context, page, pageSize int, status string) ([]*store.APIKey, int64, error)
	GetByID(ctx context.Context, id uint) (*store.APIKey, error)
	GetByKey(ctx context.Context, key string) (*store.APIKey, error)
	Create(ctx context.Context, ak *store.APIKey) error
	Update(ctx context.Context, ak *store.APIKey) error
	Delete(ctx context.Context, id uint) error
	Regenerate(ctx context.Context, id uint) (string, error)
	CountByStatus(ctx context.Context) (total, active, inactive, expired, rateLimited int, err error)
	IncrementUsage(ctx context.Context, id uint) error
	ResetDailyUsage(ctx context.Context) error
}

// PaginatedResponse is a generic paginated response.
type PaginatedResponse struct {
	Data       any   `json:"data"`
	Total      int64 `json:"total"`
	Page       int   `json:"page"`
	PageSize   int   `json:"page_size"`
	TotalPages int   `json:"total_pages"`
}

// APIKeyCreateResponse is the response when creating or regenerating an API key.
type APIKeyCreateResponse struct {
	ID   uint   `json:"id"`
	Key  string `json:"key"`
	Name string `json:"name"`
}

// APIKeyCreateRequest is the request body for creating an API key.
type APIKeyCreateRequest struct {
	Name           string            `json:"name"`
	ModelWhitelist store.StringSlice `json:"model_whitelist,omitempty"`
	RateLimit      *int              `json:"rate_limit,omitempty"`
	DailyLimit     *int              `json:"daily_limit,omitempty"`
	ExpiresAt      *string           `json:"expires_at,omitempty"`
}

// APIKeyUpdateRequest is the request body for updating an API key.
type APIKeyUpdateRequest struct {
	Name           *string            `json:"name,omitempty"`
	Status         *string            `json:"status,omitempty"`
	ModelWhitelist *store.StringSlice `json:"model_whitelist,omitempty"`
	RateLimit      *int               `json:"rate_limit,omitempty"`
	DailyLimit     *int               `json:"daily_limit,omitempty"`
	ExpiresAt      *string            `json:"expires_at,omitempty"`
}

// APIKeyStatsResponse is the response for API key stats endpoint.
type APIKeyStatsResponse struct {
	Total       int `json:"total"`
	Active      int `json:"active"`
	Inactive    int `json:"inactive"`
	Expired     int `json:"expired"`
	RateLimited int `json:"rate_limited"`
}

// handleListAPIKeys returns paginated API keys with optional status filter.
func handleListAPIKeys(aks APIKeyStoreInterface) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
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
		status := r.URL.Query().Get("status")

		keys, total, err := aks.List(r.Context(), page, pageSize, status)
		if err != nil {
			WriteError(w, 500, "server_error", "list_failed", "Failed to list API keys")
			return
		}

		totalPages := 0
		if total > 0 {
			totalPages = int((total + int64(pageSize) - 1) / int64(pageSize))
		}

		WriteJSON(w, http.StatusOK, PaginatedResponse{
			Data:       keys,
			Total:      total,
			Page:       page,
			PageSize:   pageSize,
			TotalPages: totalPages,
		})
	}
}

// handleGetAPIKey returns a single API key by ID.
func handleGetAPIKey(aks APIKeyStoreInterface) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := parseIDParam(r)
		if err != nil {
			WriteError(w, 400, "invalid_request", "invalid_id", "Invalid API key ID")
			return
		}

		ak, err := aks.GetByID(r.Context(), id)
		if err != nil {
			if errors.Is(err, store.ErrNotFound) {
				WriteError(w, 404, "not_found", "apikey_not_found", "API key not found")
				return
			}
			WriteError(w, 500, "server_error", "get_failed", "Failed to get API key")
			return
		}

		WriteJSON(w, http.StatusOK, ak)
	}
}

// handleCreateAPIKey creates a new API key and returns {id, key, name}.
func handleCreateAPIKey(aks APIKeyStoreInterface) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req APIKeyCreateRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			WriteError(w, 400, "invalid_request", "invalid_json", "Invalid JSON in request body")
			return
		}

		if req.Name == "" {
			WriteError(w, 400, "invalid_request", "name_required", "Name is required")
			return
		}

		ak := &store.APIKey{
			Name:           req.Name,
			ModelWhitelist: req.ModelWhitelist,
		}
		if req.RateLimit != nil {
			ak.RateLimit = *req.RateLimit
		}
		if req.DailyLimit != nil {
			ak.DailyLimit = *req.DailyLimit
		}
		if req.ExpiresAt != nil {
			t, err := time.Parse(time.RFC3339, *req.ExpiresAt)
			if err != nil {
				WriteError(w, 400, "invalid_request", "invalid_expires_at", "expires_at must be RFC3339 format")
				return
			}
			ak.ExpiresAt = &t
		}

		if err := aks.Create(r.Context(), ak); err != nil {
			WriteError(w, 500, "server_error", "create_failed", "Failed to create API key")
			return
		}

		WriteJSON(w, http.StatusCreated, APIKeyCreateResponse{
			ID:   ak.ID,
			Key:  ak.Key,
			Name: ak.Name,
		})
	}
}

// handleUpdateAPIKey updates an existing API key (partial update via PATCH).
func handleUpdateAPIKey(aks APIKeyStoreInterface) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := parseIDParam(r)
		if err != nil {
			WriteError(w, 400, "invalid_request", "invalid_id", "Invalid API key ID")
			return
		}

		ak, err := aks.GetByID(r.Context(), id)
		if err != nil {
			if errors.Is(err, store.ErrNotFound) {
				WriteError(w, 404, "not_found", "apikey_not_found", "API key not found")
				return
			}
			WriteError(w, 500, "server_error", "get_failed", "Failed to get API key")
			return
		}

		var req APIKeyUpdateRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			WriteError(w, 400, "invalid_request", "invalid_json", "Invalid JSON in request body")
			return
		}

		if req.Name != nil {
			ak.Name = *req.Name
		}
		if req.Status != nil {
			ak.Status = *req.Status
		}
		if req.ModelWhitelist != nil {
			ak.ModelWhitelist = *req.ModelWhitelist
		}
		if req.RateLimit != nil {
			ak.RateLimit = *req.RateLimit
		}
		if req.DailyLimit != nil {
			ak.DailyLimit = *req.DailyLimit
		}
		if req.ExpiresAt != nil {
			t, err := time.Parse(time.RFC3339, *req.ExpiresAt)
			if err != nil {
				WriteError(w, 400, "invalid_request", "invalid_expires_at", "expires_at must be RFC3339 format")
				return
			}
			ak.ExpiresAt = &t
		}

		if err := aks.Update(r.Context(), ak); err != nil {
			WriteError(w, 500, "server_error", "update_failed", "Failed to update API key")
			return
		}

		WriteJSON(w, http.StatusOK, ak)
	}
}

// handleDeleteAPIKey soft-deletes an API key by ID.
func handleDeleteAPIKey(aks APIKeyStoreInterface) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := parseIDParam(r)
		if err != nil {
			WriteError(w, 400, "invalid_request", "invalid_id", "Invalid API key ID")
			return
		}

		if err := aks.Delete(r.Context(), id); err != nil {
			WriteError(w, 500, "server_error", "delete_failed", "Failed to delete API key")
			return
		}

		w.WriteHeader(http.StatusNoContent)
	}
}

// handleRegenerateAPIKey generates a new key value for an existing API key.
func handleRegenerateAPIKey(aks APIKeyStoreInterface) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := parseIDParam(r)
		if err != nil {
			WriteError(w, 400, "invalid_request", "invalid_id", "Invalid API key ID")
			return
		}

		ak, err := aks.GetByID(r.Context(), id)
		if err != nil {
			if errors.Is(err, store.ErrNotFound) {
				WriteError(w, 404, "not_found", "apikey_not_found", "API key not found")
				return
			}
			WriteError(w, 500, "server_error", "get_failed", "Failed to get API key")
			return
		}

		newKey, err := aks.Regenerate(r.Context(), id)
		if err != nil {
			WriteError(w, 500, "server_error", "regenerate_failed", "Failed to regenerate API key")
			return
		}

		WriteJSON(w, http.StatusOK, APIKeyCreateResponse{
			ID:   ak.ID,
			Key:  newKey,
			Name: ak.Name,
		})
	}
}

// handleAPIKeyStats returns real counts of API keys by status.
func handleAPIKeyStats(aks APIKeyStoreInterface) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		total, active, inactive, expired, rateLimited, err := aks.CountByStatus(r.Context())
		if err != nil {
			WriteError(w, 500, "server_error", "stats_failed", "Failed to get API key stats")
			return
		}

		WriteJSON(w, http.StatusOK, APIKeyStatsResponse{
			Total:       total,
			Active:      active,
			Inactive:    inactive,
			Expired:     expired,
			RateLimited: rateLimited,
		})
	}
}

// parseIDParam extracts and parses the "id" URL parameter.
func parseIDParam(r *http.Request) (uint, error) {
	idStr := chi.URLParam(r, "id")
	id, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		return 0, err
	}
	return uint(id), nil
}
