package store

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	seedconfig "github.com/crmmc/grokforge/config"
)

func TestParseSeedFile(t *testing.T) {
	seed, err := loadSeedData("", seedconfig.SeedFS)
	if err != nil {
		t.Fatalf("loadSeedData: %v", err)
	}
	if len(seed.Families) != 5 {
		t.Fatalf("expected 5 families, got %d", len(seed.Families))
	}

	// Verify mode counts per family
	expected := map[string]int{
		"grok-4.20":                5,
		"grok-imagine-image":       2,
		"grok-imagine-image-lite":  1,
		"grok-imagine-image-edit":  1,
		"grok-imagine-video":       1,
	}
	totalModes := 0
	for _, f := range seed.Families {
		want, ok := expected[f.Model]
		if !ok {
			t.Errorf("unexpected family: %s", f.Model)
			continue
		}
		if len(f.Modes) != want {
			t.Errorf("family %s: expected %d modes, got %d", f.Model, want, len(f.Modes))
		}
		totalModes += len(f.Modes)
	}
	if totalModes != 10 {
		t.Errorf("expected 10 total modes, got %d", totalModes)
	}
}

func TestSeedModels_EmptyTable(t *testing.T) {
	db := setupModelTestDB(t)
	ctx := context.Background()

	if err := SeedModels(ctx, db, "", seedconfig.SeedFS); err != nil {
		t.Fatalf("SeedModels: %v", err)
	}

	// Verify 5 families
	var families []ModelFamily
	if err := db.Find(&families).Error; err != nil {
		t.Fatalf("query families: %v", err)
	}
	if len(families) != 5 {
		t.Fatalf("expected 5 families, got %d", len(families))
	}

	// Verify mode counts
	expectedModes := map[string]int{
		"grok-4.20": 5,
		"grok-imagine-image": 2, "grok-imagine-image-lite": 1,
		"grok-imagine-image-edit": 1,
		"grok-imagine-video": 1,
	}
	for _, f := range families {
		var modes []ModelMode
		db.Where("model_id = ?", f.ID).Find(&modes)
		want := expectedModes[f.Model]
		if len(modes) != want {
			t.Errorf("family %s: expected %d modes, got %d", f.Model, want, len(modes))
		}
	}

	// Verify DefaultModeID is set for all families
	for _, f := range families {
		if f.DefaultModeID == nil {
			t.Errorf("family %s: DefaultModeID is nil", f.Model)
		}
	}
}

func TestSeedModels_NonEmpty(t *testing.T) {
	db := setupModelTestDB(t)
	ctx := context.Background()

	// Pre-populate with one family
	f := &ModelFamily{Model: "existing", Type: "chat", PoolFloor: "basic", Enabled: true}
	if err := db.Create(f).Error; err != nil {
		t.Fatalf("create existing: %v", err)
	}

	if err := SeedModels(ctx, db, "", seedconfig.SeedFS); err != nil {
		t.Fatalf("SeedModels: %v", err)
	}

	// Should still be 1 family (seed skipped)
	var count int64
	db.Model(&ModelFamily{}).Count(&count)
	if count != 1 {
		t.Errorf("expected 1 family (seed skipped), got %d", count)
	}
}

func TestSeedModels_ExternalFile(t *testing.T) {
	db := setupModelTestDB(t)
	ctx := context.Background()

	// Create temp dir with a custom seed file
	tmpDir := t.TempDir()
	seedContent := `
[[family]]
model          = "custom-model"
display_name   = "Custom Model"
type           = "chat"
pool_floor     = "basic"
upstream_model = "custom"

  [[family.mode]]
  mode           = "default"
  upstream_mode  = "auto"
`
	if err := os.WriteFile(filepath.Join(tmpDir, "models.seed.toml"), []byte(seedContent), 0644); err != nil {
		t.Fatalf("write temp seed: %v", err)
	}

	if err := SeedModels(ctx, db, tmpDir, seedconfig.SeedFS); err != nil {
		t.Fatalf("SeedModels: %v", err)
	}

	// Should have 1 family from external file, not 5 from embed
	var count int64
	db.Model(&ModelFamily{}).Count(&count)
	if count != 1 {
		t.Errorf("expected 1 family from external file, got %d", count)
	}

	var f ModelFamily
	db.First(&f)
	if f.Model != "custom-model" {
		t.Errorf("expected custom-model, got %s", f.Model)
	}
}

func TestSeedModels_EmbedFallback(t *testing.T) {
	db := setupModelTestDB(t)
	ctx := context.Background()

	// Use empty temp dir (no external seed file) -> should fallback to embed
	tmpDir := t.TempDir()

	if err := SeedModels(ctx, db, tmpDir, seedconfig.SeedFS); err != nil {
		t.Fatalf("SeedModels: %v", err)
	}

	var count int64
	db.Model(&ModelFamily{}).Count(&count)
	if count != 5 {
		t.Errorf("expected 5 families from embed fallback, got %d", count)
	}
}

func TestSeedModels_DefaultModeID(t *testing.T) {
	db := setupModelTestDB(t)
	ctx := context.Background()

	if err := SeedModels(ctx, db, "", seedconfig.SeedFS); err != nil {
		t.Fatalf("SeedModels: %v", err)
	}

	var families []ModelFamily
	db.Find(&families)

	for _, f := range families {
		if f.DefaultModeID == nil {
			t.Errorf("family %s: DefaultModeID is nil", f.Model)
			continue
		}
		// Verify the default mode has mode="default"
		var mode ModelMode
		if err := db.First(&mode, *f.DefaultModeID).Error; err != nil {
			t.Errorf("family %s: failed to load default mode: %v", f.Model, err)
			continue
		}
		if mode.Mode != "default" {
			t.Errorf("family %s: default mode is %q, expected 'default'", f.Model, mode.Mode)
		}
	}
}

func TestSeedModels_PoolFloorOverride(t *testing.T) {
	db := setupModelTestDB(t)
	ctx := context.Background()

	if err := SeedModels(ctx, db, "", seedconfig.SeedFS); err != nil {
		t.Fatalf("SeedModels: %v", err)
	}

	// Find grok-4.20 family
	var family ModelFamily
	if err := db.Where("model = ?", "grok-4.20").First(&family).Error; err != nil {
		t.Fatalf("find grok-4.20: %v", err)
	}

	var modes []ModelMode
	db.Where("model_id = ?", family.ID).Find(&modes)

	overrides := map[string]string{
		"expert": "super",
		"heavy":  "heavy",
	}
	for _, m := range modes {
		if expected, ok := overrides[m.Mode]; ok {
			if m.PoolFloorOverride == nil {
				t.Errorf("mode %s: expected pool_floor_override=%q, got nil", m.Mode, expected)
			} else if *m.PoolFloorOverride != expected {
				t.Errorf("mode %s: expected pool_floor_override=%q, got %q", m.Mode, expected, *m.PoolFloorOverride)
			}
		} else {
			if m.PoolFloorOverride != nil {
				t.Errorf("mode %s: expected nil pool_floor_override, got %q", m.Mode, *m.PoolFloorOverride)
			}
		}
	}
}

