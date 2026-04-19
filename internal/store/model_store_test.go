package store

import (
	"context"
	"errors"
	"testing"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func setupModelTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatalf("failed to open test database: %v", err)
	}
	if err := AutoMigrate(db); err != nil {
		t.Fatalf("failed to migrate test database: %v", err)
	}
	return db
}

func newTestFamily(model, typ string) *ModelFamily {
	return &ModelFamily{
		Model:               model,
		Type:                typ,
		PoolFloor:           "basic",
		Enabled:             true,
		UpstreamModel:       "test-upstream",
		DefaultUpstreamMode: "auto",
	}
}

func newTestImageFamily(model string) *ModelFamily {
	return &ModelFamily{
		Model:     model,
		Type:      "image_ws",
		PoolFloor: "super",
		Enabled:   true,
	}
}

func newTestMode(modelID uint, mode string) *ModelMode {
	return &ModelMode{
		ModelID:      modelID,
		Mode:         mode,
		Enabled:      true,
		UpstreamMode: "MODE_" + mode,
	}
}

func TestModelFamily_CRUD(t *testing.T) {
	db := setupModelTestDB(t)
	s := NewModelStore(db)
	ctx := context.Background()

	// Create
	f := newTestFamily("grok-3", "chat")
	f.DisplayName = "Grok 3"
	if err := s.CreateFamily(ctx, f); err != nil {
		t.Fatalf("create family: %v", err)
	}
	if f.ID == 0 {
		t.Fatal("expected non-zero ID after create")
	}
	// Verify default mode was auto-created
	if f.DefaultModeID == nil {
		t.Fatal("expected DefaultModeID to be set after create")
	}
	modes, err := s.ListModesByFamily(ctx, f.ID)
	if err != nil {
		t.Fatalf("list modes: %v", err)
	}
	if len(modes) != 1 {
		t.Fatalf("expected 1 auto-created mode, got %d", len(modes))
	}
	if modes[0].Mode != "default" {
		t.Fatalf("expected auto-created mode to be 'default', got %q", modes[0].Mode)
	}
	if modes[0].UpstreamMode != "auto" {
		t.Fatalf("expected default mode upstream_mode 'auto', got %q", modes[0].UpstreamMode)
	}

	// Get
	got, err := s.GetFamily(ctx, f.ID)
	if err != nil {
		t.Fatalf("get family: %v", err)
	}
	if got.Model != "grok-3" || got.DisplayName != "Grok 3" {
		t.Errorf("unexpected family: %+v", got)
	}

	// Update
	got.DisplayName = "Grok 3 Updated"
	if err := s.UpdateFamily(ctx, got); err != nil {
		t.Fatalf("update family: %v", err)
	}
	got2, _ := s.GetFamily(ctx, f.ID)
	if got2.DisplayName != "Grok 3 Updated" {
		t.Errorf("expected updated display name, got %q", got2.DisplayName)
	}

	// List
	families, err := s.ListFamilies(ctx)
	if err != nil {
		t.Fatalf("list families: %v", err)
	}
	if len(families) != 1 {
		t.Errorf("expected 1 family, got %d", len(families))
	}

	// Delete
	if err := s.DeleteFamily(ctx, f.ID); err != nil {
		t.Fatalf("delete family: %v", err)
	}
	_, err = s.GetFamily(ctx, f.ID)
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("expected ErrNotFound after delete, got %v", err)
	}
}

func TestModelFamily_UniqueModel(t *testing.T) {
	db := setupModelTestDB(t)
	s := NewModelStore(db)
	ctx := context.Background()

	f1 := newTestFamily("grok-3", "chat")
	if err := s.CreateFamily(ctx, f1); err != nil {
		t.Fatalf("create first: %v", err)
	}
	f2 := newTestFamily("grok-3", "chat")
	err := s.CreateFamily(ctx, f2)
	if err == nil {
		t.Fatal("expected error for duplicate model name")
	}
}

func TestModelMode_CRUD(t *testing.T) {
	db := setupModelTestDB(t)
	s := NewModelStore(db)
	ctx := context.Background()

	f := newTestFamily("grok-3", "chat")
	if err := s.CreateFamily(ctx, f); err != nil {
		t.Fatalf("create family: %v", err)
	}

	// Create non-default mode
	m := &ModelMode{ModelID: f.ID, Mode: "mini", Enabled: true, UpstreamMode: "mini"}
	if err := s.CreateMode(ctx, m); err != nil {
		t.Fatalf("create mode: %v", err)
	}
	if m.ID == 0 {
		t.Fatal("expected non-zero mode ID")
	}

	// Get mode
	got, err := s.GetMode(ctx, m.ID)
	if err != nil {
		t.Fatalf("get mode: %v", err)
	}
	if got.Mode != "mini" {
		t.Errorf("expected mode 'mini', got %q", got.Mode)
	}

	// List by family (1 auto default + 1 manual)
	modes, err := s.ListModesByFamily(ctx, f.ID)
	if err != nil {
		t.Fatalf("list modes: %v", err)
	}
	if len(modes) != 2 {
		t.Errorf("expected 2 modes, got %d", len(modes))
	}

	// Update mode — change UpstreamMode
	got.UpstreamMode = "mini-updated"
	if err := s.UpdateMode(ctx, got); err != nil {
		t.Fatalf("update mode: %v", err)
	}
	got2, _ := s.GetMode(ctx, m.ID)
	if got2.UpstreamMode != "mini-updated" {
		t.Errorf("expected updated upstream_mode, got %q", got2.UpstreamMode)
	}

	// Delete mode
	if err := s.DeleteMode(ctx, m.ID); err != nil {
		t.Fatalf("delete mode: %v", err)
	}
	_, err = s.GetMode(ctx, m.ID)
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("expected ErrNotFound after delete, got %v", err)
	}
}

func TestModelMode_UniqueMode(t *testing.T) {
	db := setupModelTestDB(t)
	s := NewModelStore(db)
	ctx := context.Background()

	f := newTestFamily("grok-3", "chat")
	if err := s.CreateFamily(ctx, f); err != nil {
		t.Fatalf("create family: %v", err)
	}
	m1 := newTestMode(f.ID, "mini")
	if err := s.CreateMode(ctx, m1); err != nil {
		t.Fatalf("create first mode: %v", err)
	}
	m2 := newTestMode(f.ID, "mini")
	err := s.CreateMode(ctx, m2)
	if err == nil {
		t.Fatal("expected error for duplicate mode in same family")
	}
}

func TestModelMode_CrossFamily(t *testing.T) {
	db := setupModelTestDB(t)
	s := NewModelStore(db)
	ctx := context.Background()

	f1 := newTestFamily("grok-3", "chat")
	f2 := newTestFamily("grok-4", "chat")
	if err := s.CreateFamily(ctx, f1); err != nil {
		t.Fatalf("create f1: %v", err)
	}
	if err := s.CreateFamily(ctx, f2); err != nil {
		t.Fatalf("create f2: %v", err)
	}
	// Same mode name in different families should be allowed
	m1 := newTestMode(f1.ID, "fast")
	m2 := newTestMode(f2.ID, "fast")
	if err := s.CreateMode(ctx, m1); err != nil {
		t.Fatalf("create mode in f1: %v", err)
	}
	if err := s.CreateMode(ctx, m2); err != nil {
		t.Fatalf("create mode in f2: %v", err)
	}
}

func TestConflict_CreateFamily(t *testing.T) {
	db := setupModelTestDB(t)
	s := NewModelStore(db)
	ctx := context.Background()

	// Create family "grok-3" (auto-creates default mode), then add "mini" mode
	f := newTestFamily("grok-3", "chat")
	if err := s.CreateFamily(ctx, f); err != nil {
		t.Fatalf("create family: %v", err)
	}
	miniMode := newTestMode(f.ID, "mini")
	if err := s.CreateMode(ctx, miniMode); err != nil {
		t.Fatalf("create mode: %v", err)
	}

	// Creating family "grok-3-mini" should conflict with the non-default request name
	f2 := newTestFamily("grok-3-mini", "chat")
	err := s.CreateFamily(ctx, f2)
	if !errors.Is(err, ErrConflict) {
		t.Errorf("expected ErrConflict, got %v", err)
	}
}

func TestConflict_CreateMode(t *testing.T) {
	db := setupModelTestDB(t)
	s := NewModelStore(db)
	ctx := context.Background()

	// Create family "grok-3-mini"
	f1 := newTestFamily("grok-3-mini", "chat")
	if err := s.CreateFamily(ctx, f1); err != nil {
		t.Fatalf("create family: %v", err)
	}

	// Create family "grok-3"
	f2 := newTestFamily("grok-3", "chat")
	if err := s.CreateFamily(ctx, f2); err != nil {
		t.Fatalf("create family: %v", err)
	}

	// Creating a non-default mode "mini" under "grok-3" -> derived "grok-3-mini" conflicts with family
	m := newTestMode(f2.ID, "mini")
	err := s.CreateMode(ctx, m)
	if !errors.Is(err, ErrConflict) {
		t.Errorf("expected ErrConflict, got %v", err)
	}
}

func TestConflict_UpdateFamily(t *testing.T) {
	db := setupModelTestDB(t)
	s := NewModelStore(db)
	ctx := context.Background()

	f1 := newTestFamily("grok-3", "chat")
	f2 := newTestFamily("grok-4", "chat")
	if err := s.CreateFamily(ctx, f1); err != nil {
		t.Fatalf("create f1: %v", err)
	}
	if err := s.CreateFamily(ctx, f2); err != nil {
		t.Fatalf("create f2: %v", err)
	}

	// Rename f2 to "grok-3" should conflict
	f2.Model = "grok-3"
	f2.UpstreamModel = "test-upstream"
	err := s.UpdateFamily(ctx, f2)
	if !errors.Is(err, ErrConflict) {
		t.Errorf("expected ErrConflict, got %v", err)
	}
}

func TestConflict_UpdateMode(t *testing.T) {
	db := setupModelTestDB(t)
	s := NewModelStore(db)
	ctx := context.Background()

	// family "grok-3-fast" exists
	f1 := newTestFamily("grok-3-fast", "chat")
	if err := s.CreateFamily(ctx, f1); err != nil {
		t.Fatalf("create f1: %v", err)
	}

	// family "grok-3" with mode "mini"
	f2 := newTestFamily("grok-3", "chat")
	if err := s.CreateFamily(ctx, f2); err != nil {
		t.Fatalf("create f2: %v", err)
	}
	m := newTestMode(f2.ID, "mini")
	if err := s.CreateMode(ctx, m); err != nil {
		t.Fatalf("create mode: %v", err)
	}

	// Update non-default mode name to "fast" -> derived "grok-3-fast" conflicts
	m.Mode = "fast"
	err := s.UpdateMode(ctx, m)
	if !errors.Is(err, ErrConflict) {
		t.Errorf("expected ErrConflict, got %v", err)
	}
}

func TestConflict_D08Example(t *testing.T) {
	db := setupModelTestDB(t)
	s := NewModelStore(db)
	ctx := context.Background()

	// Scenario A: grok-4 has auto default mode plus non-default "mini", then create grok-4-mini family
	f1 := newTestFamily("grok-4", "chat")
	if err := s.CreateFamily(ctx, f1); err != nil {
		t.Fatalf("create grok-4: %v", err)
	}
	m := newTestMode(f1.ID, "mini")
	if err := s.CreateMode(ctx, m); err != nil {
		t.Fatalf("create mini mode: %v", err)
	}
	f2 := newTestFamily("grok-4-mini", "chat")
	err := s.CreateFamily(ctx, f2)
	if !errors.Is(err, ErrConflict) {
		t.Errorf("scenario A: expected ErrConflict, got %v", err)
	}

	// Scenario B: fresh DB, grok-4-mini family exists, then add non-default mini mode to grok-4
	db2 := setupModelTestDB(t)
	s2 := NewModelStore(db2)

	fa := newTestFamily("grok-4-mini", "chat")
	if err := s2.CreateFamily(ctx, fa); err != nil {
		t.Fatalf("create grok-4-mini: %v", err)
	}
	fb := newTestFamily("grok-4", "chat")
	if err := s2.CreateFamily(ctx, fb); err != nil {
		t.Fatalf("create grok-4: %v", err)
	}
	m2 := newTestMode(fb.ID, "mini")
	err = s2.CreateMode(ctx, m2)
	if !errors.Is(err, ErrConflict) {
		t.Errorf("scenario B: expected ErrConflict, got %v", err)
	}
}

func TestConflict_NoFalsePositive(t *testing.T) {
	db := setupModelTestDB(t)
	s := NewModelStore(db)
	ctx := context.Background()

	f := newTestFamily("grok-3", "chat")
	if err := s.CreateFamily(ctx, f); err != nil {
		t.Fatalf("create family: %v", err)
	}

	// Updating family without changing model name should not conflict
	f.DisplayName = "Updated Name"
	if err := s.UpdateFamily(ctx, f); err != nil {
		t.Errorf("update family should not conflict: %v", err)
	}

	// Get the auto-created default mode
	modes, _ := s.ListModesByFamily(ctx, f.ID)
	var defaultMode *ModelMode
	for _, m := range modes {
		if m.Mode == "default" {
			defaultMode = m
			break
		}
	}
	if defaultMode == nil {
		t.Fatal("expected auto-created default mode")
	}

	// Updating mode's UpstreamMode without changing mode name should not conflict
	defaultMode.UpstreamMode = "updated-upstream-mode"
	if err := s.UpdateMode(ctx, defaultMode); err != nil {
		t.Errorf("update mode should not conflict: %v", err)
	}
}

func TestDeleteFamily_CascadeModes(t *testing.T) {
	db := setupModelTestDB(t)
	s := NewModelStore(db)
	ctx := context.Background()

	f := newTestFamily("grok-3", "chat")
	if err := s.CreateFamily(ctx, f); err != nil {
		t.Fatalf("create family: %v", err)
	}
	// Add 2 more modes (1 auto default + 2 manual = 3 total)
	for _, mode := range []string{"fast", "turbo"} {
		m := newTestMode(f.ID, mode)
		if err := s.CreateMode(ctx, m); err != nil {
			t.Fatalf("create mode %s: %v", mode, err)
		}
	}

	modes, _ := s.ListModesByFamily(ctx, f.ID)
	if len(modes) != 3 {
		t.Fatalf("expected 3 modes before delete, got %d", len(modes))
	}

	if err := s.DeleteFamily(ctx, f.ID); err != nil {
		t.Fatalf("delete family: %v", err)
	}

	modes, _ = s.ListModesByFamily(ctx, f.ID)
	if len(modes) != 0 {
		t.Errorf("expected 0 modes after cascade delete, got %d", len(modes))
	}
}

func TestCreateFamily_InvalidType(t *testing.T) {
	db := setupModelTestDB(t)
	s := NewModelStore(db)
	ctx := context.Background()

	f := newTestFamily("grok-3", "invalid_type")
	err := s.CreateFamily(ctx, f)
	if err == nil {
		t.Fatal("expected error for invalid type")
	}
}

func TestCreateFamily_InvalidPoolFloor(t *testing.T) {
	db := setupModelTestDB(t)
	s := NewModelStore(db)
	ctx := context.Background()

	f := &ModelFamily{
		Model:               "grok-3",
		Type:                "chat",
		PoolFloor:           "invalid_pool",
		Enabled:             true,
		UpstreamModel:       "test-upstream",
		DefaultUpstreamMode: "auto",
	}
	err := s.CreateFamily(ctx, f)
	if err == nil {
		t.Fatal("expected error for invalid pool_floor")
	}
}

func TestCreateFamily_NormalizesTrimmedIdentifiers(t *testing.T) {
	db := setupModelTestDB(t)
	s := NewModelStore(db)
	ctx := context.Background()

	f := &ModelFamily{
		Model:               "  grok-3  ",
		Type:                "  chat  ",
		PoolFloor:           "  basic  ",
		Enabled:             true,
		UpstreamModel:       "  test-upstream  ",
		DefaultUpstreamMode: "  auto  ",
	}
	if err := s.CreateFamily(ctx, f); err != nil {
		t.Fatalf("create family: %v", err)
	}

	got, err := s.GetFamily(ctx, f.ID)
	if err != nil {
		t.Fatalf("get family: %v", err)
	}
	if got.Model != "grok-3" {
		t.Fatalf("expected trimmed model, got %q", got.Model)
	}
	if got.Type != "chat" {
		t.Fatalf("expected trimmed type, got %q", got.Type)
	}
	if got.PoolFloor != "basic" {
		t.Fatalf("expected trimmed pool floor, got %q", got.PoolFloor)
	}
}

func TestCreateFamily_PersistsDisabledState(t *testing.T) {
	db := setupModelTestDB(t)
	s := NewModelStore(db)
	ctx := context.Background()

	f := &ModelFamily{
		Model:               "grok-disabled",
		Type:                "chat",
		PoolFloor:           "basic",
		Enabled:             false,
		UpstreamModel:       "test-upstream",
		DefaultUpstreamMode: "auto",
	}
	if err := s.CreateFamily(ctx, f); err != nil {
		t.Fatalf("create family: %v", err)
	}

	got, err := s.GetFamily(ctx, f.ID)
	if err != nil {
		t.Fatalf("get family: %v", err)
	}
	if got.Enabled {
		t.Fatal("expected family to stay disabled after create")
	}
}

func TestCreateMode_PersistsDisabledState(t *testing.T) {
	db := setupModelTestDB(t)
	s := NewModelStore(db)
	ctx := context.Background()

	f := newTestFamily("grok-3", "chat")
	if err := s.CreateFamily(ctx, f); err != nil {
		t.Fatalf("create family: %v", err)
	}

	mode := &ModelMode{
		ModelID:      f.ID,
		Mode:         "expert",
		Enabled:      false,
		UpstreamMode: "MODE_EXPERT",
	}
	if err := s.CreateMode(ctx, mode); err != nil {
		t.Fatalf("create disabled mode: %v", err)
	}

	got, err := s.GetMode(ctx, mode.ID)
	if err != nil {
		t.Fatalf("get mode: %v", err)
	}
	if got.Enabled {
		t.Fatal("expected mode to stay disabled after create")
	}
}

func TestCreateMode_ImageFamilyAllowsEmptyUpstream(t *testing.T) {
	db := setupModelTestDB(t)
	s := NewModelStore(db)
	ctx := context.Background()

	f := newTestImageFamily("grok-imagine-image")
	if err := s.CreateFamily(ctx, f); err != nil {
		t.Fatalf("create family: %v", err)
	}

	// Verify auto-created default mode has empty upstream
	modes, _ := s.ListModesByFamily(ctx, f.ID)
	if len(modes) != 1 {
		t.Fatalf("expected 1 auto-created mode, got %d", len(modes))
	}
	if modes[0].UpstreamMode != "" {
		t.Fatalf("expected empty upstream_mode for image default mode, got %q", modes[0].UpstreamMode)
	}
}

func TestCreateMode_ImageFamilyRejectsUpstreamMapping(t *testing.T) {
	db := setupModelTestDB(t)
	s := NewModelStore(db)
	ctx := context.Background()

	f := newTestImageFamily("grok-imagine-image")
	if err := s.CreateFamily(ctx, f); err != nil {
		t.Fatalf("create family: %v", err)
	}

	mode := &ModelMode{
		ModelID:      f.ID,
		Mode:         "pro",
		Enabled:      true,
		UpstreamMode: "MODEL_MODE_FAST",
	}
	err := s.CreateMode(ctx, mode)
	if !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("expected ErrInvalidInput, got %v", err)
	}
}

func TestCreateMode_NormalizesTrimmedIdentifiers(t *testing.T) {
	db := setupModelTestDB(t)
	s := NewModelStore(db)
	ctx := context.Background()

	f := newTestFamily("grok-3", "chat")
	if err := s.CreateFamily(ctx, f); err != nil {
		t.Fatalf("create family: %v", err)
	}
	override := "  super  "
	mode := &ModelMode{
		ModelID:           f.ID,
		Mode:              "  fast  ",
		Enabled:           true,
		PoolFloorOverride: &override,
		UpstreamMode:      "  MODE_FAST  ",
	}
	if err := s.CreateMode(ctx, mode); err != nil {
		t.Fatalf("create mode: %v", err)
	}

	got, err := s.GetMode(ctx, mode.ID)
	if err != nil {
		t.Fatalf("get mode: %v", err)
	}
	if got.Mode != "fast" {
		t.Fatalf("expected trimmed mode, got %q", got.Mode)
	}
	if got.UpstreamMode != "MODE_FAST" {
		t.Fatalf("expected trimmed upstream_mode, got %q", got.UpstreamMode)
	}
	if got.PoolFloorOverride == nil || *got.PoolFloorOverride != "super" {
		t.Fatalf("expected trimmed pool floor override, got %v", got.PoolFloorOverride)
	}
}

func TestUpdateFamily_TypeChangeRequiresExistingModeUpstream(t *testing.T) {
	db := setupModelTestDB(t)
	s := NewModelStore(db)
	ctx := context.Background()

	family := newTestImageFamily("grok-imagine-image")
	if err := s.CreateFamily(ctx, family); err != nil {
		t.Fatalf("create family: %v", err)
	}

	family.Type = "chat"
	family.UpstreamModel = "test-upstream"
	err := s.UpdateFamily(ctx, family)
	if !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("expected ErrInvalidInput, got %v", err)
	}
}

func TestModelConstraints_ModelIDRequiresExistingFamily(t *testing.T) {
	db := setupModelTestDB(t)

	mode := &ModelMode{
		ModelID:      999,
		Mode:         "default",
		Enabled:      true,
		UpstreamMode: "MODE_DEFAULT",
	}
	if err := db.Create(mode).Error; err == nil {
		t.Fatal("expected foreign-key error for orphan mode")
	}
}

func TestModelConstraints_DefaultModeMustBelongToSameFamily(t *testing.T) {
	db := setupModelTestDB(t)
	s := NewModelStore(db)
	ctx := context.Background()

	f1 := newTestFamily("grok-3", "chat")
	f2 := newTestFamily("grok-4", "chat")
	if err := s.CreateFamily(ctx, f1); err != nil {
		t.Fatalf("create f1: %v", err)
	}
	if err := s.CreateFamily(ctx, f2); err != nil {
		t.Fatalf("create f2: %v", err)
	}

	// Get f2's auto-created default mode
	modes, _ := s.ListModesByFamily(ctx, f2.ID)
	var f2DefaultMode *ModelMode
	for _, m := range modes {
		if m.Mode == "default" {
			f2DefaultMode = m
			break
		}
	}
	if f2DefaultMode == nil {
		t.Fatal("expected f2 to have auto-created default mode")
	}

	if err := db.Model(&ModelFamily{}).
		Where("id = ?", f1.ID).
		Update("default_mode_id", f2DefaultMode.ID).Error; err == nil {
		t.Fatal("expected same-family constraint error for default_mode_id")
	}
}

func TestUpdateMode_CannotDisableDefaultMode(t *testing.T) {
	db := setupModelTestDB(t)
	s := NewModelStore(db)
	ctx := context.Background()

	f := newTestFamily("grok-3", "chat")
	if err := s.CreateFamily(ctx, f); err != nil {
		t.Fatalf("create family: %v", err)
	}

	// Get the auto-created default mode
	modes, _ := s.ListModesByFamily(ctx, f.ID)
	var defaultMode *ModelMode
	for _, m := range modes {
		if m.Mode == "default" {
			defaultMode = m
			break
		}
	}
	if defaultMode == nil {
		t.Fatal("expected auto-created default mode")
	}

	defaultMode.Enabled = false
	err := s.UpdateMode(ctx, defaultMode)
	if !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("expected ErrInvalidInput, got %v", err)
	}
}

func TestUpdateMode_CannotMoveFamily(t *testing.T) {
	db := setupModelTestDB(t)
	s := NewModelStore(db)
	ctx := context.Background()

	f1 := newTestFamily("grok-3", "chat")
	f2 := newTestFamily("grok-4", "chat")
	if err := s.CreateFamily(ctx, f1); err != nil {
		t.Fatalf("create f1: %v", err)
	}
	if err := s.CreateFamily(ctx, f2); err != nil {
		t.Fatalf("create f2: %v", err)
	}

	// Get f1's auto-created default mode
	modes, _ := s.ListModesByFamily(ctx, f1.ID)
	var defaultMode *ModelMode
	for _, m := range modes {
		if m.Mode == "default" {
			defaultMode = m
			break
		}
	}
	if defaultMode == nil {
		t.Fatal("expected auto-created default mode")
	}

	defaultMode.ModelID = f2.ID
	err := s.UpdateMode(ctx, defaultMode)
	if !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("expected ErrInvalidInput, got %v", err)
	}
}

func TestListEnabledFamilies(t *testing.T) {
	db := setupModelTestDB(t)
	s := NewModelStore(db)
	ctx := context.Background()

	f1 := newTestFamily("grok-3", "chat")
	f2 := newTestFamily("grok-4", "chat")
	f3 := newTestImageFamily("grok-5")

	for _, f := range []*ModelFamily{f1, f2, f3} {
		if err := s.CreateFamily(ctx, f); err != nil {
			t.Fatalf("create family %s: %v", f.Model, err)
		}
	}

	// Disable f2 after creation to verify enabled filtering on updates too.
	f2.Enabled = false
	if err := s.UpdateFamily(ctx, f2); err != nil {
		t.Fatalf("disable f2: %v", err)
	}

	enabled, err := s.ListEnabledFamilies(ctx)
	if err != nil {
		t.Fatalf("list enabled: %v", err)
	}
	if len(enabled) != 2 {
		t.Errorf("expected 2 enabled families, got %d", len(enabled))
	}
	for _, f := range enabled {
		if !f.Enabled {
			t.Errorf("expected only enabled families, got disabled: %s", f.Model)
		}
	}
}

func TestDeriveRequestName(t *testing.T) {
	tests := []struct {
		family    string
		mode      string
		isDefault bool
		want      string
	}{
		{"grok-3", "default", true, "grok-3"},
		{"grok-3", "mini", false, "grok-3-mini"},
		{"grok-4", "fast", false, "grok-4-fast"},
	}
	for _, tt := range tests {
		got := DeriveRequestName(tt.family, tt.mode, tt.isDefault)
		if got != tt.want {
			t.Errorf("DeriveRequestName(%q, %q, %v) = %q, want %q",
				tt.family, tt.mode, tt.isDefault, got, tt.want)
		}
	}
}

// --- New tests ---

func TestCreateFamily_AutoCreatesDefaultMode(t *testing.T) {
	db := setupModelTestDB(t)
	s := NewModelStore(db)
	ctx := context.Background()

	f := newTestFamily("grok-3", "chat")
	f.DefaultUpstreamMode = "MODEL_MODE_DEFAULT"
	if err := s.CreateFamily(ctx, f); err != nil {
		t.Fatalf("create family: %v", err)
	}

	if f.DefaultModeID == nil {
		t.Fatal("expected DefaultModeID to be set")
	}

	modes, err := s.ListModesByFamily(ctx, f.ID)
	if err != nil {
		t.Fatalf("list modes: %v", err)
	}
	if len(modes) != 1 {
		t.Fatalf("expected 1 mode, got %d", len(modes))
	}
	if modes[0].Mode != "default" {
		t.Errorf("expected mode name 'default', got %q", modes[0].Mode)
	}
	if modes[0].UpstreamMode != "MODEL_MODE_DEFAULT" {
		t.Errorf("expected upstream_mode 'MODEL_MODE_DEFAULT', got %q", modes[0].UpstreamMode)
	}
	if !modes[0].Enabled {
		t.Error("expected default mode to be enabled")
	}
	if *f.DefaultModeID != modes[0].ID {
		t.Errorf("expected DefaultModeID=%d, got %d", modes[0].ID, *f.DefaultModeID)
	}
}

func TestCreateMode_RejectsDefaultModeName(t *testing.T) {
	db := setupModelTestDB(t)
	s := NewModelStore(db)
	ctx := context.Background()

	f := newTestFamily("grok-3", "chat")
	if err := s.CreateFamily(ctx, f); err != nil {
		t.Fatalf("create family: %v", err)
	}

	mode := &ModelMode{
		ModelID:      f.ID,
		Mode:         "default",
		Enabled:      true,
		UpstreamMode: "MODE_DEFAULT",
	}
	err := s.CreateMode(ctx, mode)
	if !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("expected ErrInvalidInput, got %v", err)
	}
}

func TestDeleteMode_RejectsDefaultMode(t *testing.T) {
	db := setupModelTestDB(t)
	s := NewModelStore(db)
	ctx := context.Background()

	f := newTestFamily("grok-3", "chat")
	if err := s.CreateFamily(ctx, f); err != nil {
		t.Fatalf("create family: %v", err)
	}

	// Get the auto-created default mode
	modes, _ := s.ListModesByFamily(ctx, f.ID)
	var defaultMode *ModelMode
	for _, m := range modes {
		if m.Mode == "default" {
			defaultMode = m
			break
		}
	}
	if defaultMode == nil {
		t.Fatal("expected auto-created default mode")
	}

	err := s.DeleteMode(ctx, defaultMode.ID)
	if !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("expected ErrInvalidInput, got %v", err)
	}
}

func TestUpdateMode_CannotRenameDefaultMode(t *testing.T) {
	db := setupModelTestDB(t)
	s := NewModelStore(db)
	ctx := context.Background()

	f := newTestFamily("grok-3", "chat")
	if err := s.CreateFamily(ctx, f); err != nil {
		t.Fatalf("create family: %v", err)
	}

	// Get the auto-created default mode
	modes, _ := s.ListModesByFamily(ctx, f.ID)
	var defaultMode *ModelMode
	for _, m := range modes {
		if m.Mode == "default" {
			defaultMode = m
			break
		}
	}
	if defaultMode == nil {
		t.Fatal("expected auto-created default mode")
	}

	defaultMode.Mode = "renamed"
	err := s.UpdateMode(ctx, defaultMode)
	if !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("expected ErrInvalidInput, got %v", err)
	}
}

func TestCreateMode_ForceThinking(t *testing.T) {
	db := setupModelTestDB(t)
	s := NewModelStore(db)
	ctx := context.Background()

	f := newTestFamily("grok-3", "chat")
	if err := s.CreateFamily(ctx, f); err != nil {
		t.Fatalf("create family: %v", err)
	}

	mode := &ModelMode{
		ModelID:       f.ID,
		Mode:          "think",
		Enabled:       true,
		UpstreamMode:  "MODE_THINK",
		ForceThinking: true,
	}
	if err := s.CreateMode(ctx, mode); err != nil {
		t.Fatalf("create mode: %v", err)
	}

	got, err := s.GetMode(ctx, mode.ID)
	if err != nil {
		t.Fatalf("get mode: %v", err)
	}
	if !got.ForceThinking {
		t.Fatal("expected ForceThinking to be true")
	}
}

func TestCreateFamily_UpstreamModelRequired(t *testing.T) {
	db := setupModelTestDB(t)
	s := NewModelStore(db)
	ctx := context.Background()

	f := &ModelFamily{
		Model:               "grok-3",
		Type:                "chat",
		PoolFloor:           "basic",
		Enabled:             true,
		DefaultUpstreamMode: "auto",
		// UpstreamModel intentionally omitted
	}
	err := s.CreateFamily(ctx, f)
	if err == nil {
		t.Fatal("expected error for chat family without upstream_model")
	}
	if !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("expected ErrInvalidInput, got %v", err)
	}
}
