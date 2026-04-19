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
	Model         string     `toml:"model"`
	DisplayName   string     `toml:"display_name"`
	Type          string     `toml:"type"`
	PoolFloor     string     `toml:"pool_floor"`
	UpstreamModel string     `toml:"upstream_model"`
	Description   string     `toml:"description"`
	Modes         []SeedMode `toml:"mode"`
}

// SeedMode represents a mode variant in the seed file.
type SeedMode struct {
	Mode              string `toml:"mode"`
	UpstreamMode      string `toml:"upstream_mode"`
	PoolFloorOverride string `toml:"pool_floor_override,omitempty"`
	ForceThinking     bool   `toml:"force_thinking,omitempty"`
	EnablePro         bool   `toml:"enable_pro,omitempty"`
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
// CreateFamily auto-creates the default mode; additional modes are created via CreateMode.
func importFamily(ctx context.Context, modelStore *ModelStore, sf SeedFamily) error {
	// Determine default mode's upstream_mode from the first mode (must be "default")
	var defaultUpstreamMode string
	if len(sf.Modes) > 0 {
		defaultUpstreamMode = sf.Modes[0].UpstreamMode
	}

	family := &ModelFamily{
		Model:               sf.Model,
		DisplayName:         sf.DisplayName,
		Type:                sf.Type,
		PoolFloor:           sf.PoolFloor,
		UpstreamModel:       sf.UpstreamModel,
		Enabled:             true,
		Description:         sf.Description,
		DefaultUpstreamMode: defaultUpstreamMode,
	}
	if err := modelStore.CreateFamily(ctx, family); err != nil {
		return fmt.Errorf("create family: %w", err)
	}

	// Update default mode with seed-specific fields (EnablePro, ForceThinking)
	if len(sf.Modes) > 0 && family.DefaultModeID != nil {
		firstMode := sf.Modes[0]
		if firstMode.EnablePro || firstMode.ForceThinking {
			defaultMode, err := modelStore.GetMode(ctx, *family.DefaultModeID)
			if err != nil {
				return fmt.Errorf("get default mode: %w", err)
			}
			defaultMode.EnablePro = firstMode.EnablePro
			defaultMode.ForceThinking = firstMode.ForceThinking
			if err := modelStore.UpdateMode(ctx, defaultMode); err != nil {
				return fmt.Errorf("update default mode: %w", err)
			}
		}
	}

	// Create additional modes (skip the first one — it's the auto-created default)
	for _, sm := range sf.Modes[1:] {
		mode := &ModelMode{
			ModelID:       family.ID,
			Mode:          sm.Mode,
			Enabled:       true,
			UpstreamMode:  sm.UpstreamMode,
			ForceThinking: sm.ForceThinking,
			EnablePro:     sm.EnablePro,
		}
		if sm.PoolFloorOverride != "" {
			override := sm.PoolFloorOverride
			mode.PoolFloorOverride = &override
		}
		if err := modelStore.CreateMode(ctx, mode); err != nil {
			return fmt.Errorf("create mode %s: %w", sm.Mode, err)
		}
	}

	return nil
}
