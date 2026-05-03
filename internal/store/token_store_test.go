package store

import (
	"context"
	"testing"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// setupTokenTestDB creates an in-memory SQLite database for token testing
func setupTokenTestDB(t *testing.T) *gorm.DB {
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

// Test 1: Token model has Remark field (string, nullable)
func TestTokenModel_RemarkField(t *testing.T) {
	db := setupTokenTestDB(t)
	ctx := context.Background()

	token := &Token{
		Token:  "test_token_remark",
		Pool:   "ssoBasic",
		Status: TokenStatusActive,
		Remark: "This is a test remark",
	}

	if err := db.WithContext(ctx).Create(token).Error; err != nil {
		t.Fatalf("failed to create token with remark: %v", err)
	}

	var found Token
	if err := db.WithContext(ctx).First(&found, token.ID).Error; err != nil {
		t.Fatalf("failed to find token: %v", err)
	}

	if found.Remark != "This is a test remark" {
		t.Errorf("expected remark 'This is a test remark', got '%s'", found.Remark)
	}
}

// Test 2: Token model has NsfwEnabled field (bool, default false)
func TestTokenModel_NsfwEnabledField(t *testing.T) {
	db := setupTokenTestDB(t)
	ctx := context.Background()

	// Test default value is false
	token := &Token{
		Token:  "test_token_nsfw_default",
		Pool:   "ssoBasic",
		Status: TokenStatusActive,
	}

	if err := db.WithContext(ctx).Create(token).Error; err != nil {
		t.Fatalf("failed to create token: %v", err)
	}

	var found Token
	if err := db.WithContext(ctx).First(&found, token.ID).Error; err != nil {
		t.Fatalf("failed to find token: %v", err)
	}

	if found.NsfwEnabled != false {
		t.Errorf("expected NsfwEnabled to be false by default, got %v", found.NsfwEnabled)
	}

	// Test setting to true
	token2 := &Token{
		Token:       "test_token_nsfw_true",
		Pool:        "ssoBasic",
		Status:      TokenStatusActive,
		NsfwEnabled: true,
	}

	if err := db.WithContext(ctx).Create(token2).Error; err != nil {
		t.Fatalf("failed to create token with NsfwEnabled=true: %v", err)
	}

	var found2 Token
	if err := db.WithContext(ctx).First(&found2, token2.ID).Error; err != nil {
		t.Fatalf("failed to find token2: %v", err)
	}

	if found2.NsfwEnabled != true {
		t.Errorf("expected NsfwEnabled to be true, got %v", found2.NsfwEnabled)
	}
}

// Test 3: AutoMigrate adds columns without data loss
func TestTokenModel_AutoMigratePreservesData(t *testing.T) {
	db := setupTokenTestDB(t)
	ctx := context.Background()

	// Create a token with all fields
	token := &Token{
		Token:       "test_token_migrate",
		Pool:        "ssoSuper",
		Status:      TokenStatusActive,
		Quotas:      IntMap{"auto": 100},
		FailCount:   2,
		Remark:      "Original remark",
		NsfwEnabled: true,
	}

	if err := db.WithContext(ctx).Create(token).Error; err != nil {
		t.Fatalf("failed to create token: %v", err)
	}

	// Run AutoMigrate again (simulates app restart)
	if err := AutoMigrate(db); err != nil {
		t.Fatalf("failed to run AutoMigrate: %v", err)
	}

	// Verify data is preserved
	var found Token
	if err := db.WithContext(ctx).First(&found, token.ID).Error; err != nil {
		t.Fatalf("failed to find token after AutoMigrate: %v", err)
	}

	if found.Token != "test_token_migrate" {
		t.Errorf("expected token 'test_token_migrate', got '%s'", found.Token)
	}
	if found.Remark != "Original remark" {
		t.Errorf("expected remark 'Original remark', got '%s'", found.Remark)
	}
	if found.NsfwEnabled != true {
		t.Errorf("expected NsfwEnabled true, got %v", found.NsfwEnabled)
	}
}

func TestTokenModel_AutoMigrateResetsLegacyCoolingStatus(t *testing.T) {
	db := setupTokenTestDB(t)
	ctx := context.Background()

	token := &Token{
		Token:  "test_token_legacy_cooling",
		Pool:   "ssoSuper",
		Status: "cooling",
		Quotas: IntMap{"auto": 0},
	}
	if err := db.WithContext(ctx).Create(token).Error; err != nil {
		t.Fatalf("failed to create legacy cooling token: %v", err)
	}

	if err := AutoMigrate(db); err != nil {
		t.Fatalf("failed to run AutoMigrate: %v", err)
	}

	var found Token
	if err := db.WithContext(ctx).First(&found, token.ID).Error; err != nil {
		t.Fatalf("failed to find token after AutoMigrate: %v", err)
	}
	if found.Status != TokenStatusActive {
		t.Errorf("expected legacy cooling status to reset to active, got %q", found.Status)
	}
}

// Test 4: Token can be created with remark and nsfw_enabled
func TestTokenStore_CreateWithRemarkAndNsfw(t *testing.T) {
	db := setupTokenTestDB(t)
	store := NewTokenStore(db)
	ctx := context.Background()

	token := &Token{
		Token:       "test_token_create_full",
		Pool:        "ssoBasic",
		Status:      TokenStatusActive,
		Quotas:      IntMap{"auto": 50},
		Remark:      "Test remark for creation",
		NsfwEnabled: true,
	}

	if err := store.CreateToken(ctx, token); err != nil {
		t.Fatalf("failed to create token: %v", err)
	}

	found, err := store.GetToken(ctx, token.ID)
	if err != nil {
		t.Fatalf("failed to get token: %v", err)
	}

	if found.Remark != "Test remark for creation" {
		t.Errorf("expected remark 'Test remark for creation', got '%s'", found.Remark)
	}
	if found.NsfwEnabled != true {
		t.Errorf("expected NsfwEnabled true, got %v", found.NsfwEnabled)
	}
}

// Test 5: Token can be queried by nsfw_enabled filter
func TestTokenStore_ListTokensFiltered(t *testing.T) {
	db := setupTokenTestDB(t)
	store := NewTokenStore(db)
	ctx := context.Background()

	// Create tokens with different NSFW settings
	tokens := []*Token{
		{Token: "nsfw_true_1", Pool: "ssoBasic", Status: TokenStatusActive, NsfwEnabled: true},
		{Token: "nsfw_true_2", Pool: "ssoBasic", Status: TokenStatusActive, NsfwEnabled: true},
		{Token: "nsfw_false_1", Pool: "ssoBasic", Status: TokenStatusActive, NsfwEnabled: false},
		{Token: "nsfw_false_2", Pool: "ssoSuper", Status: TokenStatusActive, NsfwEnabled: false},
		{Token: "nsfw_true_disabled", Pool: "ssoBasic", Status: TokenStatusDisabled, NsfwEnabled: true},
	}

	for _, tok := range tokens {
		if err := store.CreateToken(ctx, tok); err != nil {
			t.Fatalf("failed to create token: %v", err)
		}
	}

	// Test filter by NsfwEnabled=true
	nsfwTrue := true
	filtered, err := store.ListTokensFiltered(ctx, TokenFilter{NsfwEnabled: &nsfwTrue})
	if err != nil {
		t.Fatalf("failed to list tokens filtered: %v", err)
	}
	if len(filtered) != 3 {
		t.Errorf("expected 3 tokens with NsfwEnabled=true, got %d", len(filtered))
	}

	// Test filter by NsfwEnabled=false
	nsfwFalse := false
	filtered, err = store.ListTokensFiltered(ctx, TokenFilter{NsfwEnabled: &nsfwFalse})
	if err != nil {
		t.Fatalf("failed to list tokens filtered: %v", err)
	}
	if len(filtered) != 2 {
		t.Errorf("expected 2 tokens with NsfwEnabled=false, got %d", len(filtered))
	}

	// Test filter by Status
	statusActive := TokenStatusActive
	filtered, err = store.ListTokensFiltered(ctx, TokenFilter{Status: &statusActive})
	if err != nil {
		t.Fatalf("failed to list tokens filtered by status: %v", err)
	}
	if len(filtered) != 4 {
		t.Errorf("expected 4 tokens with status=active, got %d", len(filtered))
	}

	// Test combined filter
	filtered, err = store.ListTokensFiltered(ctx, TokenFilter{Status: &statusActive, NsfwEnabled: &nsfwTrue})
	if err != nil {
		t.Fatalf("failed to list tokens with combined filter: %v", err)
	}
	if len(filtered) != 2 {
		t.Errorf("expected 2 tokens with status=active AND NsfwEnabled=true, got %d", len(filtered))
	}
}

// Test BatchUpdateTokens
func TestTokenStore_BatchUpdateTokens(t *testing.T) {
	db := setupTokenTestDB(t)
	store := NewTokenStore(db)
	ctx := context.Background()

	// Create test tokens
	tokens := []*Token{
		{Token: "batch_1", Pool: "ssoBasic", Status: TokenStatusActive, NsfwEnabled: false},
		{Token: "batch_2", Pool: "ssoBasic", Status: TokenStatusActive, NsfwEnabled: false},
		{Token: "batch_3", Pool: "ssoBasic", Status: TokenStatusActive, NsfwEnabled: false},
	}

	for _, tok := range tokens {
		if err := store.CreateToken(ctx, tok); err != nil {
			t.Fatalf("failed to create token: %v", err)
		}
	}

	// Batch update: disable tokens 1 and 2
	disabled := TokenStatusDisabled
	count, err := store.BatchUpdateTokens(ctx, BatchUpdateRequest{
		IDs:    []uint{tokens[0].ID, tokens[1].ID},
		Status: &disabled,
	})
	if err != nil {
		t.Fatalf("failed to batch update tokens: %v", err)
	}
	if count != 2 {
		t.Errorf("expected 2 rows affected, got %d", count)
	}

	// Verify tokens are disabled
	for i := 0; i < 2; i++ {
		found, err := store.GetToken(ctx, tokens[i].ID)
		if err != nil {
			t.Fatalf("failed to get token: %v", err)
		}
		if found.Status != TokenStatusDisabled {
			t.Errorf("expected token %d status to be disabled, got %s", i, found.Status)
		}
	}

	// Token 3 should still be active
	found, err := store.GetToken(ctx, tokens[2].ID)
	if err != nil {
		t.Fatalf("failed to get token 3: %v", err)
	}
	if found.Status != TokenStatusActive {
		t.Errorf("expected token 3 status to be active, got %s", found.Status)
	}

	// Batch update: enable NSFW for tokens 1 and 3
	nsfwTrue := true
	count, err = store.BatchUpdateTokens(ctx, BatchUpdateRequest{
		IDs:         []uint{tokens[0].ID, tokens[2].ID},
		NsfwEnabled: &nsfwTrue,
	})
	if err != nil {
		t.Fatalf("failed to batch update NSFW: %v", err)
	}
	if count != 2 {
		t.Errorf("expected 2 rows affected, got %d", count)
	}

	// Verify NSFW is enabled
	for _, id := range []uint{tokens[0].ID, tokens[2].ID} {
		found, err := store.GetToken(ctx, id)
		if err != nil {
			t.Fatalf("failed to get token: %v", err)
		}
		if !found.NsfwEnabled {
			t.Errorf("expected token %d NsfwEnabled to be true", id)
		}
	}

	// Token 2 should still have NsfwEnabled=false
	found, _ = store.GetToken(ctx, tokens[1].ID)
	if found.NsfwEnabled {
		t.Error("expected token 2 NsfwEnabled to be false")
	}
}
