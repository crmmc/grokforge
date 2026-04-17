package store

import (
	"context"
	"embed"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/BurntSushi/toml"
	"gorm.io/gorm"
)

// SeedFile represents the top-level structure of models.seed.toml.
type SeedFile struct {
	Families []SeedFamily `toml:"family"`
}

// SeedFamily represents a model family in the seed file.
type SeedFamily struct {
	Model        string     `toml:"model"`
	DisplayName  string     `toml:"display_name"`
	Type         string     `toml:"type"`
	PoolFloor    string     `toml:"pool_floor"`
	DefaultMode  string     `toml:"default_mode"`
	QuotaDefault string     `toml:"quota_default"`
	Description  string     `toml:"description"`
	Modes        []SeedMode `toml:"mode"`
}

// SeedMode represents a mode variant in the seed file.
type SeedMode struct {
	Mode              string `toml:"mode"`
	UpstreamModel     string `toml:"upstream_model"`
	UpstreamMode      string `toml:"upstream_mode"`
	PoolFloorOverride string `toml:"pool_floor_override,omitempty"`
	QuotaOverride     string `toml:"quota_override"`
}

// SeedModels imports seed data into an empty model_family table.
// If the table already has data, it returns nil without changes.
// It tries configDir/models.seed.toml first, then falls back to fallbackFS.
func SeedModels(ctx context.Context, db *gorm.DB, configDir string, fallbackFS embed.FS) error {
	var count int64
	if err := db.WithContext(ctx).Model(&ModelFamily{}).Count(&count).Error; err != nil {
		return fmt.Errorf("count model families: %w", err)
	}
	if count > 0 {
		return nil
	}

	seed, err := loadSeedData(configDir, fallbackFS)
	if err != nil {
		return fmt.Errorf("load seed data: %w", err)
	}

	return db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		modelStore := NewModelStore(tx)
		for _, sf := range seed.Families {
			if err := importFamily(ctx, modelStore, sf); err != nil {
				return fmt.Errorf("import family %s: %w", sf.Model, err)
			}
		}
		slog.Info("seed models imported", "families", len(seed.Families))
		return nil
	})
}

// loadSeedData reads the seed file from configDir or falls back to embed.FS.
func loadSeedData(configDir string, fallbackFS embed.FS) (*SeedFile, error) {
	if configDir != "" {
		externalPath := filepath.Join(configDir, "models.seed.toml")
		if _, err := os.Stat(externalPath); err == nil {
			var seed SeedFile
			if _, err := toml.DecodeFile(externalPath, &seed); err != nil {
				return nil, fmt.Errorf("decode external seed %s: %w", externalPath, err)
			}
			slog.Info("loaded external seed file", "path", externalPath)
			return &seed, nil
		} else if !os.IsNotExist(err) {
			return nil, fmt.Errorf("stat external seed %s: %w", externalPath, err)
		}
	}

	var seed SeedFile
	if _, err := toml.DecodeFS(fallbackFS, "models.seed.toml", &seed); err != nil {
		return nil, fmt.Errorf("decode embedded seed: %w", err)
	}
	return &seed, nil
}

// importFamily creates a family and its modes using the same validation path as admin CRUD.
func importFamily(ctx context.Context, modelStore *ModelStore, sf SeedFamily) error {
	family := &ModelFamily{
		Model:       sf.Model,
		DisplayName: sf.DisplayName,
		Type:        sf.Type,
		PoolFloor:   sf.PoolFloor,
		Enabled:     true,
		Description: sf.Description,
	}
	if sf.QuotaDefault != "" {
		family.QuotaDefault = &sf.QuotaDefault
	}
	if err := modelStore.CreateFamily(ctx, family); err != nil {
		return fmt.Errorf("create family: %w", err)
	}

	var defaultModeID *uint
	for _, sm := range sf.Modes {
		mode := &ModelMode{
			ModelID:       family.ID,
			Mode:          sm.Mode,
			Enabled:       true,
			UpstreamModel: sm.UpstreamModel,
			UpstreamMode:  sm.UpstreamMode,
		}
		if sm.PoolFloorOverride != "" {
			override := sm.PoolFloorOverride
			mode.PoolFloorOverride = &override
		}
		if sm.QuotaOverride != "" {
			override := sm.QuotaOverride
			mode.QuotaOverride = &override
		}
		if err := modelStore.CreateMode(ctx, mode); err != nil {
			return fmt.Errorf("create mode %s: %w", sm.Mode, err)
		}
		if sm.Mode == sf.DefaultMode {
			defaultModeID = &mode.ID
		}
	}

	if defaultModeID == nil && len(sf.Modes) > 0 {
		return fmt.Errorf("default_mode %q not found in family modes", sf.DefaultMode)
	}
	family.DefaultModeID = defaultModeID
	if err := modelStore.UpdateFamily(ctx, family); err != nil {
		return fmt.Errorf("set default_mode_id: %w", err)
	}
	return nil
}
