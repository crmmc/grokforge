package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"

	"github.com/crmmc/grokforge/internal/store"
)

// ModelStoreInterface defines methods for model family/mode CRUD operations.
type ModelStoreInterface interface {
	ListFamilies(ctx context.Context) ([]*store.ModelFamily, error)
	GetFamily(ctx context.Context, id uint) (*store.ModelFamily, error)
	CreateFamily(ctx context.Context, f *store.ModelFamily) error
	UpdateFamily(ctx context.Context, f *store.ModelFamily) error
	DeleteFamily(ctx context.Context, id uint) error
	ListModesByFamily(ctx context.Context, familyID uint) ([]*store.ModelMode, error)
	GetMode(ctx context.Context, id uint) (*store.ModelMode, error)
	CreateMode(ctx context.Context, m *store.ModelMode) error
	UpdateMode(ctx context.Context, m *store.ModelMode) error
	DeleteMode(ctx context.Context, id uint) error
}

// RegistryRefresher refreshes the in-memory model registry after CRUD ops.
type RegistryRefresher interface {
	Refresh(ctx context.Context) error
}

// FamilyResponse wraps a ModelFamily with its modes for API responses.
type FamilyResponse struct {
	store.ModelFamily
	Modes []*store.ModelMode `json:"modes"`
}

// refreshRegistry calls Refresh on the registry, logging errors but not failing.
func refreshRegistry(ctx context.Context, reg RegistryRefresher) {
	if reg == nil {
		return
	}
	if err := reg.Refresh(ctx); err != nil {
		slog.Error("failed to refresh model registry after CRUD", "error", err)
	}
}

// --- Family handlers ---

// handleListFamilies returns all families, each with its modes attached.
func handleListFamilies(ms ModelStoreInterface, reg RegistryRefresher) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		families, err := ms.ListFamilies(r.Context())
		if err != nil {
			WriteError(w, 500, "server_error", "list_failed", "Failed to list model families")
			return
		}
		result := make([]FamilyResponse, 0, len(families))
		for _, f := range families {
			modes, _ := ms.ListModesByFamily(r.Context(), f.ID)
			if modes == nil {
				modes = []*store.ModelMode{}
			}
			result = append(result, FamilyResponse{ModelFamily: *f, Modes: modes})
		}
		WriteJSON(w, http.StatusOK, result)
	}
}

// handleCreateFamily creates a new model family.
func handleCreateFamily(ms ModelStoreInterface, reg RegistryRefresher) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var f store.ModelFamily
		if err := json.NewDecoder(r.Body).Decode(&f); err != nil {
			WriteError(w, 400, "invalid_request", "invalid_json", "Invalid JSON in request body")
			return
		}
		if f.Model == "" {
			WriteError(w, 400, "invalid_request", "model_required", "Model name is required")
			return
		}
		if f.Type == "" {
			WriteError(w, 400, "invalid_request", "type_required", "Type is required")
			return
		}
		if err := ms.CreateFamily(r.Context(), &f); err != nil {
			if errors.Is(err, store.ErrConflict) {
				WriteError(w, 409, "conflict", "name_conflict", err.Error())
				return
			}
			WriteError(w, 500, "server_error", "create_failed", "Failed to create model family")
			return
		}
		refreshRegistry(r.Context(), reg)
		modes, _ := ms.ListModesByFamily(r.Context(), f.ID)
		if modes == nil {
			modes = []*store.ModelMode{}
		}
		WriteJSON(w, http.StatusCreated, FamilyResponse{ModelFamily: f, Modes: modes})
	}
}

// handleGetFamily returns a single family with its modes.
func handleGetFamily(ms ModelStoreInterface, reg RegistryRefresher) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := parseIDParam(r)
		if err != nil {
			WriteError(w, 400, "invalid_request", "invalid_id", "Invalid family ID")
			return
		}
		f, err := ms.GetFamily(r.Context(), id)
		if err != nil {
			if errors.Is(err, store.ErrNotFound) {
				WriteError(w, 404, "not_found", "family_not_found", "Model family not found")
				return
			}
			WriteError(w, 500, "server_error", "get_failed", "Failed to get model family")
			return
		}
		modes, _ := ms.ListModesByFamily(r.Context(), f.ID)
		if modes == nil {
			modes = []*store.ModelMode{}
		}
		WriteJSON(w, http.StatusOK, FamilyResponse{ModelFamily: *f, Modes: modes})
	}
}

// handleUpdateFamily updates an existing model family.
func handleUpdateFamily(ms ModelStoreInterface, reg RegistryRefresher) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := parseIDParam(r)
		if err != nil {
			WriteError(w, 400, "invalid_request", "invalid_id", "Invalid family ID")
			return
		}
		existing, err := ms.GetFamily(r.Context(), id)
		if err != nil {
			if errors.Is(err, store.ErrNotFound) {
				WriteError(w, 404, "not_found", "family_not_found", "Model family not found")
				return
			}
			WriteError(w, 500, "server_error", "get_failed", "Failed to get model family")
			return
		}
		var f store.ModelFamily
		if err := json.NewDecoder(r.Body).Decode(&f); err != nil {
			WriteError(w, 400, "invalid_request", "invalid_json", "Invalid JSON in request body")
			return
		}
		f.ID = existing.ID
		f.CreatedAt = existing.CreatedAt
		if err := ms.UpdateFamily(r.Context(), &f); err != nil {
			if errors.Is(err, store.ErrConflict) {
				WriteError(w, 409, "conflict", "name_conflict", err.Error())
				return
			}
			WriteError(w, 500, "server_error", "update_failed", "Failed to update model family")
			return
		}
		refreshRegistry(r.Context(), reg)
		modes, _ := ms.ListModesByFamily(r.Context(), f.ID)
		if modes == nil {
			modes = []*store.ModelMode{}
		}
		WriteJSON(w, http.StatusOK, FamilyResponse{ModelFamily: f, Modes: modes})
	}
}

// handleDeleteFamily deletes a model family and all its modes.
func handleDeleteFamily(ms ModelStoreInterface, reg RegistryRefresher) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := parseIDParam(r)
		if err != nil {
			WriteError(w, 400, "invalid_request", "invalid_id", "Invalid family ID")
			return
		}
		if err := ms.DeleteFamily(r.Context(), id); err != nil {
			if errors.Is(err, store.ErrNotFound) {
				WriteError(w, 404, "not_found", "family_not_found", "Model family not found")
				return
			}
			WriteError(w, 500, "server_error", "delete_failed", "Failed to delete model family")
			return
		}
		refreshRegistry(r.Context(), reg)
		w.WriteHeader(http.StatusNoContent)
	}
}

// --- Mode handlers ---

// handleCreateMode creates a new model mode.
func handleCreateMode(ms ModelStoreInterface, reg RegistryRefresher) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var m store.ModelMode
		if err := json.NewDecoder(r.Body).Decode(&m); err != nil {
			WriteError(w, 400, "invalid_request", "invalid_json", "Invalid JSON in request body")
			return
		}
		if m.ModelID == 0 {
			WriteError(w, 400, "invalid_request", "model_id_required", "model_id is required")
			return
		}
		if m.Mode == "" {
			WriteError(w, 400, "invalid_request", "mode_required", "Mode name is required")
			return
		}
		if err := ms.CreateMode(r.Context(), &m); err != nil {
			if errors.Is(err, store.ErrConflict) {
				WriteError(w, 409, "conflict", "name_conflict", err.Error())
				return
			}
			WriteError(w, 500, "server_error", "create_failed", "Failed to create model mode")
			return
		}
		refreshRegistry(r.Context(), reg)
		WriteJSON(w, http.StatusCreated, m)
	}
}

// handleGetMode returns a single mode by ID.
func handleGetMode(ms ModelStoreInterface) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := parseIDParam(r)
		if err != nil {
			WriteError(w, 400, "invalid_request", "invalid_id", "Invalid mode ID")
			return
		}
		m, err := ms.GetMode(r.Context(), id)
		if err != nil {
			if errors.Is(err, store.ErrNotFound) {
				WriteError(w, 404, "not_found", "mode_not_found", "Model mode not found")
				return
			}
			WriteError(w, 500, "server_error", "get_failed", "Failed to get model mode")
			return
		}
		WriteJSON(w, http.StatusOK, m)
	}
}

// handleUpdateMode updates an existing model mode.
func handleUpdateMode(ms ModelStoreInterface, reg RegistryRefresher) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := parseIDParam(r)
		if err != nil {
			WriteError(w, 400, "invalid_request", "invalid_id", "Invalid mode ID")
			return
		}
		existing, err := ms.GetMode(r.Context(), id)
		if err != nil {
			if errors.Is(err, store.ErrNotFound) {
				WriteError(w, 404, "not_found", "mode_not_found", "Model mode not found")
				return
			}
			WriteError(w, 500, "server_error", "get_failed", "Failed to get model mode")
			return
		}
		var m store.ModelMode
		if err := json.NewDecoder(r.Body).Decode(&m); err != nil {
			WriteError(w, 400, "invalid_request", "invalid_json", "Invalid JSON in request body")
			return
		}
		m.ID = existing.ID
		m.CreatedAt = existing.CreatedAt
		if err := ms.UpdateMode(r.Context(), &m); err != nil {
			if errors.Is(err, store.ErrConflict) {
				WriteError(w, 409, "conflict", "name_conflict", err.Error())
				return
			}
			WriteError(w, 500, "server_error", "update_failed", "Failed to update model mode")
			return
		}
		refreshRegistry(r.Context(), reg)
		WriteJSON(w, http.StatusOK, m)
	}
}

// handleDeleteMode deletes a model mode.
func handleDeleteMode(ms ModelStoreInterface, reg RegistryRefresher) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := parseIDParam(r)
		if err != nil {
			WriteError(w, 400, "invalid_request", "invalid_id", "Invalid mode ID")
			return
		}
		if err := ms.DeleteMode(r.Context(), id); err != nil {
			if errors.Is(err, store.ErrNotFound) {
				WriteError(w, 404, "not_found", "mode_not_found", "Model mode not found")
				return
			}
			WriteError(w, 500, "server_error", "delete_failed", "Failed to delete model mode")
			return
		}
		refreshRegistry(r.Context(), reg)
		w.WriteHeader(http.StatusNoContent)
	}
}
