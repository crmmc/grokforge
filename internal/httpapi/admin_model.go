package httpapi

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"sync"

	"github.com/crmmc/grokforge/internal/registry"
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

type txCapableModelStore interface {
	BeginTx(ctx context.Context) (*store.ModelStoreTx, error)
}

// FamilyResponse wraps a ModelFamily with its modes for API responses.
type FamilyResponse struct {
	store.ModelFamily
	Modes []*store.ModelMode `json:"modes"`
}

type familyCreateRequest struct {
	Model       string `json:"model"`
	DisplayName string `json:"display_name"`
	Type        string `json:"type"`
	Enabled     *bool  `json:"enabled"`
	PoolFloor   string `json:"pool_floor"`
	Description string `json:"description"`
}

type familyUpdateRequest struct {
	Model         string       `json:"model"`
	DisplayName   string       `json:"display_name"`
	Type          string       `json:"type"`
	Enabled       *bool        `json:"enabled"`
	PoolFloor     string       `json:"pool_floor"`
	DefaultModeID optionalUint `json:"default_mode_id"`
	Description   string       `json:"description"`
}

type modeCreateRequest struct {
	ModelID           uint    `json:"model_id"`
	Mode              string  `json:"mode"`
	Enabled           *bool   `json:"enabled"`
	PoolFloorOverride *string `json:"pool_floor_override"`
	UpstreamMode      string  `json:"upstream_mode"`
	UpstreamModel     string  `json:"upstream_model"`
}

type modeUpdateRequest struct {
	ModelID           uint    `json:"model_id"`
	Mode              string  `json:"mode"`
	Enabled           *bool   `json:"enabled"`
	PoolFloorOverride *string `json:"pool_floor_override"`
	UpstreamMode      string  `json:"upstream_mode"`
	UpstreamModel     string  `json:"upstream_model"`
}

func refreshRegistry(ctx context.Context, reg RegistryRefresher) error {
	if reg == nil {
		return nil
	}
	return reg.Refresh(ctx)
}

var adminModelMu sync.Mutex

func mutateAndRefreshRegistry(
	ctx context.Context,
	ms ModelStoreInterface,
	reg RegistryRefresher,
	mutate func(context.Context, ModelStoreInterface) error,
) error {
	adminModelMu.Lock()
	defer adminModelMu.Unlock()

	txStore, ok := ms.(txCapableModelStore)
	registryRef, registryOK := reg.(*registry.ModelRegistry)
	if !ok || !registryOK || registryRef == nil {
		if err := mutate(ctx, ms); err != nil {
			return err
		}
		return refreshRegistry(ctx, reg)
	}

	txHandle, err := txStore.BeginTx(ctx)
	if err != nil {
		return err
	}
	committed := false
	defer func() {
		if !committed {
			_ = txHandle.Rollback()
		}
	}()

	transactionalStore := txHandle.Store()
	if err := mutate(ctx, transactionalStore); err != nil {
		return err
	}

	snapshot, err := registryRef.BuildSnapshotFromStore(ctx, transactionalStore)
	if err != nil {
		return err
	}
	if err := registryRef.CommitAndApply(txHandle, snapshot); err != nil {
		return err
	}
	committed = true
	return nil
}

func loadFamilyResponse(ctx context.Context, ms ModelStoreInterface, familyID uint) (*FamilyResponse, error) {
	family, err := ms.GetFamily(ctx, familyID)
	if err != nil {
		return nil, err
	}
	modes, err := ms.ListModesByFamily(ctx, family.ID)
	if err != nil {
		return nil, err
	}
	if modes == nil {
		modes = []*store.ModelMode{}
	}
	return &FamilyResponse{ModelFamily: *family, Modes: modes}, nil
}

func writeModelStoreError(w http.ResponseWriter, err error, fallbackCode, fallbackMsg string) {
	switch {
	case errors.Is(err, store.ErrConflict):
		WriteError(w, http.StatusConflict, "conflict", "name_conflict", err.Error())
	case errors.Is(err, store.ErrInvalidInput):
		WriteError(w, http.StatusBadRequest, "invalid_request", "invalid_model_definition", err.Error())
	default:
		WriteError(w, http.StatusInternalServerError, "server_error", fallbackCode, fallbackMsg)
	}
}

func boolValue(v *bool, defaultValue bool) bool {
	if v == nil {
		return defaultValue
	}
	return *v
}

func normalizeRequestIdentifier(value string) string {
	return strings.TrimSpace(value)
}

func normalizeOptionalRequestString(value *string) *string {
	if value == nil {
		return nil
	}
	trimmed := strings.TrimSpace(*value)
	if trimmed == "" {
		return nil
	}
	return &trimmed
}

// handleListFamilies returns all families, each with its modes attached.
func handleListFamilies(ms ModelStoreInterface, _ RegistryRefresher) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		families, err := ms.ListFamilies(r.Context())
		if err != nil {
			WriteError(w, http.StatusInternalServerError, "server_error", "list_failed", "Failed to list model families")
			return
		}
		result := make([]FamilyResponse, 0, len(families))
		for _, family := range families {
			modes, err := ms.ListModesByFamily(r.Context(), family.ID)
			if err != nil {
				WriteError(w, http.StatusInternalServerError, "server_error", "list_failed", "Failed to list model family modes")
				return
			}
			if modes == nil {
				modes = []*store.ModelMode{}
			}
			result = append(result, FamilyResponse{ModelFamily: *family, Modes: modes})
		}
		WriteJSON(w, http.StatusOK, result)
	}
}

// handleCreateFamily creates a new model family.
func handleCreateFamily(ms ModelStoreInterface, reg RegistryRefresher) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req familyCreateRequest
		if err := decodeJSONBodyStrict(r, &req); err != nil {
			WriteError(w, http.StatusBadRequest, "invalid_request", "invalid_json", "Invalid JSON in request body")
			return
		}
		if normalizeRequestIdentifier(req.Model) == "" {
			WriteError(w, http.StatusBadRequest, "invalid_request", "model_required", "Model name is required")
			return
		}
		if normalizeRequestIdentifier(req.Type) == "" {
			WriteError(w, http.StatusBadRequest, "invalid_request", "type_required", "Type is required")
			return
		}
		family := &store.ModelFamily{
			Model:       normalizeRequestIdentifier(req.Model),
			DisplayName: req.DisplayName,
			Type:        normalizeRequestIdentifier(req.Type),
			Enabled:     boolValue(req.Enabled, true),
			PoolFloor:   normalizeRequestIdentifier(req.PoolFloor),
			Description: req.Description,
		}
		if err := mutateAndRefreshRegistry(r.Context(), ms, reg, func(ctx context.Context, txStore ModelStoreInterface) error {
			return txStore.CreateFamily(ctx, family)
		}); err != nil {
			writeModelStoreError(w, err, "create_failed", "Failed to create model family")
			return
		}
		resp, err := loadFamilyResponse(r.Context(), ms, family.ID)
		if err != nil {
			WriteError(w, http.StatusInternalServerError, "server_error", "get_failed", "Failed to load created model family")
			return
		}
		WriteJSON(w, http.StatusCreated, resp)
	}
}

// handleGetFamily returns a single family with its modes.
func handleGetFamily(ms ModelStoreInterface, _ RegistryRefresher) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := parseIDParam(r)
		if err != nil {
			WriteError(w, http.StatusBadRequest, "invalid_request", "invalid_id", "Invalid family ID")
			return
		}
		resp, err := loadFamilyResponse(r.Context(), ms, id)
		if err != nil {
			if errors.Is(err, store.ErrNotFound) {
				WriteError(w, http.StatusNotFound, "not_found", "family_not_found", "Model family not found")
				return
			}
			WriteError(w, http.StatusInternalServerError, "server_error", "get_failed", "Failed to get model family")
			return
		}
		WriteJSON(w, http.StatusOK, resp)
	}
}

// handleUpdateFamily updates an existing model family.
func handleUpdateFamily(ms ModelStoreInterface, reg RegistryRefresher) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := parseIDParam(r)
		if err != nil {
			WriteError(w, http.StatusBadRequest, "invalid_request", "invalid_id", "Invalid family ID")
			return
		}
		existing, err := ms.GetFamily(r.Context(), id)
		if err != nil {
			if errors.Is(err, store.ErrNotFound) {
				WriteError(w, http.StatusNotFound, "not_found", "family_not_found", "Model family not found")
				return
			}
			WriteError(w, http.StatusInternalServerError, "server_error", "get_failed", "Failed to get model family")
			return
		}

		var req familyUpdateRequest
		if err := decodeJSONBodyStrict(r, &req); err != nil {
			WriteError(w, http.StatusBadRequest, "invalid_request", "invalid_json", "Invalid JSON in request body")
			return
		}
		if normalizeRequestIdentifier(req.Model) == "" {
			WriteError(w, http.StatusBadRequest, "invalid_request", "model_required", "Model name is required")
			return
		}
		if normalizeRequestIdentifier(req.Type) == "" {
			WriteError(w, http.StatusBadRequest, "invalid_request", "type_required", "Type is required")
			return
		}

		family := &store.ModelFamily{
			ID:          existing.ID,
			Model:       normalizeRequestIdentifier(req.Model),
			DisplayName: req.DisplayName,
			Type:        normalizeRequestIdentifier(req.Type),
			Enabled:     boolValue(req.Enabled, existing.Enabled),
			PoolFloor:   normalizeRequestIdentifier(req.PoolFloor),
			Description: req.Description,
			CreatedAt:   existing.CreatedAt,
		}
		if req.DefaultModeID.Set {
			family.DefaultModeID = req.DefaultModeID.Value
		} else {
			family.DefaultModeID = existing.DefaultModeID
		}

		if err := mutateAndRefreshRegistry(r.Context(), ms, reg, func(ctx context.Context, txStore ModelStoreInterface) error {
			return txStore.UpdateFamily(ctx, family)
		}); err != nil {
			writeModelStoreError(w, err, "update_failed", "Failed to update model family")
			return
		}
		resp, err := loadFamilyResponse(r.Context(), ms, family.ID)
		if err != nil {
			WriteError(w, http.StatusInternalServerError, "server_error", "get_failed", "Failed to load updated model family")
			return
		}
		WriteJSON(w, http.StatusOK, resp)
	}
}

// handleDeleteFamily deletes a model family and all its modes.
func handleDeleteFamily(ms ModelStoreInterface, reg RegistryRefresher) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := parseIDParam(r)
		if err != nil {
			WriteError(w, http.StatusBadRequest, "invalid_request", "invalid_id", "Invalid family ID")
			return
		}
		if err := mutateAndRefreshRegistry(r.Context(), ms, reg, func(ctx context.Context, txStore ModelStoreInterface) error {
			return txStore.DeleteFamily(ctx, id)
		}); err != nil {
			if errors.Is(err, store.ErrNotFound) {
				WriteError(w, http.StatusNotFound, "not_found", "family_not_found", "Model family not found")
				return
			}
			writeModelStoreError(w, err, "delete_failed", "Failed to delete model family")
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

// handleCreateMode creates a new model mode.
func handleCreateMode(ms ModelStoreInterface, reg RegistryRefresher) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req modeCreateRequest
		if err := decodeJSONBodyStrict(r, &req); err != nil {
			WriteError(w, http.StatusBadRequest, "invalid_request", "invalid_json", "Invalid JSON in request body")
			return
		}
		if req.ModelID == 0 {
			WriteError(w, http.StatusBadRequest, "invalid_request", "model_id_required", "model_id is required")
			return
		}
		if normalizeRequestIdentifier(req.Mode) == "" {
			WriteError(w, http.StatusBadRequest, "invalid_request", "mode_required", "Mode name is required")
			return
		}
		mode := &store.ModelMode{
			ModelID:           req.ModelID,
			Mode:              normalizeRequestIdentifier(req.Mode),
			Enabled:           boolValue(req.Enabled, true),
			PoolFloorOverride: normalizeOptionalRequestString(req.PoolFloorOverride),
			UpstreamMode:      normalizeRequestIdentifier(req.UpstreamMode),
			UpstreamModel:     normalizeRequestIdentifier(req.UpstreamModel),
		}
		if err := mutateAndRefreshRegistry(r.Context(), ms, reg, func(ctx context.Context, txStore ModelStoreInterface) error {
			return txStore.CreateMode(ctx, mode)
		}); err != nil {
			writeModelStoreError(w, err, "create_failed", "Failed to create model mode")
			return
		}
		WriteJSON(w, http.StatusCreated, mode)
	}
}

// handleGetMode returns a single mode by ID.
func handleGetMode(ms ModelStoreInterface) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := parseIDParam(r)
		if err != nil {
			WriteError(w, http.StatusBadRequest, "invalid_request", "invalid_id", "Invalid mode ID")
			return
		}
		mode, err := ms.GetMode(r.Context(), id)
		if err != nil {
			if errors.Is(err, store.ErrNotFound) {
				WriteError(w, http.StatusNotFound, "not_found", "mode_not_found", "Model mode not found")
				return
			}
			WriteError(w, http.StatusInternalServerError, "server_error", "get_failed", "Failed to get model mode")
			return
		}
		WriteJSON(w, http.StatusOK, mode)
	}
}

// handleUpdateMode updates an existing model mode.
func handleUpdateMode(ms ModelStoreInterface, reg RegistryRefresher) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := parseIDParam(r)
		if err != nil {
			WriteError(w, http.StatusBadRequest, "invalid_request", "invalid_id", "Invalid mode ID")
			return
		}
		existing, err := ms.GetMode(r.Context(), id)
		if err != nil {
			if errors.Is(err, store.ErrNotFound) {
				WriteError(w, http.StatusNotFound, "not_found", "mode_not_found", "Model mode not found")
				return
			}
			WriteError(w, http.StatusInternalServerError, "server_error", "get_failed", "Failed to get model mode")
			return
		}

		var req modeUpdateRequest
		if err := decodeJSONBodyStrict(r, &req); err != nil {
			WriteError(w, http.StatusBadRequest, "invalid_request", "invalid_json", "Invalid JSON in request body")
			return
		}
		if req.ModelID == 0 {
			WriteError(w, http.StatusBadRequest, "invalid_request", "model_id_required", "model_id is required")
			return
		}
		if normalizeRequestIdentifier(req.Mode) == "" {
			WriteError(w, http.StatusBadRequest, "invalid_request", "mode_required", "Mode name is required")
			return
		}

		mode := &store.ModelMode{
			ID:                existing.ID,
			ModelID:           req.ModelID,
			Mode:              normalizeRequestIdentifier(req.Mode),
			Enabled:           boolValue(req.Enabled, existing.Enabled),
			PoolFloorOverride: normalizeOptionalRequestString(req.PoolFloorOverride),
			UpstreamMode:      normalizeRequestIdentifier(req.UpstreamMode),
			UpstreamModel:     normalizeRequestIdentifier(req.UpstreamModel),
			CreatedAt:         existing.CreatedAt,
		}
		if err := mutateAndRefreshRegistry(r.Context(), ms, reg, func(ctx context.Context, txStore ModelStoreInterface) error {
			return txStore.UpdateMode(ctx, mode)
		}); err != nil {
			writeModelStoreError(w, err, "update_failed", "Failed to update model mode")
			return
		}
		WriteJSON(w, http.StatusOK, mode)
	}
}

// handleDeleteMode deletes a model mode.
func handleDeleteMode(ms ModelStoreInterface, reg RegistryRefresher) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := parseIDParam(r)
		if err != nil {
			WriteError(w, http.StatusBadRequest, "invalid_request", "invalid_id", "Invalid mode ID")
			return
		}
		if err := mutateAndRefreshRegistry(r.Context(), ms, reg, func(ctx context.Context, txStore ModelStoreInterface) error {
			return txStore.DeleteMode(ctx, id)
		}); err != nil {
			if errors.Is(err, store.ErrNotFound) {
				WriteError(w, http.StatusNotFound, "not_found", "mode_not_found", "Model mode not found")
				return
			}
			writeModelStoreError(w, err, "delete_failed", "Failed to delete model mode")
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}
