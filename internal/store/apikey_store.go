package store

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"time"

	"gorm.io/gorm"
)

// generateAPIKey generates a new API key with "gf-" prefix + 48 hex chars = 51 chars total.
func generateAPIKey() (string, error) {
	b := make([]byte, 24)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("generate api key: %w", err)
	}
	return "gf-" + hex.EncodeToString(b), nil
}

// APIKeyStore handles API key persistence and queries.
type APIKeyStore struct {
	db *gorm.DB
}

// NewAPIKeyStore creates a new APIKeyStore.
func NewAPIKeyStore(db *gorm.DB) *APIKeyStore {
	return &APIKeyStore{db: db}
}

// List returns paginated API keys with optional status filter, ordered by created_at DESC.
func (s *APIKeyStore) List(ctx context.Context, page, pageSize int, status string) ([]*APIKey, int64, error) {
	var total int64
	q := s.db.WithContext(ctx).Model(&APIKey{})
	if status != "" {
		q = q.Where("status = ?", status)
	}
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	var keys []*APIKey
	offset := (page - 1) * pageSize
	if err := q.Order("created_at DESC").Offset(offset).Limit(pageSize).Find(&keys).Error; err != nil {
		return nil, 0, err
	}
	return keys, total, nil
}

// GetByID returns an API key by ID.
func (s *APIKeyStore) GetByID(ctx context.Context, id uint) (*APIKey, error) {
	var ak APIKey
	if err := s.db.WithContext(ctx).First(&ak, id).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &ak, nil
}

// GetByKey returns an API key by key string.
func (s *APIKeyStore) GetByKey(ctx context.Context, key string) (*APIKey, error) {
	var ak APIKey
	if err := s.db.WithContext(ctx).Where("key = ?", key).First(&ak).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &ak, nil
}

// Create creates a new API key, generating the key value.
func (s *APIKeyStore) Create(ctx context.Context, ak *APIKey) error {
	key, err := generateAPIKey()
	if err != nil {
		return err
	}
	ak.Key = key
	return s.db.WithContext(ctx).Create(ak).Error
}

// Update saves changes to an existing API key.
func (s *APIKeyStore) Update(ctx context.Context, ak *APIKey) error {
	return s.db.WithContext(ctx).Save(ak).Error
}

// Delete soft-deletes an API key by ID.
func (s *APIKeyStore) Delete(ctx context.Context, id uint) error {
	return s.db.WithContext(ctx).Delete(&APIKey{}, id).Error
}

// Regenerate generates a new key value for an existing API key, returns the new key.
func (s *APIKeyStore) Regenerate(ctx context.Context, id uint) (string, error) {
	newKey, err := generateAPIKey()
	if err != nil {
		return "", err
	}
	result := s.db.WithContext(ctx).Model(&APIKey{}).Where("id = ?", id).Update("key", newKey)
	if result.Error != nil {
		return "", result.Error
	}
	if result.RowsAffected == 0 {
		return "", ErrNotFound
	}
	return newKey, nil
}

// CountByStatus returns counts of API keys grouped by status.
func (s *APIKeyStore) CountByStatus(ctx context.Context) (total, active, inactive, expired, rateLimited int, err error) {
	var results []struct {
		Status string
		Count  int
	}
	err = s.db.WithContext(ctx).
		Model(&APIKey{}).
		Select("status, COUNT(*) as count").
		Group("status").
		Find(&results).Error
	if err != nil {
		return
	}
	for _, r := range results {
		total += r.Count
		switch r.Status {
		case "active":
			active = r.Count
		case "inactive":
			inactive = r.Count
		case "expired":
			expired = r.Count
		case "rate_limited":
			rateLimited = r.Count
		}
	}
	return
}

// IncrementUsage atomically increments daily_used and total_used, and updates last_used_at.
func (s *APIKeyStore) IncrementUsage(ctx context.Context, id uint) error {
	now := time.Now()
	return s.db.WithContext(ctx).
		Model(&APIKey{}).
		Where("id = ?", id).
		Updates(map[string]any{
			"daily_used":   gorm.Expr("daily_used + 1"),
			"total_used":   gorm.Expr("total_used + 1"),
			"last_used_at": now,
		}).Error
}

// ResetDailyUsage resets daily_used to 0 for all non-deleted API keys.
func (s *APIKeyStore) ResetDailyUsage(ctx context.Context) error {
	return s.db.WithContext(ctx).
		Model(&APIKey{}).
		Where("deleted_at IS NULL").
		Update("daily_used", 0).Error
}
