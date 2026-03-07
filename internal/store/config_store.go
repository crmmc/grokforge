package store

import (
	"encoding/json"
	"fmt"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// ConfigStore handles configuration persistence via config_entries table.
type ConfigStore struct {
	db *gorm.DB
}

// NewConfigStore creates a new ConfigStore.
func NewConfigStore(db *gorm.DB) *ConfigStore {
	return &ConfigStore{db: db}
}

// Get returns the value for a config key, or empty string if not found.
func (s *ConfigStore) Get(key string) (string, error) {
	var entry ConfigEntry
	err := s.db.Where("key = ?", key).First(&entry).Error
	if err == gorm.ErrRecordNotFound {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("config get %q: %w", key, err)
	}
	return entry.Value, nil
}

// Set upserts a config key-value pair.
func (s *ConfigStore) Set(key, value string) error {
	entry := ConfigEntry{Key: key, Value: value}
	return s.db.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "key"}},
		DoUpdates: clause.AssignmentColumns([]string{"value", "updated_at"}),
	}).Create(&entry).Error
}

// GetAll returns all config entries as a map.
func (s *ConfigStore) GetAll() (map[string]string, error) {
	var entries []ConfigEntry
	if err := s.db.Find(&entries).Error; err != nil {
		return nil, fmt.Errorf("config get all: %w", err)
	}
	m := make(map[string]string, len(entries))
	for _, e := range entries {
		m[e.Key] = e.Value
	}
	return m, nil
}

// SetMany upserts multiple config key-value pairs in a single transaction.
func (s *ConfigStore) SetMany(kvs map[string]string) error {
	return s.db.Transaction(func(tx *gorm.DB) error {
		for k, v := range kvs {
			entry := ConfigEntry{Key: k, Value: v}
			if err := tx.Clauses(clause.OnConflict{
				Columns:   []clause.Column{{Name: "key"}},
				DoUpdates: clause.AssignmentColumns([]string{"value", "updated_at"}),
			}).Create(&entry).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

// Delete removes a config entry by key.
func (s *ConfigStore) Delete(key string) error {
	return s.db.Where("key = ?", key).Delete(&ConfigEntry{}).Error
}

// GetJSON reads a config key and unmarshals JSON value into dest.
func (s *ConfigStore) GetJSON(key string, dest any) error {
	val, err := s.Get(key)
	if err != nil {
		return err
	}
	if val == "" {
		return nil
	}
	return json.Unmarshal([]byte(val), dest)
}

// SetJSON marshals value to JSON and stores it.
func (s *ConfigStore) SetJSON(key string, value any) error {
	b, err := json.Marshal(value)
	if err != nil {
		return fmt.Errorf("config set json %q: %w", key, err)
	}
	return s.Set(key, string(b))
}
