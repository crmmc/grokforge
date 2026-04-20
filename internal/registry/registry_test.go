package registry

import (
	"sync"
	"testing"
	"time"

	"github.com/crmmc/grokforge/internal/modelconfig"
	"golang.org/x/net/context"
)

func TestRegistry_Basic(t *testing.T) {
	specs := []modelconfig.ModelSpec{
		{ID: "grok-4", DisplayName: "Grok 4", Type: "chat", Enabled: true, PoolFloor: "basic", QuotaMode: "auto", UpstreamModel: "grok-3", UpstreamMode: "default", PublicType: "chat"},
		{ID: "grok-4-heavy", DisplayName: "Grok 4 Heavy", Type: "chat", Enabled: true, PoolFloor: "heavy", QuotaMode: "heavy", UpstreamModel: "grok-3", UpstreamMode: "heavy", PublicType: "chat"},
		{ID: "flux-1", DisplayName: "Flux 1", Type: "image_ws", Enabled: true, PoolFloor: "basic", QuotaMode: "auto", PublicType: "image_ws"},
	}
	reg := NewModelRegistry(specs)

	if got := reg.Count(); got != 3 {
		t.Errorf("Count() = %d, want 3", got)
	}
}

func TestRegistry_Resolve(t *testing.T) {
	specs := []modelconfig.ModelSpec{
		{ID: "grok-4", DisplayName: "Grok 4", Type: "chat", Enabled: true, PoolFloor: "basic", QuotaMode: "auto", UpstreamModel: "grok-3", UpstreamMode: "default", PublicType: "chat"},
		{ID: "grok-4-heavy", DisplayName: "Grok 4 Heavy", Type: "chat", Enabled: true, PoolFloor: "heavy", QuotaMode: "heavy", UpstreamModel: "grok-3", UpstreamMode: "heavy", PublicType: "chat"},
	}
	reg := NewModelRegistry(specs)

	rm, ok := reg.Resolve("grok-4")
	if !ok {
		t.Fatal("Resolve('grok-4') returned false")
	}
	if rm.ID != "grok-4" {
		t.Errorf("ID = %q, want 'grok-4'", rm.ID)
	}
	if rm.UpstreamModel != "grok-3" {
		t.Errorf("UpstreamModel = %q, want 'grok-3'", rm.UpstreamModel)
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
	reg := NewModelRegistry(nil)
	_, ok := reg.Resolve("nonexistent")
	if ok {
		t.Error("Resolve('nonexistent') should return false")
	}
}

func TestRegistry_DisabledExcluded(t *testing.T) {
	specs := []modelconfig.ModelSpec{
		{ID: "enabled-model", Type: "chat", Enabled: true, PoolFloor: "basic", QuotaMode: "auto", PublicType: "chat"},
		{ID: "disabled-model", Type: "chat", Enabled: false, PoolFloor: "basic", QuotaMode: "auto", PublicType: "chat"},
	}
	reg := NewModelRegistry(specs)

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
		{ID: "grok-4", Type: "chat", Enabled: true, PoolFloor: "basic", QuotaMode: "auto", PublicType: "chat"},
		{ID: "grok-4-heavy", Type: "chat", Enabled: true, PoolFloor: "heavy", QuotaMode: "heavy", PublicType: "chat"},
		{ID: "flux-1", Type: "image_ws", Enabled: true, PoolFloor: "basic", QuotaMode: "auto", PublicType: "image_ws"},
	}
	reg := NewModelRegistry(specs)

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
	reg := NewModelRegistry(nil)
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
		{ID: "grok-4", Type: "chat", Enabled: true, PoolFloor: "basic", QuotaMode: "auto", PublicType: "chat"},
		{ID: "flux-1", Type: "image_ws", Enabled: true, PoolFloor: "basic", QuotaMode: "auto", PublicType: "image_ws"},
		{ID: "disabled", Type: "chat", Enabled: false, PoolFloor: "basic", QuotaMode: "auto", PublicType: "chat"},
	}
	reg := NewModelRegistry(specs)

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
		{ID: "grok-4", Type: "chat", Enabled: true, PoolFloor: "basic", QuotaMode: "auto", PublicType: "chat"},
		{ID: "grok-4-heavy", Type: "chat", Enabled: true, PoolFloor: "heavy", QuotaMode: "heavy", PublicType: "chat"},
	}
	reg := NewModelRegistry(specs)

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
		{ID: "b-model", Type: "chat", Enabled: true, PoolFloor: "basic", QuotaMode: "auto", PublicType: "chat"},
		{ID: "a-model", Type: "chat", Enabled: true, PoolFloor: "basic", QuotaMode: "auto", PublicType: "chat"},
	}
	reg := NewModelRegistry(specs)

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
		{ID: "grok-4", Type: "chat", Enabled: true, PoolFloor: "basic", QuotaMode: "auto", PublicType: "chat"},
		{ID: "flux-1", Type: "image_ws", Enabled: true, PoolFloor: "basic", QuotaMode: "auto", PublicType: "image_ws"},
	}
	reg := NewModelRegistry(specs)

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
				}
			}
		}()
	}

	wg.Wait()
}

func TestNewTestRegistry(t *testing.T) {
	specs := []modelconfig.ModelSpec{
		{ID: "test-model", Type: "chat", Enabled: true, PoolFloor: "basic", QuotaMode: "auto", PublicType: "chat"},
	}
	reg := NewTestRegistry(specs)
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
