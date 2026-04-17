package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"gorm.io/gorm"
)

var validTypes = map[string]struct{}{
	"chat": {}, "image": {}, "image_edit": {}, "video": {},
}

var validPoolFloors = map[string]struct{}{
	"basic": {}, "super": {}, "heavy": {},
}

// ModelStore provides CRUD operations for ModelFamily and ModelMode.
type ModelStore struct {
	db *gorm.DB
}

// NewModelStore creates a new ModelStore.
func NewModelStore(db *gorm.DB) *ModelStore {
	return &ModelStore{db: db}
}

// DeriveRequestName computes the request name for a mode.
// Default mode returns familyModel; non-default returns familyModel + "-" + mode.
func DeriveRequestName(familyModel string, mode string, isDefault bool) string {
	if isDefault {
		return familyModel
	}
	return familyModel + "-" + mode
}

// --- Family CRUD ---

// GetFamily returns a model family by ID.
func (s *ModelStore) GetFamily(ctx context.Context, id uint) (*ModelFamily, error) {
	var f ModelFamily
	if err := s.db.WithContext(ctx).First(&f, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &f, nil
}

// ListFamilies returns all model families.
func (s *ModelStore) ListFamilies(ctx context.Context) ([]*ModelFamily, error) {
	var families []*ModelFamily
	err := s.db.WithContext(ctx).Find(&families).Error
	return families, err
}

// ListEnabledFamilies returns all enabled model families.
func (s *ModelStore) ListEnabledFamilies(ctx context.Context) ([]*ModelFamily, error) {
	var families []*ModelFamily
	err := s.db.WithContext(ctx).Where("enabled = ?", true).Find(&families).Error
	return families, err
}

// CreateFamily creates a new model family with conflict checking.
func (s *ModelStore) CreateFamily(ctx context.Context, f *ModelFamily) error {
	normalizeFamilyInput(f)
	if f.Model == "" {
		return fmt.Errorf("%w: model is required", ErrInvalidInput)
	}
	if f.Type == "" {
		return fmt.Errorf("%w: type is required", ErrInvalidInput)
	}
	if f.PoolFloor == "" {
		return fmt.Errorf("%w: pool_floor is required", ErrInvalidInput)
	}
	if _, ok := validTypes[f.Type]; !ok {
		return fmt.Errorf("%w: invalid type %q", ErrInvalidInput, f.Type)
	}
	if _, ok := validPoolFloors[f.PoolFloor]; !ok {
		return fmt.Errorf("%w: invalid pool_floor %q", ErrInvalidInput, f.PoolFloor)
	}
	if f.DefaultModeID != nil {
		return fmt.Errorf("%w: default_mode_id cannot be set when creating a family", ErrInvalidInput)
	}
	if err := validateNullableJSON("quota_default", f.QuotaDefault); err != nil {
		return err
	}
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := lockModelTables(tx); err != nil {
			return err
		}
		newNames := map[string]struct{}{f.Model: {}}
		if err := s.checkConflict(tx, ctx, newNames, 0, 0); err != nil {
			return err
		}
		return createFamilyRecord(tx, f)
	})
}

// UpdateFamily updates an existing model family with conflict checking.
func (s *ModelStore) UpdateFamily(ctx context.Context, f *ModelFamily) error {
	normalizeFamilyInput(f)
	if f.Model == "" {
		return fmt.Errorf("%w: model is required", ErrInvalidInput)
	}
	if f.Type == "" {
		return fmt.Errorf("%w: type is required", ErrInvalidInput)
	}
	if f.PoolFloor == "" {
		return fmt.Errorf("%w: pool_floor is required", ErrInvalidInput)
	}
	if _, ok := validTypes[f.Type]; !ok {
		return fmt.Errorf("%w: invalid type %q", ErrInvalidInput, f.Type)
	}
	if _, ok := validPoolFloors[f.PoolFloor]; !ok {
		return fmt.Errorf("%w: invalid pool_floor %q", ErrInvalidInput, f.PoolFloor)
	}
	if err := validateNullableJSON("quota_default", f.QuotaDefault); err != nil {
		return err
	}
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := lockModelTables(tx); err != nil {
			return err
		}
		// Validate DefaultModeID belongs to this family (D-04)
		if f.DefaultModeID != nil {
			var mode ModelMode
			if err := tx.First(&mode, *f.DefaultModeID).Error; err != nil {
				if errors.Is(err, gorm.ErrRecordNotFound) {
					return fmt.Errorf("%w: default_mode_id %d not found", ErrInvalidInput, *f.DefaultModeID)
				}
				return err
			}
			if mode.ModelID != f.ID {
				return fmt.Errorf("%w: default_mode_id %d does not belong to family %d", ErrInvalidInput, *f.DefaultModeID, f.ID)
			}
			if !mode.Enabled {
				return fmt.Errorf("%w: default_mode_id %d must reference an enabled mode", ErrInvalidInput, *f.DefaultModeID)
			}
		}

		// Compute all derived names for this family
		newNames := map[string]struct{}{f.Model: {}}
		modes, err := listModesOrdered(tx, f.ID)
		if err != nil {
			return err
		}
		if len(modes) > 0 && f.DefaultModeID == nil {
			return fmt.Errorf("%w: default_mode_id is required when a family has modes", ErrInvalidInput)
		}
		if err := validateModesForFamilyType(f.Type, modes); err != nil {
			return fmt.Errorf("%w: %s", ErrInvalidInput, err)
		}
		for _, m := range modes {
			isDefault := f.DefaultModeID != nil && *f.DefaultModeID == m.ID
			name := DeriveRequestName(f.Model, m.Mode, isDefault)
			newNames[name] = struct{}{}
		}

		if err := s.checkConflict(tx, ctx, newNames, f.ID, 0); err != nil {
			return err
		}
		return saveFamilyRecord(tx, f)
	})
}

// DeleteFamily deletes a model family and all its modes.
func (s *ModelStore) DeleteFamily(ctx context.Context, id uint) error {
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := lockModelTables(tx); err != nil {
			return err
		}
		if err := tx.Model(&ModelFamily{}).
			Where("id = ?", id).
			Update("default_mode_id", nil).Error; err != nil {
			return err
		}
		// Delete all modes belonging to this family
		if err := tx.Where("model_id = ?", id).Delete(&ModelMode{}).Error; err != nil {
			return err
		}
		result := tx.Delete(&ModelFamily{}, id)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			return ErrNotFound
		}
		return nil
	})
}

// --- Mode CRUD ---

// GetMode returns a model mode by ID.
func (s *ModelStore) GetMode(ctx context.Context, id uint) (*ModelMode, error) {
	var m ModelMode
	if err := s.db.WithContext(ctx).First(&m, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &m, nil
}

// ListModesByFamily returns all modes for a given family.
func (s *ModelStore) ListModesByFamily(ctx context.Context, familyID uint) ([]*ModelMode, error) {
	var modes []*ModelMode
	err := s.db.WithContext(ctx).Where("model_id = ?", familyID).Find(&modes).Error
	return modes, err
}

// CreateMode creates a new model mode with conflict checking.
func (s *ModelStore) CreateMode(ctx context.Context, m *ModelMode) error {
	normalizeModeInput(m)
	if m.Mode == "" {
		return fmt.Errorf("%w: mode is required", ErrInvalidInput)
	}
	if m.PoolFloorOverride != nil {
		if _, ok := validPoolFloors[*m.PoolFloorOverride]; !ok {
			return fmt.Errorf("%w: invalid pool_floor_override %q", ErrInvalidInput, *m.PoolFloorOverride)
		}
	}
	if err := validateNullableJSON("quota_override", m.QuotaOverride); err != nil {
		return err
	}
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := lockModelTables(tx); err != nil {
			return err
		}
		// Verify family exists
		var family ModelFamily
		if err := tx.First(&family, m.ModelID).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return fmt.Errorf("%w: family %d not found", ErrInvalidInput, m.ModelID)
			}
			return err
		}
		if err := validateModeUpstream(family.Type, m); err != nil {
			return err
		}
		existingModes, err := listModesOrdered(tx, family.ID)
		if err != nil {
			return err
		}
		becomesDefault := len(existingModes) == 0
		if becomesDefault && family.DefaultModeID != nil {
			return fmt.Errorf("%w: family %d has default_mode_id but no modes", ErrInvalidInput, family.ID)
		}
		if len(existingModes) > 0 && family.DefaultModeID == nil {
			return fmt.Errorf("%w: family %d is missing default_mode_id", ErrInvalidInput, family.ID)
		}
		if becomesDefault && m.Mode != "default" {
			return fmt.Errorf("%w: the first mode must be named default", ErrInvalidInput)
		}
		if becomesDefault && !m.Enabled {
			return fmt.Errorf("%w: the first mode must be enabled", ErrInvalidInput)
		}
		newNames := map[string]struct{}{
			DeriveRequestName(family.Model, m.Mode, becomesDefault): {},
		}
		if err := s.checkConflict(tx, ctx, newNames, family.ID, 0); err != nil {
			return err
		}
		if err := createModeRecord(tx, m); err != nil {
			return err
		}
		if !becomesDefault {
			return nil
		}
		family.DefaultModeID = &m.ID
		return saveFamilyRecord(tx, &family)
	})
}

// UpdateMode updates an existing model mode with conflict checking.
func (s *ModelStore) UpdateMode(ctx context.Context, m *ModelMode) error {
	normalizeModeInput(m)
	if m.Mode == "" {
		return fmt.Errorf("%w: mode is required", ErrInvalidInput)
	}
	if m.PoolFloorOverride != nil {
		if _, ok := validPoolFloors[*m.PoolFloorOverride]; !ok {
			return fmt.Errorf("%w: invalid pool_floor_override %q", ErrInvalidInput, *m.PoolFloorOverride)
		}
	}
	if err := validateNullableJSON("quota_override", m.QuotaOverride); err != nil {
		return err
	}
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := lockModelTables(tx); err != nil {
			return err
		}
		var existing ModelMode
		if err := tx.First(&existing, m.ID).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrNotFound
			}
			return err
		}
		if existing.ModelID != m.ModelID {
			return fmt.Errorf("%w: moving a mode to another family is not supported", ErrInvalidInput)
		}

		var family ModelFamily
		if err := tx.First(&family, m.ModelID).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return fmt.Errorf("%w: family %d not found", ErrInvalidInput, m.ModelID)
			}
			return err
		}
		if err := validateModeUpstream(family.Type, m); err != nil {
			return err
		}
		if family.DefaultModeID != nil && *family.DefaultModeID == m.ID && !m.Enabled {
			return fmt.Errorf("%w: cannot disable the current default mode", ErrInvalidInput)
		}
		isDefault := family.DefaultModeID != nil && *family.DefaultModeID == m.ID
		newNames := map[string]struct{}{
			DeriveRequestName(family.Model, m.Mode, isDefault): {},
		}
		if err := s.checkConflict(tx, ctx, newNames, 0, m.ID); err != nil {
			return err
		}
		return saveModeRecord(tx, m)
	})
}

// DeleteMode deletes a model mode and clears default_mode_id if referenced.
func (s *ModelStore) DeleteMode(ctx context.Context, id uint) error {
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := lockModelTables(tx); err != nil {
			return err
		}
		var mode ModelMode
		if err := tx.First(&mode, id).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrNotFound
			}
			return err
		}
		var family ModelFamily
		if err := tx.First(&family, mode.ModelID).Error; err != nil {
			return err
		}
		if family.DefaultModeID != nil && *family.DefaultModeID == id {
			var remaining int64
			if err := tx.Model(&ModelMode{}).
				Where("model_id = ? AND id <> ?", mode.ModelID, id).
				Count(&remaining).Error; err != nil {
				return err
			}
			if remaining > 0 {
				return fmt.Errorf("%w: cannot delete the default mode while other modes exist", ErrInvalidInput)
			}
		}
		// Clear default_mode_id on any family referencing this mode
		if err := tx.Model(&ModelFamily{}).
			Where("default_mode_id = ?", id).
			Update("default_mode_id", nil).Error; err != nil {
			return err
		}
		result := tx.Delete(&ModelMode{}, id)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			return ErrNotFound
		}
		return nil
	})
}

func validateNullableJSON(name string, value *string) error {
	if value == nil {
		return nil
	}
	trimmed := strings.TrimSpace(*value)
	if trimmed == "" {
		return nil
	}
	var raw json.RawMessage
	if err := json.Unmarshal([]byte(trimmed), &raw); err != nil {
		return fmt.Errorf("%w: %s must be valid JSON", ErrInvalidInput, name)
	}
	return nil
}

// --- Conflict checking ---

// checkConflict verifies that newNames don't conflict with existing derived request names.
// excludeFamilyID and excludeModeID are used during updates to exclude the record being updated.
// Must be called within a transaction.
func (s *ModelStore) checkConflict(tx *gorm.DB, ctx context.Context,
	newNames map[string]struct{}, excludeFamilyID, excludeModeID uint) error {

	var families []ModelFamily
	if err := tx.WithContext(ctx).Find(&families).Error; err != nil {
		return err
	}
	var modes []ModelMode
	if err := tx.WithContext(ctx).Find(&modes).Error; err != nil {
		return err
	}

	// Build existing request name set
	existing := make(map[string]struct{})
	for _, f := range families {
		if f.ID == excludeFamilyID {
			continue
		}
		if excludeModeID != 0 && f.DefaultModeID != nil && *f.DefaultModeID == excludeModeID {
			continue
		}
		existing[f.Model] = struct{}{}
	}
	for _, m := range modes {
		if m.ID == excludeModeID {
			continue
		}
		// When excluding a family, also skip all modes belonging to it
		if excludeFamilyID != 0 && m.ModelID == excludeFamilyID {
			continue
		}
		var familyModel string
		for _, f := range families {
			if f.ID == m.ModelID {
				familyModel = f.Model
				break
			}
		}
		if familyModel == "" {
			continue
		}
		isDefault := false
		for _, f := range families {
			if f.ID == m.ModelID && f.DefaultModeID != nil && *f.DefaultModeID == m.ID {
				isDefault = true
				break
			}
		}
		name := DeriveRequestName(familyModel, m.Mode, isDefault)
		existing[name] = struct{}{}
	}

	for name := range newNames {
		if _, conflict := existing[name]; conflict {
			return fmt.Errorf("request name %q conflicts with existing model: %w", name, ErrConflict)
		}
	}
	return nil
}
