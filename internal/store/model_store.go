package store

import (
	"context"
	"errors"
	"fmt"

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
	if _, ok := validTypes[f.Type]; !ok {
		return fmt.Errorf("invalid type %q", f.Type)
	}
	if _, ok := validPoolFloors[f.PoolFloor]; !ok {
		return fmt.Errorf("invalid pool_floor %q", f.PoolFloor)
	}
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		newNames := map[string]struct{}{f.Model: {}}
		if err := s.checkConflict(tx, ctx, newNames, 0, 0); err != nil {
			return err
		}
		return tx.Create(f).Error
	})
}
// UpdateFamily updates an existing model family with conflict checking.
func (s *ModelStore) UpdateFamily(ctx context.Context, f *ModelFamily) error {
	if _, ok := validTypes[f.Type]; !ok {
		return fmt.Errorf("invalid type %q", f.Type)
	}
	if _, ok := validPoolFloors[f.PoolFloor]; !ok {
		return fmt.Errorf("invalid pool_floor %q", f.PoolFloor)
	}
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// Validate DefaultModeID belongs to this family (D-04)
		if f.DefaultModeID != nil {
			var mode ModelMode
			if err := tx.First(&mode, *f.DefaultModeID).Error; err != nil {
				if errors.Is(err, gorm.ErrRecordNotFound) {
					return fmt.Errorf("default_mode_id %d not found", *f.DefaultModeID)
				}
				return err
			}
			if mode.ModelID != f.ID {
				return fmt.Errorf("default_mode_id %d does not belong to family %d", *f.DefaultModeID, f.ID)
			}
		}

		// Compute all derived names for this family
		newNames := map[string]struct{}{f.Model: {}}
		var modes []ModelMode
		if err := tx.Where("model_id = ?", f.ID).Find(&modes).Error; err != nil {
			return err
		}
		for _, m := range modes {
			isDefault := f.DefaultModeID != nil && *f.DefaultModeID == m.ID
			name := DeriveRequestName(f.Model, m.Mode, isDefault)
			newNames[name] = struct{}{}
		}

		if err := s.checkConflict(tx, ctx, newNames, f.ID, 0); err != nil {
			return err
		}
		return tx.Save(f).Error
	})
}
// DeleteFamily deletes a model family and all its modes.
func (s *ModelStore) DeleteFamily(ctx context.Context, id uint) error {
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
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
	if m.PoolFloorOverride != nil {
		if _, ok := validPoolFloors[*m.PoolFloorOverride]; !ok {
			return fmt.Errorf("invalid pool_floor_override %q", *m.PoolFloorOverride)
		}
	}
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// Verify family exists
		var family ModelFamily
		if err := tx.First(&family, m.ModelID).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return fmt.Errorf("family %d not found", m.ModelID)
			}
			return err
		}
		// New mode cannot be default yet, so derive name = family.Model + "-" + m.Mode
		newNames := map[string]struct{}{
			DeriveRequestName(family.Model, m.Mode, false): {},
		}
		if err := s.checkConflict(tx, ctx, newNames, 0, 0); err != nil {
			return err
		}
		return tx.Create(m).Error
	})
}

// UpdateMode updates an existing model mode with conflict checking.
func (s *ModelStore) UpdateMode(ctx context.Context, m *ModelMode) error {
	if m.PoolFloorOverride != nil {
		if _, ok := validPoolFloors[*m.PoolFloorOverride]; !ok {
			return fmt.Errorf("invalid pool_floor_override %q", *m.PoolFloorOverride)
		}
	}
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var family ModelFamily
		if err := tx.First(&family, m.ModelID).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return fmt.Errorf("family %d not found", m.ModelID)
			}
			return err
		}
		isDefault := family.DefaultModeID != nil && *family.DefaultModeID == m.ID
		newNames := map[string]struct{}{
			DeriveRequestName(family.Model, m.Mode, isDefault): {},
		}
		if err := s.checkConflict(tx, ctx, newNames, 0, m.ID); err != nil {
			return err
		}
		return tx.Save(m).Error
	})
}
// DeleteMode deletes a model mode and clears default_mode_id if referenced.
func (s *ModelStore) DeleteMode(ctx context.Context, id uint) error {
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
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
