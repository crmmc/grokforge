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

	// Create mode
	m := &ModelMode{ModelID: f.ID, Mode: "mini", UpstreamMode: "mini", UpstreamModel: "grok-3"}
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
	if len(modes) != 1 {
		t.Errorf("expected 1 mode, got %d", len(modes))
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
	m1 := &ModelMode{ModelID: f.ID, Mode: "mini"}
	if err := s.CreateMode(ctx, m1); err != nil {
		t.Fatalf("create first mode: %v", err)
	}
	m2 := &ModelMode{ModelID: f.ID, Mode: "mini"}
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
	m1 := &ModelMode{ModelID: f1.ID, Mode: "fast"}
	m2 := &ModelMode{ModelID: f2.ID, Mode: "fast"}
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

	// Create family "grok-3" with mode "mini" -> derived name "grok-3-mini"
	f := newTestFamily("grok-3", "chat")
	if err := s.CreateFamily(ctx, f); err != nil {
		t.Fatalf("create family: %v", err)
	}
	m := &ModelMode{ModelID: f.ID, Mode: "mini"}
	if err := s.CreateMode(ctx, m); err != nil {
		t.Fatalf("create mode: %v", err)
	}

	// Creating family "grok-3-mini" should conflict with derived name
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

	// Creating mode "mini" under "grok-3" -> derived "grok-3-mini" conflicts with family
	m := &ModelMode{ModelID: f2.ID, Mode: "mini"}
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
	m := &ModelMode{ModelID: f2.ID, Mode: "mini"}
	if err := s.CreateMode(ctx, m); err != nil {
		t.Fatalf("create mode: %v", err)
	}

	// Update mode name to "fast" -> derived "grok-3-fast" conflicts
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

	// Scenario A: grok-4 + mini mode exists, then create grok-4-mini family
	f1 := newTestFamily("grok-4", "chat")
	if err := s.CreateFamily(ctx, f1); err != nil {
		t.Fatalf("create grok-4: %v", err)
	}
	m := &ModelMode{ModelID: f1.ID, Mode: "mini"}
	if err := s.CreateMode(ctx, m); err != nil {
		t.Fatalf("create mini mode: %v", err)
	}
	f2 := newTestFamily("grok-4-mini", "chat")
	err := s.CreateFamily(ctx, f2)
	if !errors.Is(err, ErrConflict) {
		t.Errorf("scenario A: expected ErrConflict, got %v", err)
	}

	// Scenario B: fresh DB, grok-4-mini family exists, then add mini mode to grok-4
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
	m2 := &ModelMode{ModelID: fb.ID, Mode: "mini"}
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
	m := &ModelMode{ModelID: f.ID, Mode: "fast"}
	if err := s.CreateMode(ctx, m); err != nil {
		t.Fatalf("create mode: %v", err)
	}

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
	for _, mode := range []string{"mini", "fast", "turbo"} {
		m := &ModelMode{ModelID: f.ID, Mode: mode}
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
	m := &ModelMode{ModelID: f.ID, Mode: "default"}
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
	m := &ModelMode{ModelID: f2.ID, Mode: "fast"}
	if err := s.CreateMode(ctx, m); err != nil {
		t.Fatalf("create mode: %v", err)
	}

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

	// Disable f2 after creation (GORM default:true treats false as zero value on create)
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
