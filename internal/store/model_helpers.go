package store

import (
	"fmt"
	"strings"

	"gorm.io/gorm"
)

func createFamilyRecord(tx *gorm.DB, family *ModelFamily) error {
	return tx.Select(
		"Model",
		"DisplayName",
		"Type",
		"Enabled",
		"PoolFloor",
		"DefaultModeID",
		"QuotaDefault",
		"Description",
	).Create(family).Error
}

func saveFamilyRecord(tx *gorm.DB, family *ModelFamily) error {
	return tx.Select(
		"Model",
		"DisplayName",
		"Type",
		"Enabled",
		"PoolFloor",
		"DefaultModeID",
		"QuotaDefault",
		"Description",
	).Save(family).Error
}

func createModeRecord(tx *gorm.DB, mode *ModelMode) error {
	return tx.Select(
		"ModelID",
		"Mode",
		"Enabled",
		"PoolFloorOverride",
		"UpstreamMode",
		"UpstreamModel",
		"QuotaOverride",
	).Create(mode).Error
}

func saveModeRecord(tx *gorm.DB, mode *ModelMode) error {
	return tx.Select(
		"ModelID",
		"Mode",
		"Enabled",
		"PoolFloorOverride",
		"UpstreamMode",
		"UpstreamModel",
		"QuotaOverride",
	).Save(mode).Error
}

func listModesOrdered(tx *gorm.DB, familyID uint) ([]ModelMode, error) {
	var modes []ModelMode
	err := tx.Where("model_id = ?", familyID).Order("id ASC").Find(&modes).Error
	return modes, err
}

func normalizeIdentifier(value string) string {
	return strings.TrimSpace(value)
}

func normalizeOptionalText(value *string) *string {
	if value == nil {
		return nil
	}
	trimmed := strings.TrimSpace(*value)
	if trimmed == "" {
		return nil
	}
	return &trimmed
}

func normalizeFamilyInput(family *ModelFamily) {
	family.Model = normalizeIdentifier(family.Model)
	family.Type = normalizeIdentifier(family.Type)
	family.PoolFloor = normalizeIdentifier(family.PoolFloor)
	family.QuotaDefault = normalizeOptionalText(family.QuotaDefault)
}

func normalizeModeInput(mode *ModelMode) {
	mode.Mode = normalizeIdentifier(mode.Mode)
	mode.PoolFloorOverride = normalizeOptionalText(mode.PoolFloorOverride)
	mode.UpstreamMode = normalizeIdentifier(mode.UpstreamMode)
	mode.UpstreamModel = normalizeIdentifier(mode.UpstreamModel)
	mode.QuotaOverride = normalizeOptionalText(mode.QuotaOverride)
}

func validateModeUpstream(familyType string, mode *ModelMode) error {
	if !familyRequiresUpstream(familyType) {
		if strings.TrimSpace(mode.UpstreamModel) != "" || strings.TrimSpace(mode.UpstreamMode) != "" {
			return fmt.Errorf("%w: upstream mapping is not supported for type %s", ErrInvalidInput, familyType)
		}
		return nil
	}
	if strings.TrimSpace(mode.UpstreamModel) == "" {
		return fmt.Errorf("%w: upstream_model is required", ErrInvalidInput)
	}
	if strings.TrimSpace(mode.UpstreamMode) == "" {
		return fmt.Errorf("%w: upstream_mode is required", ErrInvalidInput)
	}
	return nil
}

func familyRequiresUpstream(familyType string) bool {
	return familyType != "image"
}

func validateModesForFamilyType(familyType string, modes []ModelMode) error {
	for _, mode := range modes {
		if err := validateModeUpstream(familyType, &mode); err != nil {
			return fmt.Errorf("mode %s: %w", mode.Mode, err)
		}
	}
	return nil
}

func lockModelTables(tx *gorm.DB) error {
	if tx.Dialector.Name() != "postgres" {
		return nil
	}
	return tx.Exec("LOCK TABLE model_families, model_modes IN SHARE ROW EXCLUSIVE MODE").Error
}
