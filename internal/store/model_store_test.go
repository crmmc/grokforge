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
		Model:     model,
		Type:      typ,
		PoolFloor: "basic",
		Enabled:   true,
	}
}

func newTestMode(modelID uint, mode string) *ModelMode {
	return &ModelMode{
		ModelID:       modelID,
		Mode:          mode,
		Enabled:       true,
		UpstreamModel: "test-upstream",
		UpstreamMode:  "MODE_" + mode,
	}
}

func createDefaultMode(t *testing.T, s *ModelStore, ctx context.Context, familyID uint) *ModelMode {
	t.Helper()
	mode := newTestMode(familyID, "default")
	if err := s.CreateMode(ctx, mode); err != nil {
		t.Fatalf("create default mode: %v", err)
	}
	return mode
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
	createDefaultMode(t, s, ctx, f.ID)

	// Create mode
	m := &ModelMode{ModelID: f.ID, Mode: "mini", Enabled: true, UpstreamMode: "mini", UpstreamModel: "grok-3"}
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

	// List by family
	modes, err := s.ListModesByFamily(ctx, f.ID)
	if err != nil {
		t.Fatalf("list modes: %v", err)
	}
	if len(modes) != 2 {
		t.Errorf("expected 2 modes, got %d", len(modes))
	}

	// Update mode
	got.UpstreamModel = "grok-3-updated"
	if err := s.UpdateMode(ctx, got); err != nil {
		t.Fatalf("update mode: %v", err)
	}
	got2, _ := s.GetMode(ctx, m.ID)
	if got2.UpstreamModel != "grok-3-updated" {
		t.Errorf("expected updated upstream_model, got %q", got2.UpstreamModel)
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
	createDefaultMode(t, s, ctx, f.ID)
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
	createDefaultMode(t, s, ctx, f1.ID)
	createDefaultMode(t, s, ctx, f2.ID)
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

	// Create family "grok-3" with a default mode, then add non-default mode "mini"
	f := newTestFamily("grok-3", "chat")
	if err := s.CreateFamily(ctx, f); err != nil {
		t.Fatalf("create family: %v", err)
	}
	defaultMode := newTestMode(f.ID, "default")
	if err := s.CreateMode(ctx, defaultMode); err != nil {
		t.Fatalf("create default mode: %v", err)
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

	defaultMode := newTestMode(f2.ID, "default")
	if err := s.CreateMode(ctx, defaultMode); err != nil {
		t.Fatalf("create default mode: %v", err)
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
	defaultMode := newTestMode(f2.ID, "default")
	if err := s.CreateMode(ctx, defaultMode); err != nil {
		t.Fatalf("create default mode: %v", err)
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

	// Scenario A: grok-4 has a default mode plus non-default "mini", then create grok-4-mini family
	f1 := newTestFamily("grok-4", "chat")
	if err := s.CreateFamily(ctx, f1); err != nil {
		t.Fatalf("create grok-4: %v", err)
	}
	defaultMode := newTestMode(f1.ID, "default")
	if err := s.CreateMode(ctx, defaultMode); err != nil {
		t.Fatalf("create default mode: %v", err)
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
	defaultMode2 := newTestMode(fb.ID, "default")
	if err := s2.CreateMode(ctx, defaultMode2); err != nil {
		t.Fatalf("create default mode: %v", err)
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
	m := createDefaultMode(t, s, ctx, f.ID)
	f.DefaultModeID = &m.ID

	// Updating family without changing model name should not conflict
	f.DisplayName = "Updated Name"
	if err := s.UpdateFamily(ctx, f); err != nil {
		t.Errorf("update family should not conflict: %v", err)
	}

	// Updating mode without changing mode name should not conflict
	m.UpstreamModel = "updated-upstream"
	if err := s.UpdateMode(ctx, m); err != nil {
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
	createDefaultMode(t, s, ctx, f.ID)
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

func TestDeleteMode_ClearDefaultModeID(t *testing.T) {
	db := setupModelTestDB(t)
	s := NewModelStore(db)
	ctx := context.Background()

	f := newTestFamily("grok-3", "chat")
	if err := s.CreateFamily(ctx, f); err != nil {
		t.Fatalf("create family: %v", err)
	}
	m := newTestMode(f.ID, "default")
	if err := s.CreateMode(ctx, m); err != nil {
		t.Fatalf("create mode: %v", err)
	}

	// Set as default mode
	f.DefaultModeID = &m.ID
	if err := s.UpdateFamily(ctx, f); err != nil {
		t.Fatalf("set default mode: %v", err)
	}

	// Delete the default mode
	if err := s.DeleteMode(ctx, m.ID); err != nil {
		t.Fatalf("delete mode: %v", err)
	}

	// Family's default_mode_id should be nil
	got, _ := s.GetFamily(ctx, f.ID)
	if got.DefaultModeID != nil {
		t.Errorf("expected nil default_mode_id after deleting default mode, got %v", *got.DefaultModeID)
	}
}

func TestDeleteMode_DefaultWithRemainingModesFails(t *testing.T) {
	db := setupModelTestDB(t)
	s := NewModelStore(db)
	ctx := context.Background()

	f := newTestFamily("grok-3", "chat")
	if err := s.CreateFamily(ctx, f); err != nil {
		t.Fatalf("create family: %v", err)
	}
	defaultMode := &ModelMode{ModelID: f.ID, Mode: "default", Enabled: true, UpstreamModel: "grok-3", UpstreamMode: "MODE_DEFAULT"}
	if err := s.CreateMode(ctx, defaultMode); err != nil {
		t.Fatalf("create default mode: %v", err)
	}
	expertMode := &ModelMode{ModelID: f.ID, Mode: "expert", Enabled: true, UpstreamModel: "grok-3", UpstreamMode: "MODE_EXPERT"}
	if err := s.CreateMode(ctx, expertMode); err != nil {
		t.Fatalf("create expert mode: %v", err)
	}

	f.DefaultModeID = &defaultMode.ID
	if err := s.UpdateFamily(ctx, f); err != nil {
		t.Fatalf("set default mode: %v", err)
	}

	err := s.DeleteMode(ctx, defaultMode.ID)
	if err == nil {
		t.Fatal("expected error when deleting default mode with remaining modes")
	}
	if !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("expected ErrInvalidInput, got %v", err)
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

	f := &ModelFamily{Model: "grok-3", Type: "chat", PoolFloor: "invalid_pool", Enabled: true}
	err := s.CreateFamily(ctx, f)
	if err == nil {
		t.Fatal("expected error for invalid pool_floor")
	}
}

func TestCreateFamily_NormalizesTrimmedIdentifiersAndBlankQuota(t *testing.T) {
	db := setupModelTestDB(t)
	s := NewModelStore(db)
	ctx := context.Background()

	blankQuota := "   "
	f := &ModelFamily{
		Model:        "  grok-3  ",
		Type:         "  chat  ",
		PoolFloor:    "  basic  ",
		Enabled:      true,
		QuotaDefault: &blankQuota,
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
	if got.QuotaDefault != nil {
		t.Fatalf("expected blank quota_default to normalize to nil, got %v", *got.QuotaDefault)
	}
}

func TestUpdateFamily_DefaultModeID_Validation(t *testing.T) {
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

	// Create mode under f2
	m := createDefaultMode(t, s, ctx, f2.ID)

	// Try to set f1's default_mode_id to a mode belonging to f2
	f1.DefaultModeID = &m.ID
	err := s.UpdateFamily(ctx, f1)
	if err == nil {
		t.Fatal("expected error when default_mode_id belongs to another family")
	}

	// Non-existent mode ID
	nonExistent := uint(9999)
	f1.DefaultModeID = &nonExistent
	err = s.UpdateFamily(ctx, f1)
	if err == nil {
		t.Fatal("expected error for non-existent default_mode_id")
	}
}

func TestListEnabledFamilies(t *testing.T) {
	db := setupModelTestDB(t)
	s := NewModelStore(db)
	ctx := context.Background()

	f1 := newTestFamily("grok-3", "chat")
	f2 := newTestFamily("grok-4", "chat")
	f3 := newTestFamily("grok-5", "image")

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

func TestCreateFamily_PersistsDisabledState(t *testing.T) {
	db := setupModelTestDB(t)
	s := NewModelStore(db)
	ctx := context.Background()

	f := &ModelFamily{
		Model:     "grok-disabled",
		Type:      "chat",
		PoolFloor: "basic",
		Enabled:   false,
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
	defaultMode := newTestMode(f.ID, "default")
	if err := s.CreateMode(ctx, defaultMode); err != nil {
		t.Fatalf("create default mode: %v", err)
	}

	mode := &ModelMode{
		ModelID:       f.ID,
		Mode:          "expert",
		Enabled:       false,
		UpstreamModel: "grok-3",
		UpstreamMode:  "MODE_EXPERT",
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

func TestCreateMode_FirstModeDefaultBecomesDefault(t *testing.T) {
	db := setupModelTestDB(t)
	s := NewModelStore(db)
	ctx := context.Background()

	f := newTestFamily("grok-3", "chat")
	if err := s.CreateFamily(ctx, f); err != nil {
		t.Fatalf("create family: %v", err)
	}
	mode := &ModelMode{
		ModelID:       f.ID,
		Mode:          "default",
		Enabled:       true,
		UpstreamModel: "grok-3",
		UpstreamMode:  "MODE_DEFAULT",
	}
	if err := s.CreateMode(ctx, mode); err != nil {
		t.Fatalf("create mode: %v", err)
	}

	got, err := s.GetFamily(ctx, f.ID)
	if err != nil {
		t.Fatalf("get family: %v", err)
	}
	if got.DefaultModeID == nil || *got.DefaultModeID != mode.ID {
		t.Fatalf("expected first mode to become default, got %v", got.DefaultModeID)
	}
}

func TestCreateMode_FirstModeMustBeNamedDefault(t *testing.T) {
	db := setupModelTestDB(t)
	s := NewModelStore(db)
	ctx := context.Background()

	f := newTestFamily("grok-3", "chat")
	if err := s.CreateFamily(ctx, f); err != nil {
		t.Fatalf("create family: %v", err)
	}
	mode := &ModelMode{
		ModelID:       f.ID,
		Mode:          "expert",
		Enabled:       true,
		UpstreamModel: "grok-3",
		UpstreamMode:  "MODE_EXPERT",
	}
	err := s.CreateMode(ctx, mode)
	if !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("expected ErrInvalidInput, got %v", err)
	}
}

func TestCreateMode_FirstModeMustBeEnabled(t *testing.T) {
	db := setupModelTestDB(t)
	s := NewModelStore(db)
	ctx := context.Background()

	f := newTestFamily("grok-3", "chat")
	if err := s.CreateFamily(ctx, f); err != nil {
		t.Fatalf("create family: %v", err)
	}
	mode := &ModelMode{
		ModelID:       f.ID,
		Mode:          "default",
		Enabled:       false,
		UpstreamModel: "grok-3",
		UpstreamMode:  "MODE_DEFAULT",
	}
	err := s.CreateMode(ctx, mode)
	if !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("expected ErrInvalidInput, got %v", err)
	}
}

func TestCreateMode_ImageFamilyAllowsEmptyUpstream(t *testing.T) {
	db := setupModelTestDB(t)
	s := NewModelStore(db)
	ctx := context.Background()

	f := newTestFamily("grok-imagine-image", "image")
	f.PoolFloor = "super"
	if err := s.CreateFamily(ctx, f); err != nil {
		t.Fatalf("create family: %v", err)
	}

	mode := &ModelMode{
		ModelID: f.ID,
		Mode:    "default",
		Enabled: true,
	}
	if err := s.CreateMode(ctx, mode); err != nil {
		t.Fatalf("expected image mode creation to succeed without upstream, got %v", err)
	}
}

func TestCreateMode_ImageFamilyRejectsUpstreamMapping(t *testing.T) {
	db := setupModelTestDB(t)
	s := NewModelStore(db)
	ctx := context.Background()

	f := newTestFamily("grok-imagine-image", "image")
	f.PoolFloor = "super"
	if err := s.CreateFamily(ctx, f); err != nil {
		t.Fatalf("create family: %v", err)
	}

	mode := &ModelMode{
		ModelID:       f.ID,
		Mode:          "default",
		Enabled:       true,
		UpstreamModel: "grok-3",
		UpstreamMode:  "MODEL_MODE_FAST",
	}
	err := s.CreateMode(ctx, mode)
	if !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("expected ErrInvalidInput, got %v", err)
	}
}

func TestCreateMode_NormalizesTrimmedIdentifiersAndBlankQuota(t *testing.T) {
	db := setupModelTestDB(t)
	s := NewModelStore(db)
	ctx := context.Background()

	f := newTestFamily("grok-3", "chat")
	if err := s.CreateFamily(ctx, f); err != nil {
		t.Fatalf("create family: %v", err)
	}
	blankQuota := "   "
	override := "  super  "
	mode := &ModelMode{
		ModelID:           f.ID,
		Mode:              "  default  ",
		Enabled:           true,
		PoolFloorOverride: &override,
		UpstreamModel:     "  grok-3  ",
		UpstreamMode:      "  MODE_DEFAULT  ",
		QuotaOverride:     &blankQuota,
	}
	if err := s.CreateMode(ctx, mode); err != nil {
		t.Fatalf("create mode: %v", err)
	}

	got, err := s.GetMode(ctx, mode.ID)
	if err != nil {
		t.Fatalf("get mode: %v", err)
	}
	if got.Mode != "default" {
		t.Fatalf("expected trimmed mode, got %q", got.Mode)
	}
	if got.UpstreamModel != "grok-3" {
		t.Fatalf("expected trimmed upstream_model, got %q", got.UpstreamModel)
	}
	if got.UpstreamMode != "MODE_DEFAULT" {
		t.Fatalf("expected trimmed upstream_mode, got %q", got.UpstreamMode)
	}
	if got.PoolFloorOverride == nil || *got.PoolFloorOverride != "super" {
		t.Fatalf("expected trimmed pool floor override, got %v", got.PoolFloorOverride)
	}
	if got.QuotaOverride != nil {
		t.Fatalf("expected blank quota_override to normalize to nil, got %v", *got.QuotaOverride)
	}
}

func TestUpdateFamily_TypeChangeRequiresExistingModeUpstream(t *testing.T) {
	db := setupModelTestDB(t)
	s := NewModelStore(db)
	ctx := context.Background()

	family := newTestFamily("grok-imagine-image", "image")
	if err := s.CreateFamily(ctx, family); err != nil {
		t.Fatalf("create family: %v", err)
	}

	mode := &ModelMode{
		ModelID: family.ID,
		Mode:    "default",
		Enabled: true,
	}
	if err := s.CreateMode(ctx, mode); err != nil {
		t.Fatalf("create mode: %v", err)
	}

	family.Type = "chat"
	err := s.UpdateFamily(ctx, family)
	if !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("expected ErrInvalidInput, got %v", err)
	}
}

func TestModelConstraints_ModelIDRequiresExistingFamily(t *testing.T) {
	db := setupModelTestDB(t)

	mode := &ModelMode{
		ModelID:       999,
		Mode:          "default",
		Enabled:       true,
		UpstreamModel: "grok-3",
		UpstreamMode:  "MODE_DEFAULT",
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

	mode := newTestMode(f2.ID, "default")
	if err := s.CreateMode(ctx, mode); err != nil {
		t.Fatalf("create mode: %v", err)
	}

	if err := db.Model(&ModelFamily{}).
		Where("id = ?", f1.ID).
		Update("default_mode_id", mode.ID).Error; err == nil {
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
	defaultMode := newTestMode(f.ID, "default")
	if err := s.CreateMode(ctx, defaultMode); err != nil {
		t.Fatalf("create default mode: %v", err)
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
	mode := newTestMode(f1.ID, "default")
	if err := s.CreateMode(ctx, mode); err != nil {
		t.Fatalf("create mode: %v", err)
	}

	mode.ModelID = f2.ID
	err := s.UpdateMode(ctx, mode)
	if !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("expected ErrInvalidInput, got %v", err)
	}
}

func TestUpdateFamily_DefaultModeMustBeEnabled(t *testing.T) {
	db := setupModelTestDB(t)
	s := NewModelStore(db)
	ctx := context.Background()

	f := newTestFamily("grok-3", "chat")
	if err := s.CreateFamily(ctx, f); err != nil {
		t.Fatalf("create family: %v", err)
	}
	defaultMode := newTestMode(f.ID, "default")
	if err := s.CreateMode(ctx, defaultMode); err != nil {
		t.Fatalf("create default mode: %v", err)
	}
	expertMode := newTestMode(f.ID, "expert")
	expertMode.Enabled = false
	expertMode.UpstreamMode = "MODE_EXPERT"
	if err := s.CreateMode(ctx, expertMode); err != nil {
		t.Fatalf("create expert mode: %v", err)
	}

	f.DefaultModeID = &expertMode.ID
	err := s.UpdateFamily(ctx, f)
	if !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("expected ErrInvalidInput, got %v", err)
	}
}
