package registry

import (
	"sync"
	"testing"
	"time"

	"github.com/crmmc/grokforge/internal/modelconfig"
	"golang.org/x/net/context"
)

// testModes returns a minimal set of ModeSpec for tests that need modes.
func testModes() []modelconfig.ModeSpec {
	return []modelconfig.ModeSpec{
		{ID: "auto", UpstreamName: "DEFAULT", WindowSeconds: 7200, DefaultQuota: map[string]int{"basic": 30, "super": 30, "heavy": 30}},
		{ID: "heavy", UpstreamName: "HEAVY", WindowSeconds: 86400, DefaultQuota: map[string]int{"basic": 0, "super": 0, "heavy": 10}},
	}
}

func TestRegistry_Basic(t *testing.T) {
	specs := []modelconfig.ModelSpec{
		{ID: "grok-4", DisplayName: "Grok 4", Type: "chat", Enabled: true, PoolFloor: "basic", Mode: "auto", UpstreamMode: "default", PublicType: "chat"},
		{ID: "grok-4-heavy", DisplayName: "Grok 4 Heavy", Type: "chat", Enabled: true, PoolFloor: "heavy", Mode: "heavy", UpstreamMode: "heavy", PublicType: "chat"},
		{ID: "flux-1", DisplayName: "Flux 1", Type: "image_ws", Enabled: true, PoolFloor: "basic", Mode: "auto", PublicType: "image_ws"},
	}
	reg := NewModelRegistry(specs, testModes())

	if got := reg.Count(); got != 3 {
		t.Errorf("Count() = %d, want 3", got)
	}
}

func TestRegistry_Resolve(t *testing.T) {
	specs := []modelconfig.ModelSpec{
		{ID: "grok-4", DisplayName: "Grok 4", Type: "chat", Enabled: true, PoolFloor: "basic", Mode: "auto", UpstreamMode: "default", PublicType: "chat"},
		{ID: "grok-4-heavy", DisplayName: "Grok 4 Heavy", Type: "chat", Enabled: true, PoolFloor: "heavy", Mode: "heavy", UpstreamMode: "heavy", PublicType: "chat"},
	}
	reg := NewModelRegistry(specs, testModes())

	rm, ok := reg.Resolve("grok-4")
	if !ok {
		t.Fatal("Resolve('grok-4') returned false")
	}
	if rm.ID != "grok-4" {
		t.Errorf("ID = %q, want 'grok-4'", rm.ID)
	}
	if rm.Mode != "auto" {
		t.Errorf("Mode = %q, want 'auto'", rm.Mode)
	}
	if rm.PublicType != "chat" {
		t.Errorf("PublicType = %q, want 'chat'", rm.PublicType)
	}

	rm2, ok := reg.Resolve("grok-4-heavy")
	if !ok {
		t.Fatal("Resolve('grok-4-heavy') returned false")
	}
	if rm2.PoolFloor != "heavy" {
		t.Errorf("PoolFloor = %q, want 'heavy'", rm2.PoolFloor)
	}
}

func TestRegistry_ResolveNotFound(t *testing.T) {
	reg := NewModelRegistry(nil, nil)
	_, ok := reg.Resolve("nonexistent")
	if ok {
		t.Error("Resolve('nonexistent') should return false")
	}
}

func TestRegistry_DisabledExcluded(t *testing.T) {
	specs := []modelconfig.ModelSpec{
		{ID: "enabled-model", Type: "chat", Enabled: true, PoolFloor: "basic", Mode: "auto", PublicType: "chat"},
		{ID: "disabled-model", Type: "chat", Enabled: false, PoolFloor: "basic", Mode: "auto", PublicType: "chat"},
	}
	reg := NewModelRegistry(specs, testModes())

	if reg.Count() != 1 {
		t.Errorf("Count() = %d, want 1", reg.Count())
	}
	if _, ok := reg.Resolve("disabled-model"); ok {
		t.Error("disabled model should not be resolvable")
	}
	if _, ok := reg.Resolve("enabled-model"); !ok {
		t.Error("enabled model should be resolvable")
	}
}

func TestRegistry_EnabledByType(t *testing.T) {
	specs := []modelconfig.ModelSpec{
		{ID: "grok-4", Type: "chat", Enabled: true, PoolFloor: "basic", Mode: "auto", PublicType: "chat"},
		{ID: "grok-4-heavy", Type: "chat", Enabled: true, PoolFloor: "heavy", Mode: "heavy", PublicType: "chat"},
		{ID: "flux-1", Type: "image_ws", Enabled: true, PoolFloor: "basic", Mode: "auto", PublicType: "image_ws"},
	}
	reg := NewModelRegistry(specs, testModes())

	chatModels := reg.EnabledByType("chat")
	if len(chatModels) != 2 {
		t.Errorf("EnabledByType('chat') returned %d models, want 2", len(chatModels))
	}

	imageModels := reg.EnabledByType("image_ws")
	if len(imageModels) != 1 {
		t.Errorf("EnabledByType('image_ws') returned %d models, want 1", len(imageModels))
	}
}

func TestRegistry_EnabledByType_Empty(t *testing.T) {
	reg := NewModelRegistry(nil, nil)
	result := reg.EnabledByType("nonexistent")
	if result == nil {
		t.Error("EnabledByType should return non-nil empty slice, got nil")
	}
	if len(result) != 0 {
		t.Errorf("EnabledByType('nonexistent') returned %d models, want 0", len(result))
	}
}

func TestRegistry_AllEnabled(t *testing.T) {
	specs := []modelconfig.ModelSpec{
		{ID: "grok-4", Type: "chat", Enabled: true, PoolFloor: "basic", Mode: "auto", PublicType: "chat"},
		{ID: "flux-1", Type: "image_ws", Enabled: true, PoolFloor: "basic", Mode: "auto", PublicType: "image_ws"},
		{ID: "disabled", Type: "chat", Enabled: false, PoolFloor: "basic", Mode: "auto", PublicType: "chat"},
	}
	reg := NewModelRegistry(specs, testModes())

	all := reg.AllEnabled()
	if len(all) != 2 {
		t.Fatalf("AllEnabled() returned %d models, want 2", len(all))
	}

	// Verify returned slice is a copy
	all[0] = nil
	all2 := reg.AllEnabled()
	for _, rm := range all2 {
		if rm == nil {
			t.Error("modifying AllEnabled() result should not affect registry")
		}
	}
}

func TestRegistry_ResolvePoolFloor(t *testing.T) {
	specs := []modelconfig.ModelSpec{
		{ID: "grok-4", Type: "chat", Enabled: true, PoolFloor: "basic", Mode: "auto", PublicType: "chat"},
		{ID: "grok-4-heavy", Type: "chat", Enabled: true, PoolFloor: "heavy", Mode: "heavy", PublicType: "chat"},
	}
	reg := NewModelRegistry(specs, testModes())

	floor, ok := reg.ResolvePoolFloor("grok-4")
	if !ok {
		t.Fatal("ResolvePoolFloor('grok-4') returned false")
	}
	if floor != "basic" {
		t.Errorf("floor = %q, want 'basic'", floor)
	}

	floor2, ok := reg.ResolvePoolFloor("grok-4-heavy")
	if !ok {
		t.Fatal("ResolvePoolFloor('grok-4-heavy') returned false")
	}
	if floor2 != "heavy" {
		t.Errorf("floor = %q, want 'heavy'", floor2)
	}

	_, ok = reg.ResolvePoolFloor("nonexistent")
	if ok {
		t.Error("ResolvePoolFloor('nonexistent') should return false")
	}
}

func TestRegistry_AllRequestNames(t *testing.T) {
	specs := []modelconfig.ModelSpec{
		{ID: "b-model", Type: "chat", Enabled: true, PoolFloor: "basic", Mode: "auto", PublicType: "chat"},
		{ID: "a-model", Type: "chat", Enabled: true, PoolFloor: "basic", Mode: "auto", PublicType: "chat"},
	}
	reg := NewModelRegistry(specs, testModes())

	names := reg.AllRequestNames()
	if len(names) != 2 {
		t.Fatalf("AllRequestNames() returned %d names, want 2", len(names))
	}
	if names[0] != "a-model" || names[1] != "b-model" {
		t.Errorf("AllRequestNames() = %v, want [a-model b-model]", names)
	}
}

func TestRegistry_ConcurrentResolve(t *testing.T) {
	specs := []modelconfig.ModelSpec{
		{ID: "grok-4", Type: "chat", Enabled: true, PoolFloor: "basic", Mode: "auto", PublicType: "chat"},
		{ID: "flux-1", Type: "image_ws", Enabled: true, PoolFloor: "basic", Mode: "auto", PublicType: "image_ws"},
	}
	reg := NewModelRegistry(specs, testModes())

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ids := []string{"grok-4", "flux-1", "nonexistent"}
			for {
				select {
				case <-ctx.Done():
					return
				default:
					for _, id := range ids {
						reg.Resolve(id)
						reg.ResolvePoolFloor(id)
					}
					_ = reg.AllEnabled()
					_ = reg.EnabledByType("chat")
					_ = reg.Count()
					_ = reg.AllRequestNames()
					_ = reg.AllModes()
					reg.GetMode("auto")
					_ = reg.SupportedModes("basic")
				}
			}
		}()
	}

	wg.Wait()
}

func TestNewTestRegistry(t *testing.T) {
	specs := []modelconfig.ModelSpec{
		{ID: "test-model", Type: "chat", Enabled: true, PoolFloor: "basic", Mode: "auto", PublicType: "chat"},
	}
	reg := NewTestRegistry(specs, testModes())
	if reg.Count() != 1 {
		t.Errorf("NewTestRegistry Count() = %d, want 1", reg.Count())
	}
	rm, ok := reg.Resolve("test-model")
	if !ok {
		t.Fatal("NewTestRegistry model should be resolvable")
	}
	if rm.ID != "test-model" {
		t.Errorf("ID = %q, want 'test-model'", rm.ID)
	}
}

func TestRegistry_QuotaSyncAndCooldown(t *testing.T) {
	quotaSyncFalse := false
	specs := []modelconfig.ModelSpec{
		{ID: "tracked", Type: "chat", Enabled: true, PoolFloor: "basic", Mode: "auto", UpstreamMode: "default", PublicType: "chat"},
		{ID: "untracked", Type: "chat", Enabled: true, PoolFloor: "basic", QuotaSync: &quotaSyncFalse, CooldownSeconds: 120, UpstreamMode: "default", PublicType: "chat"},
	}
	reg := NewModelRegistry(specs, testModes())

	rm1, ok := reg.Resolve("tracked")
	if !ok {
		t.Fatal("tracked model not found")
	}
	if !rm1.QuotaSync {
		t.Error("tracked model QuotaSync should be true")
	}
	if rm1.CooldownSeconds != 0 {
		t.Errorf("tracked model CooldownSeconds = %d, want 0", rm1.CooldownSeconds)
	}

	rm2, ok := reg.Resolve("untracked")
	if !ok {
		t.Fatal("untracked model not found")
	}
	if rm2.QuotaSync {
		t.Error("untracked model QuotaSync should be false")
	}
	if rm2.CooldownSeconds != 120 {
		t.Errorf("untracked model CooldownSeconds = %d, want 120", rm2.CooldownSeconds)
	}
}

func TestRegistry_GetMode(t *testing.T) {
	modes := testModes()
	reg := NewModelRegistry(nil, modes)

	m, ok := reg.GetMode("auto")
	if !ok {
		t.Fatal("GetMode('auto') returned false")
	}
	if m.UpstreamName != "DEFAULT" {
		t.Errorf("UpstreamName = %q, want 'DEFAULT'", m.UpstreamName)
	}
	if m.WindowSeconds != 7200 {
		t.Errorf("WindowSeconds = %d, want 7200", m.WindowSeconds)
	}

	m2, ok := reg.GetMode("heavy")
	if !ok {
		t.Fatal("GetMode('heavy') returned false")
	}
	if m2.DefaultQuota["heavy"] != 10 {
		t.Errorf("DefaultQuota[heavy] = %d, want 10", m2.DefaultQuota["heavy"])
	}

	_, ok = reg.GetMode("nonexistent")
	if ok {
		t.Error("GetMode('nonexistent') should return false")
	}
}

func TestRegistry_AllModes(t *testing.T) {
	modes := testModes()
	reg := NewModelRegistry(nil, modes)

	all := reg.AllModes()
	if len(all) != 2 {
		t.Fatalf("AllModes() returned %d modes, want 2", len(all))
	}
	// Verify order preserved
	if all[0].ID != "auto" {
		t.Errorf("AllModes()[0].ID = %q, want 'auto'", all[0].ID)
	}
	if all[1].ID != "heavy" {
		t.Errorf("AllModes()[1].ID = %q, want 'heavy'", all[1].ID)
	}

	// Verify returned slice is a copy
	all[0] = modelconfig.ModeSpec{}
	all2 := reg.AllModes()
	if all2[0].ID != "auto" {
		t.Error("modifying AllModes() result should not affect registry")
	}
}

func TestRegistry_AllModes_Empty(t *testing.T) {
	reg := NewModelRegistry(nil, nil)
	all := reg.AllModes()
	if len(all) != 0 {
		t.Errorf("AllModes() returned %d modes, want 0", len(all))
	}
}

func TestRegistry_SupportedModes(t *testing.T) {
	modes := testModes()
	reg := NewModelRegistry(nil, modes)

	// basic pool: auto has 30 > 0, heavy has 0 → only auto
	basicModes := reg.SupportedModes("basic")
	if len(basicModes) != 1 {
		t.Fatalf("SupportedModes('basic') returned %d modes, want 1", len(basicModes))
	}
	if basicModes[0].ID != "auto" {
		t.Errorf("SupportedModes('basic')[0].ID = %q, want 'auto'", basicModes[0].ID)
	}

	// heavy pool: auto has 30 > 0, heavy has 10 > 0 → both
	heavyModes := reg.SupportedModes("heavy")
	if len(heavyModes) != 2 {
		t.Fatalf("SupportedModes('heavy') returned %d modes, want 2", len(heavyModes))
	}
	if heavyModes[0].ID != "auto" || heavyModes[1].ID != "heavy" {
		t.Errorf("SupportedModes('heavy') = [%s, %s], want [auto, heavy]", heavyModes[0].ID, heavyModes[1].ID)
	}

	// nonexistent pool: no modes
	none := reg.SupportedModes("nonexistent")
	if len(none) != 0 {
		t.Errorf("SupportedModes('nonexistent') returned %d modes, want 0", len(none))
	}
}
