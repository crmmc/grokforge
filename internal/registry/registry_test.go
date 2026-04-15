package registry

import (
	"context"
	"testing"

	"github.com/crmmc/grokforge/internal/store"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func setupTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatalf("failed to open test database: %v", err)
	}
	if err := store.AutoMigrate(db); err != nil {
		t.Fatalf("failed to migrate test database: %v", err)
	}
	return db
}

// seedTestData creates two families with modes for testing.
// grok-4 (chat, basic): default mode "default" + "heavy" mode (pool_floor_override=heavy)
// flux-1 (image, basic): mode "standard" (default)
// Returns (grok4Family, flux1Family).
func seedTestData(t *testing.T, db *gorm.DB) (*store.ModelFamily, *store.ModelFamily) {
	t.Helper()

	// Family: grok-4 (chat)
	grok4 := &store.ModelFamily{
		Model:       "grok-4",
		DisplayName: "Grok 4",
		Type:        "chat",
		Enabled:     true,
		PoolFloor:   "basic",
	}
	if err := db.Create(grok4).Error; err != nil {
		t.Fatalf("create grok-4 family: %v", err)
	}

	// Modes for grok-4
	defaultMode := &store.ModelMode{
		ModelID:       grok4.ID,
		Mode:          "default",
		Enabled:       true,
		UpstreamModel: "grok-3",
		UpstreamMode:  "",
	}
	if err := db.Create(defaultMode).Error; err != nil {
		t.Fatalf("create default mode: %v", err)
	}

	heavyFloor := "heavy"
	heavyMode := &store.ModelMode{
		ModelID:           grok4.ID,
		Mode:              "heavy",
		Enabled:           true,
		PoolFloorOverride: &heavyFloor,
		UpstreamModel:     "grok-3",
		UpstreamMode:      "heavy",
	}
	if err := db.Create(heavyMode).Error; err != nil {
		t.Fatalf("create heavy mode: %v", err)
	}

	// Set default mode
	grok4.DefaultModeID = &defaultMode.ID
	if err := db.Save(grok4).Error; err != nil {
		t.Fatalf("set default mode: %v", err)
	}

	// Family: flux-1 (image)
	flux1 := &store.ModelFamily{
		Model:       "flux-1",
		DisplayName: "Flux 1",
		Type:        "image",
		Enabled:     true,
		PoolFloor:   "basic",
	}
	if err := db.Create(flux1).Error; err != nil {
		t.Fatalf("create flux-1 family: %v", err)
	}

	standardMode := &store.ModelMode{
		ModelID:       flux1.ID,
		Mode:          "standard",
		Enabled:       true,
		UpstreamModel: "flux-1",
		UpstreamMode:  "standard",
	}
	if err := db.Create(standardMode).Error; err != nil {
		t.Fatalf("create standard mode: %v", err)
	}

	flux1.DefaultModeID = &standardMode.ID
	if err := db.Save(flux1).Error; err != nil {
		t.Fatalf("set flux-1 default mode: %v", err)
	}

	return grok4, flux1
}

func TestRegistry_Refresh(t *testing.T) {
	db := setupTestDB(t)
	seedTestData(t, db)

	ms := store.NewModelStore(db)
	reg := NewModelRegistry(ms)

	if err := reg.Refresh(context.Background()); err != nil {
		t.Fatalf("Refresh failed: %v", err)
	}

	// grok-4 has 2 enabled modes, flux-1 has 1 => 3 total
	if got := reg.Count(); got != 3 {
		t.Errorf("Count() = %d, want 3", got)
	}
}

func TestRegistry_Resolve(t *testing.T) {
	db := setupTestDB(t)
	seedTestData(t, db)

	ms := store.NewModelStore(db)
	reg := NewModelRegistry(ms)
	if err := reg.Refresh(context.Background()); err != nil {
		t.Fatalf("Refresh failed: %v", err)
	}

	// Default mode: "grok-4" resolves to default mode
	rm, ok := reg.Resolve("grok-4")
	if !ok {
		t.Fatal("Resolve('grok-4') returned false")
	}
	if rm.RequestName != "grok-4" {
		t.Errorf("RequestName = %q, want 'grok-4'", rm.RequestName)
	}
	if rm.UpstreamModel != "grok-3" {
		t.Errorf("UpstreamModel = %q, want 'grok-3'", rm.UpstreamModel)
	}

	// Non-default mode: "grok-4-heavy"
	rm2, ok := reg.Resolve("grok-4-heavy")
	if !ok {
		t.Fatal("Resolve('grok-4-heavy') returned false")
	}
	if rm2.RequestName != "grok-4-heavy" {
		t.Errorf("RequestName = %q, want 'grok-4-heavy'", rm2.RequestName)
	}
	if rm2.UpstreamMode != "heavy" {
		t.Errorf("UpstreamMode = %q, want 'heavy'", rm2.UpstreamMode)
	}
}

func TestRegistry_ResolveNotFound(t *testing.T) {
	db := setupTestDB(t)
	seedTestData(t, db)

	ms := store.NewModelStore(db)
	reg := NewModelRegistry(ms)
	if err := reg.Refresh(context.Background()); err != nil {
		t.Fatalf("Refresh failed: %v", err)
	}

	_, ok := reg.Resolve("nonexistent")
	if ok {
		t.Error("Resolve('nonexistent') should return false")
	}
}

func TestRegistry_EnabledByType(t *testing.T) {
	db := setupTestDB(t)
	seedTestData(t, db)

	ms := store.NewModelStore(db)
	reg := NewModelRegistry(ms)
	if err := reg.Refresh(context.Background()); err != nil {
		t.Fatalf("Refresh failed: %v", err)
	}

	chatModels := reg.EnabledByType("chat")
	if len(chatModels) != 2 {
		t.Errorf("EnabledByType('chat') returned %d models, want 2", len(chatModels))
	}

	imageModels := reg.EnabledByType("image")
	if len(imageModels) != 1 {
		t.Errorf("EnabledByType('image') returned %d models, want 1", len(imageModels))
	}
}

func TestRegistry_EnabledByType_Empty(t *testing.T) {
	db := setupTestDB(t)
	seedTestData(t, db)

	ms := store.NewModelStore(db)
	reg := NewModelRegistry(ms)
	if err := reg.Refresh(context.Background()); err != nil {
		t.Fatalf("Refresh failed: %v", err)
	}

	result := reg.EnabledByType("nonexistent")
	if result == nil {
		t.Error("EnabledByType should return non-nil empty slice, got nil")
	}
	if len(result) != 0 {
		t.Errorf("EnabledByType('nonexistent') returned %d models, want 0", len(result))
	}
}

func TestRegistry_DisabledExcluded(t *testing.T) {
	db := setupTestDB(t)

	// Create family first (GORM default:true treats false as zero value on create)
	disabledFamily := &store.ModelFamily{
		Model:     "disabled-model",
		Type:      "chat",
		Enabled:   true,
		PoolFloor: "basic",
	}
	if err := db.Create(disabledFamily).Error; err != nil {
		t.Fatalf("create family: %v", err)
	}
	disabledMode := &store.ModelMode{
		ModelID:       disabledFamily.ID,
		Mode:          "default",
		Enabled:       true,
		UpstreamModel: "disabled-model",
	}
	if err := db.Create(disabledMode).Error; err != nil {
		t.Fatalf("create mode: %v", err)
	}
	// Disable family after creation
	if err := db.Model(disabledFamily).Update("enabled", false).Error; err != nil {
		t.Fatalf("disable family: %v", err)
	}

	ms := store.NewModelStore(db)
	reg := NewModelRegistry(ms)
	if err := reg.Refresh(context.Background()); err != nil {
		t.Fatalf("Refresh failed: %v", err)
	}

	// Disabled family's modes should not appear
	_, ok := reg.Resolve("disabled-model-default")
	if ok {
		t.Error("disabled family's modes should not be in registry")
	}
	if reg.Count() != 0 {
		t.Errorf("Count() = %d, want 0 (only disabled family)", reg.Count())
	}
}

func TestRegistry_DisabledModeExcluded(t *testing.T) {
	db := setupTestDB(t)

	family := &store.ModelFamily{
		Model:     "grok-5",
		Type:      "chat",
		Enabled:   true,
		PoolFloor: "basic",
	}
	if err := db.Create(family).Error; err != nil {
		t.Fatalf("create family: %v", err)
	}

	enabledMode := &store.ModelMode{
		ModelID:       family.ID,
		Mode:          "fast",
		Enabled:       true,
		UpstreamModel: "grok-5",
		UpstreamMode:  "fast",
	}
	if err := db.Create(enabledMode).Error; err != nil {
		t.Fatalf("create enabled mode: %v", err)
	}

	disabledMode := &store.ModelMode{
		ModelID:       family.ID,
		Mode:          "slow",
		Enabled:       true,
		UpstreamModel: "grok-5",
		UpstreamMode:  "slow",
	}
	if err := db.Create(disabledMode).Error; err != nil {
		t.Fatalf("create disabled mode: %v", err)
	}
	// Disable mode after creation (GORM default:true treats false as zero value)
	if err := db.Model(disabledMode).Update("enabled", false).Error; err != nil {
		t.Fatalf("disable mode: %v", err)
	}

	ms := store.NewModelStore(db)
	reg := NewModelRegistry(ms)
	if err := reg.Refresh(context.Background()); err != nil {
		t.Fatalf("Refresh failed: %v", err)
	}

	// Only enabled mode should be in registry
	if reg.Count() != 1 {
		t.Errorf("Count() = %d, want 1 (disabled mode excluded)", reg.Count())
	}
	_, ok := reg.Resolve("grok-5-fast")
	if !ok {
		t.Error("enabled mode 'grok-5-fast' should be resolvable")
	}
	_, ok = reg.Resolve("grok-5-slow")
	if ok {
		t.Error("disabled mode 'grok-5-slow' should not be resolvable")
	}
}

func TestRegistry_EffectiveFloor(t *testing.T) {
	db := setupTestDB(t)
	seedTestData(t, db)

	ms := store.NewModelStore(db)
	reg := NewModelRegistry(ms)
	if err := reg.Refresh(context.Background()); err != nil {
		t.Fatalf("Refresh failed: %v", err)
	}

	// Default mode inherits family pool_floor ("basic")
	rm, ok := reg.Resolve("grok-4")
	if !ok {
		t.Fatal("Resolve('grok-4') returned false")
	}
	if rm.EffectiveFloor != "basic" {
		t.Errorf("EffectiveFloor = %q, want 'basic' (inherited from family)", rm.EffectiveFloor)
	}

	// Heavy mode has pool_floor_override = "heavy"
	rm2, ok := reg.Resolve("grok-4-heavy")
	if !ok {
		t.Fatal("Resolve('grok-4-heavy') returned false")
	}
	if rm2.EffectiveFloor != "heavy" {
		t.Errorf("EffectiveFloor = %q, want 'heavy' (mode override)", rm2.EffectiveFloor)
	}
}

func TestRegistry_RefreshUpdate(t *testing.T) {
	db := setupTestDB(t)
	seedTestData(t, db)

	ms := store.NewModelStore(db)
	reg := NewModelRegistry(ms)
	if err := reg.Refresh(context.Background()); err != nil {
		t.Fatalf("Refresh failed: %v", err)
	}

	if reg.Count() != 3 {
		t.Fatalf("initial Count() = %d, want 3", reg.Count())
	}

	// Add a new mode to grok-4
	newMode := &store.ModelMode{
		ModelID:       1, // grok-4's ID
		Mode:          "turbo",
		Enabled:       true,
		UpstreamModel: "grok-3",
		UpstreamMode:  "turbo",
	}
	if err := db.Create(newMode).Error; err != nil {
		t.Fatalf("create turbo mode: %v", err)
	}

	// Refresh and verify updated count
	if err := reg.Refresh(context.Background()); err != nil {
		t.Fatalf("Refresh after update failed: %v", err)
	}
	if reg.Count() != 4 {
		t.Errorf("Count() after Refresh = %d, want 4", reg.Count())
	}
	_, ok := reg.Resolve("grok-4-turbo")
	if !ok {
		t.Error("Resolve('grok-4-turbo') should succeed after Refresh")
	}
}

func TestRegistry_AllEnabled(t *testing.T) {
	db := setupTestDB(t)
	seedTestData(t, db)

	ms := store.NewModelStore(db)
	reg := NewModelRegistry(ms)
	if err := reg.Refresh(context.Background()); err != nil {
		t.Fatalf("Refresh failed: %v", err)
	}

	all := reg.AllEnabled()
	if len(all) != 3 {
		t.Fatalf("AllEnabled() returned %d models, want 3", len(all))
	}

	// Verify returned slice is a copy (modifying it doesn't affect registry)
	all[0] = nil
	all2 := reg.AllEnabled()
	for _, rm := range all2 {
		if rm == nil {
			t.Error("modifying AllEnabled() result should not affect registry")
		}
	}
}
