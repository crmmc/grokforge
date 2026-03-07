package store

import (
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"time"

	"gorm.io/gorm"
)

// StringSlice is a custom type for JSON-encoded []string in SQLite.
type StringSlice []string

// Value implements driver.Valuer for database storage.
func (s StringSlice) Value() (driver.Value, error) {
	if s == nil {
		return "[]", nil
	}
	b, err := json.Marshal(s)
	if err != nil {
		return nil, err
	}
	return string(b), nil
}

// Scan implements sql.Scanner for reading from database.
func (s *StringSlice) Scan(src any) error {
	if src == nil {
		*s = nil
		return nil
	}
	var bytes []byte
	switch v := src.(type) {
	case string:
		bytes = []byte(v)
	case []byte:
		bytes = v
	default:
		return fmt.Errorf("StringSlice.Scan: unsupported type %T", src)
	}
	return json.Unmarshal(bytes, s)
}

// APIKey represents an API key for authenticating /v1/* requests.
type APIKey struct {
	ID             uint           `gorm:"primaryKey" json:"id"`
	Key            string         `gorm:"uniqueIndex;size:128" json:"key"`
	Name           string         `gorm:"size:128" json:"name"`
	Status         string         `gorm:"index;size:32;default:active" json:"status"`
	ModelWhitelist StringSlice    `gorm:"type:text" json:"model_whitelist"`
	RateLimit      int            `gorm:"default:60" json:"rate_limit"`
	DailyLimit     int            `gorm:"default:1000" json:"daily_limit"`
	DailyUsed      int            `gorm:"default:0" json:"daily_used"`
	TotalUsed      int            `gorm:"default:0" json:"total_used"`
	LastUsedAt     *time.Time     `json:"last_used_at"`
	ExpiresAt      *time.Time     `json:"expires_at"`
	CreatedAt      time.Time      `json:"created_at"`
	UpdatedAt      time.Time      `json:"updated_at"`
	DeletedAt      gorm.DeletedAt `gorm:"index" json:"-"`
}

// Token represents a Grok authentication token.
type Token struct {
	ID                uint           `gorm:"primaryKey" json:"id"`
	Token             string         `gorm:"uniqueIndex;size:512" json:"token"`
	Pool              string         `gorm:"index;size:32" json:"pool"`   // ssoBasic or ssoSuper
	Status            string         `gorm:"index;size:32" json:"status"` // active, disabled, expired, cooling
	ChatQuota         int            `json:"chat_quota"`
	InitialChatQuota  int            `gorm:"default:0" json:"-"`
	ImageQuota        int            `json:"image_quota"`
	InitialImageQuota int            `gorm:"default:0" json:"-"`
	VideoQuota        int            `json:"video_quota"`
	InitialVideoQuota int            `gorm:"default:0" json:"-"`
	FailCount         int            `json:"fail_count"`
	CoolUntil         *time.Time     `json:"cool_until,omitempty"`
	LastUsed          *time.Time     `json:"last_used,omitempty"`
	Remark            string         `gorm:"type:text" json:"remark,omitempty"`
	NsfwEnabled       bool           `gorm:"default:false;index" json:"nsfw_enabled"`
	StatusReason      string         `gorm:"size:256" json:"status_reason,omitempty"`
	Priority          int            `gorm:"default:0;index" json:"priority"`
	CreatedAt         time.Time      `json:"created_at"`
	UpdatedAt         time.Time      `json:"updated_at"`
	DeletedAt         gorm.DeletedAt `gorm:"index" json:"-"`
}

// ConfigEntry stores configuration key-value pairs in database.
type ConfigEntry struct {
	ID        uint           `gorm:"primaryKey" json:"id"`
	Key       string         `gorm:"uniqueIndex;size:128" json:"key"`
	Value     string         `gorm:"type:text" json:"value"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	DeletedAt gorm.DeletedAt `gorm:"index" json:"-"`
}

// UsageLog records token usage history.
type UsageLog struct {
	ID           uint      `gorm:"primaryKey" json:"id"`
	TokenID      uint      `gorm:"index" json:"token_id"`
	APIKeyID     uint      `gorm:"index;default:0" json:"api_key_id"`
	Model        string    `gorm:"size:64" json:"model"`
	Endpoint     string    `gorm:"size:128" json:"endpoint"`
	Status       int       `json:"status"`
	DurationMs   int64     `gorm:"column:duration_ms" json:"duration_ms"`
	TTFTMs       int       `gorm:"default:0" json:"ttft_ms"`
	CacheTokens  int       `gorm:"default:0" json:"cache_tokens"`
	TokensInput  int       `gorm:"default:0" json:"tokens_input"`
	TokensOutput int       `gorm:"default:0" json:"tokens_output"`
	Estimated    bool      `gorm:"default:false" json:"estimated"`
	CreatedAt    time.Time `gorm:"index" json:"created_at"`
}

// AllModels returns all models for AutoMigrate.
func AllModels() []any {
	return []any{
		&Token{},
		&ConfigEntry{},
		&UsageLog{},
		&APIKey{},
	}
}

// MigrateUsageLog handles UsageLog schema changes that AutoMigrate cannot do
// (column rename from "latency" to "duration_ms").
func MigrateUsageLog(db *gorm.DB) error {
	if db.Migrator().HasColumn(&UsageLog{}, "latency") {
		if err := db.Migrator().RenameColumn(&UsageLog{}, "latency", "duration_ms"); err != nil {
			return fmt.Errorf("rename latency to duration_ms: %w", err)
		}
	}
	return nil
}

// migrateTokenQuotasData copies data from old columns and drops them.
// Must be called AFTER AutoMigrate so new columns exist.
func migrateTokenQuotasData(db *gorm.DB) error {
	m := db.Migrator()
	tok := &Token{}

	if m.HasColumn(tok, "quota") {
		if err := db.Exec("UPDATE tokens SET chat_quota = quota WHERE chat_quota = 0").Error; err != nil {
			return fmt.Errorf("migrate quota → chat_quota: %w", err)
		}
		if err := db.Exec("UPDATE tokens SET image_quota = 20 WHERE image_quota = 0").Error; err != nil {
			return fmt.Errorf("set default image_quota: %w", err)
		}
		if err := db.Exec("UPDATE tokens SET video_quota = 10 WHERE video_quota = 0").Error; err != nil {
			return fmt.Errorf("set default video_quota: %w", err)
		}
		if err := m.DropColumn(tok, "quota"); err != nil {
			return fmt.Errorf("drop quota column: %w", err)
		}
	}
	if m.HasColumn(tok, "used_today") {
		if err := m.DropColumn(tok, "used_today"); err != nil {
			return fmt.Errorf("drop used_today column: %w", err)
		}
	}

	if err := db.Exec(`
		UPDATE tokens
		SET initial_chat_quota = chat_quota
		WHERE initial_chat_quota IS NULL OR initial_chat_quota < chat_quota OR initial_chat_quota = 0
	`).Error; err != nil {
		return fmt.Errorf("backfill initial_chat_quota: %w", err)
	}
	if err := db.Exec(`
		UPDATE tokens
		SET initial_image_quota = image_quota
		WHERE initial_image_quota IS NULL OR initial_image_quota < image_quota OR initial_image_quota = 0
	`).Error; err != nil {
		return fmt.Errorf("backfill initial_image_quota: %w", err)
	}
	if err := db.Exec(`
		UPDATE tokens
		SET initial_video_quota = video_quota
		WHERE initial_video_quota IS NULL OR initial_video_quota < video_quota OR initial_video_quota = 0
	`).Error; err != nil {
		return fmt.Errorf("backfill initial_video_quota: %w", err)
	}

	return nil
}

// AutoMigrate runs GORM AutoMigrate for all models and handles data migrations.
func AutoMigrate(db *gorm.DB) error {
	// Handle column renames before generic AutoMigrate
	if err := MigrateUsageLog(db); err != nil {
		return err
	}
	if err := db.AutoMigrate(AllModels()...); err != nil {
		return err
	}
	// Migrate quota data after new columns exist
	return migrateTokenQuotasData(db)
}
